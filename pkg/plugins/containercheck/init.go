package containercheck

import (
	"k8s-cmdb/pkg/plugins"
	"sync"
)

func init() {
	plugins.Register(&Plugin{
		checkLock: sync.Mutex{},
		stopChan:  make(chan int),
	})
}
