package cache

import (
	"sync"

	"github.com/toolkits/pkg/logger"
)

//TagKTagvStruct
type TagkTagvsStruct struct { // ns/metric/tagk -> tagv
	sync.RWMutex
	Tagvs map[string]int64 `json:"tagvs"`
}

func NewTagkTagvsStruct() *TagkTagvsStruct {
	return &TagkTagvsStruct{Tagvs: make(map[string]int64, 0)}
}

func (t *TagkTagvsStruct) Set(v string, now int64) {
	t.Lock()
	defer t.Unlock()
	t.Tagvs[v] = now
}

func (t *TagkTagvsStruct) Clean(now, timeDuration int64) {
	t.Lock()
	defer t.Unlock()

	for k, v := range t.Tagvs {
		if now-v > timeDuration {
			delete(t.Tagvs, k)
			logger.Errorf("[clean index tagv] tagv:%s now:%d time duration:%d updated:%d",
				k, now, timeDuration, v)
		}
	}
}

func (t *TagkTagvsStruct) CleanEndpoint(endpoint string) {
	t.Lock()
	defer t.Unlock()
	delete(t.Tagvs, endpoint)

}

func (t *TagkTagvsStruct) CleanTagv(tagv string) {
	t.Lock()
	defer t.Unlock()
	delete(t.Tagvs, tagv)
}

func (t *TagkTagvsStruct) GetTagvs() []string {
	t.RLock()
	defer t.RUnlock()
	tagvs := []string{}
	for v, _ := range t.Tagvs {
		tagvs = append(tagvs, v)
	}
	return tagvs
}
