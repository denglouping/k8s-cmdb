package util

import (
	"github.com/patrickmn/go-cache"
	"sync"
	"time"
)

var (
	DefaultCache *cache.Cache
	CacheMutex   sync.Mutex
)

func init() {
	DefaultCache = cache.New(20*time.Minute, 5*time.Minute)
}

func GetSyncCache(key string) (interface{}, bool) {
	CacheMutex.Lock()
	result, ok := DefaultCache.Get(key)
	CacheMutex.Unlock()
	return result, ok
}

func GetOrUpdateCache(key string, f func(args ...interface{}) (interface{}, error)) (interface{}, error) {
	var result interface{}
	var ok bool
	var err error
	result, ok = DefaultCache.Get(key)
	if !ok {
		result, err = f()
		if err == nil {
			DefaultCache.Set(key, result, 20*time.Minute)
		}

		return result, err
	} else {
		return result, nil
	}
}
