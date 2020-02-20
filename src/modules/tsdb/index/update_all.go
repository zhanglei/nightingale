package index

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/backend/rpc"
	. "github.com/didi/nightingale/src/modules/tsdb/config"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
)

var (
	semaUpdateIndexAll *semaphore.Semaphore
)

func StartUpdateIndexTask() {
	if Config.Index.MaxConns != 0 {
		semaUpdateIndexAll = semaphore.NewSemaphore(Config.Index.MaxConns / 2)
	} else {
		semaUpdateIndexAll = semaphore.NewSemaphore(10)
	}

	t1 := time.NewTicker(time.Duration(Config.Index.RebuildInterval) * time.Second)
	for {
		<-t1.C

		RebuildAllIndex()
	}
}

// 重建所有索引，推往nsq
func RebuildAllIndex() error {
	//postTms := time.Now().Unix()
	start := time.Now().Unix()
	lastTs := start - Config.Index.ActiveDuration
	aggrNum := Config.NSQ.Batch

	if !UpdateIndexToNSQLock.TryAcquire() {
		return fmt.Errorf("RebuildAllIndex already Rebuiding..")
	} else {
		defer UpdateIndexToNSQLock.Release()
		var pushCnt = 0
		var oldCnt = 0
		for idx, _ := range IndexedItemCacheBigMap {
			keys := IndexedItemCacheBigMap[idx].Keys()

			i := 0
			tmpList := make([]*dataobj.TsdbItem, aggrNum)

			for _, key := range keys {
				item := IndexedItemCacheBigMap[idx].Get(key)
				if item == nil {
					continue
				}

				if item.Timestamp < lastTs { //缓存中的数据太旧了,不能用于索引的全量更新
					IndexedItemCacheBigMap[idx].Remove(key)
					logger.Debug("push index remove:", item)
					oldCnt++
					continue
				}
				logger.Debug("push index:", item)
				pushCnt++
				tmpList[i] = item
				i = i + 1

				if i == aggrNum {
					semaUpdateIndexAll.Acquire()
					go func(items []*dataobj.TsdbItem) {
						defer semaUpdateIndexAll.Release()
						rpc.Push2Index(rpc.ALLINDEX, items)
					}(tmpList)

					i = 0
				}
			}

			if i != 0 {
				semaUpdateIndexAll.Acquire()
				go func(items []*dataobj.TsdbItem) {
					defer semaUpdateIndexAll.Release()
					rpc.Push2Index(rpc.ALLINDEX, items)
				}(tmpList[:i])
			}
		}

		atomic.AddInt64(&PushIndex, int64(pushCnt))
		atomic.AddInt64(&OldIndex, int64(oldCnt))

		end := time.Now().Unix()
		logger.Infof("RebuildAllIndex end : start_ts[%d] latency[%d] old/success/all[%d/%d/%d]", start, end-start, oldCnt, pushCnt, oldCnt+pushCnt)
	}

	return nil
}
