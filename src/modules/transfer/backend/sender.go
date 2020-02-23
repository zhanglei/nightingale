package backend

import (
	"time"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/transfer/cache"
	. "github.com/didi/nightingale/src/modules/transfer/config"
	"github.com/didi/nightingale/src/toolkits/str"
)

// send
const (
	DefaultSendTaskSleepInterval = time.Millisecond * 50 //默认睡眠间隔为50ms
	MAX_SEND_RETRY               = 10
)

var (
	MinStep int //最小上报周期,单位sec
)

func startSendTasks() {

	tsdbConcurrent := Config.Tsdb.WorkerNum
	if tsdbConcurrent < 1 {
		tsdbConcurrent = 1
	}

	judgeConcurrent := Config.Judge.WorkerNum
	if judgeConcurrent < 1 {
		judgeConcurrent = 1
	}

	if Config.Tsdb.Enabled {
		for node, item := range Config.Tsdb.ClusterList {
			for _, addr := range item.Addrs {
				queue := TsdbQueues[node+addr]
				go Send2TsdbTask(queue, node, addr, tsdbConcurrent)
			}
		}
	}

	if Config.Judge.Enabled {
		judgeQueue := JudgeQueues.GetAll()
		for instance, queue := range judgeQueue {
			go Send2JudgeTask(queue, instance, judgeConcurrent)
		}
	}
}

func Send2TsdbTask(Q *list.SafeListLimited, node string, addr string, concurrent int) {
	batch := Config.Tsdb.Batch // 一次发送,最多batch条数据
	Q = TsdbQueues[node+addr]

	sema := semaphore.NewSemaphore(concurrent)

	for {
		items := Q.PopBackBy(batch)
		count := len(items)

		if count == 0 {
			time.Sleep(DefaultSendTaskSleepInterval)
			continue
		}

		tsdbItems := make([]*dataobj.TsdbItem, count)
		for i := 0; i < count; i++ {
			tsdbItems[i] = items[i].(*dataobj.TsdbItem)
			logger.Debug("send to tsdb->: ", tsdbItems[i])
		}

		//控制并发
		sema.Acquire()
		go func(addr string, tsdbItems []*dataobj.TsdbItem, count int) {
			defer sema.Release()

			resp := &dataobj.SimpleRpcResponse{}
			var err error
			sendOk := false
			for i := 0; i < 3; i++ { //最多重试3次
				err = TsdbConnPools.Call(addr, "Tsdb.Send", tsdbItems, resp)
				if err == nil {
					sendOk = true
					break
				}
				time.Sleep(time.Millisecond * 10)
			}

			// statistics
			//atomic.AddInt64(&PointOut2Tsdb, int64(count))
			if !sendOk {
				logger.Errorf("send %v to tsdb %s:%s fail: %v", tsdbItems, node, addr, err)
			} else {
				logger.Infof("send to tsdb %s:%s ok", node, addr)
			}
		}(addr, tsdbItems, count)
	}
}

// 将数据 打入 某个Tsdb的发送缓存队列, 具体是哪一个Tsdb 由一致性哈希 决定
func Push2TsdbSendQueue(items []*dataobj.MetricValue) {
	for _, item := range items {
		tsdbItem, err := convert2TsdbItem(item)
		if err != nil {
			logger.Error("E:", err)
			continue
		}

		node, err := TsdbNodeRing.GetNode(item.PK())
		if err != nil {
			logger.Error("E:", err)
			continue
		}

		cnode := Config.Tsdb.ClusterList[node]
		errCnt := 0
		for _, addr := range cnode.Addrs {
			Q := TsdbQueues[node+addr]
			if !Q.PushFront(tsdbItem) {
				errCnt += 1
			}
		}

		// statistics
		if errCnt > 0 {
			logger.Error("Push2TsdbSendQueue err num: ", errCnt)
		}
	}
}

func Send2JudgeTask(Q *list.SafeListLimited, addr string, concurrent int) {
	batch := Config.Judge.Batch
	sema := semaphore.NewSemaphore(concurrent)

	for {
		items := Q.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(DefaultSendTaskSleepInterval)
			continue
		}

		judgeItems := make([]*dataobj.JudgeItem, count)
		for i := 0; i < count; i++ {
			judgeItems[i] = items[i].(*dataobj.JudgeItem)
			logger.Info("send to judge: ", judgeItems[i])
		}

		sema.Acquire()
		go func(addr string, judgeItems []*dataobj.JudgeItem, count int) {
			defer sema.Release()

			resp := &dataobj.SimpleRpcResponse{}
			var err error
			sendOk := false
			for i := 0; i < MAX_SEND_RETRY; i++ {
				err = JudgeConnPools.Call(addr, "Judge.Send", judgeItems, resp)
				if err == nil {
					sendOk = true
					break
				}
				logger.Warningf("send judge %s fail: %v", addr, err)
				time.Sleep(time.Millisecond * 10)
			}

			if !sendOk {
				logger.Errorf("send judge %s fail: %v", addr, err)
			}

		}(addr, judgeItems, count)
	}
}

func Push2JudgeSendQueue(items []*dataobj.MetricValue) {
	for _, item := range items {
		key := str.PK(item.Metric, item.Endpoint)
		stras := cache.StraMap.GetByKey(key)

		for _, stra := range stras {

			if !TagMatch(stra.Tags, item.TagsMap) {
				continue
			}
			judgeItem := &dataobj.JudgeItem{
				Endpoint:  item.Endpoint,
				Metric:    item.Metric,
				Value:     item.Value,
				Timestamp: item.Timestamp,
				DsType:    item.CounterType,
				Tags:      item.Tags,
				TagsMap:   item.TagsMap,
				Step:      int(item.Step),
				Sid:       stra.Id,
			}

			q, exists := JudgeQueues.Get(stra.JudgeInstance)
			if exists {
				q.PushFront(judgeItem)
			}
		}

	}
}

// 打到Tsdb的数据,要根据rrdtool的特定 来限制 step、counterType、timestamp
func convert2TsdbItem(d *dataobj.MetricValue) (*dataobj.TsdbItem, error) {
	item := &dataobj.TsdbItem{
		Endpoint:  d.Endpoint,
		Metric:    d.Metric,
		Value:     d.Value,
		Timestamp: d.Timestamp,
		Tags:      d.Tags,
		TagsMap:   d.TagsMap,
		Step:      int(d.Step),
	}

	if item.Step < MinStep {
		item.Step = MinStep
	}
	item.Heartbeat = item.Step * 2

	if d.CounterType == dataobj.GAUGE {
		item.DsType = dataobj.GAUGE
		item.Min = "U"
		item.Max = "U"
	} else if d.CounterType == dataobj.COUNTER {
		item.DsType = dataobj.COUNTER
		item.Min = "0"
		item.Max = "U"
	} else if d.CounterType == dataobj.DERIVE {
		item.DsType = dataobj.DERIVE
		item.Min = "0"
		item.Max = "U"
	} else { //其他类型统一转为 GAUGE
		item.DsType = dataobj.GAUGE
		item.Min = "U"
		item.Max = "U"
	}

	item.Timestamp = alignTs(item.Timestamp, int64(item.Step)) //item.Timestamp - item.Timestamp%int64(item.Step)

	return item, nil
}

func alignTs(ts int64, period int64) int64 {
	return ts - ts%period
}

func TagMatch(straTags []model.Tag, tag map[string]string) bool {
	for _, stag := range straTags {
		if _, exists := tag[stag.Tkey]; !exists {
			return false
		}
		var match bool
		if stag.Topt == "=" { //当前策略tagkey对应的tagv
			for _, v := range stag.Tval {
				if tag[stag.Tkey] == v {
					match = true
					break
				}
			}
		}
		if !match {
			return match
		}

		if stag.Topt == "!=" {
			match = true
			for _, v := range stag.Tval {
				if tag[stag.Tkey] == v {
					match = false
					return match
				}
			}
		}
	}
	return true
}
