package plugins

import (
	"fmt"
	"k8s.io/klog/v2"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type pluginManager struct {
	plugins         map[string]Plugin
	configLock      sync.Mutex
	concurrencyLock sync.Mutex
}

type Plugin interface {
	Name() string
	Setup(string, string) error
	Stop() error
	Ready() bool
	GetResult() interface{}
	Execute()
}

var (
	// Pm xxx
	Pm *pluginManager
)

func init() {
	Pm = NewPluginManager()
}

func NewPluginManager() *pluginManager {
	return &pluginManager{
		plugins: make(map[string]Plugin),
	}
}

func Register(plugin Plugin) {
	Pm.Register(plugin)
}

func (pm *pluginManager) Register(plugin Plugin) {
	pm.plugins[plugin.Name()] = plugin
}

func (pm *pluginManager) GetPlugin(plugin string) Plugin {
	if p, ok := pm.plugins[plugin]; ok {
		return p
	} else {
		return nil
	}
}

func (pm *pluginManager) SetupPlugin(pluginStr, pluginDir, runMode string) error {
	for _, plugin := range strings.Split(pluginStr, ",") {
		if p := pm.GetPlugin(plugin); p == nil {
			return fmt.Errorf("Get Plugin %s failed, nil result", plugin)
		} else {
			klog.Infof("Start plugin: %s", plugin)
			err := p.Setup(filepath.Join(pluginDir, plugin+".conf"), runMode)
			if err != nil {
				return fmt.Errorf("Setup plugin %s failed: %s", p.Name(), err.Error())
			}
		}
	}
	return nil
}

func (pm *pluginManager) Ready(pluginStr string) bool {
	for _, plugin := range strings.Split(pluginStr, ",") {
		p := pm.GetPlugin(plugin)
		for {
			if p.Ready() {
				break
			}
			time.Sleep(time.Second)
		}
	}
	return true
}

func (pm *pluginManager) GetResult(pluginStr string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, plugin := range strings.Split(pluginStr, ",") {
		p := pm.GetPlugin(plugin)
		result[plugin] = p.GetResult()
	}
	return result
}
