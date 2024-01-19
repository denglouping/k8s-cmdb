package plugins

import (
	"fmt"
	"k8s.io/klog/v2"
	"path/filepath"
	"strings"
	"sync"
)

type pluginManager struct {
	plugins         map[string]Plugin
	configLock      sync.Mutex
	concurrencyLock sync.Mutex
}

type Plugin interface {
	Name() string
	Setup(string, func(key string, value interface{})) error
	Stop() error
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

func (pm *pluginManager) SetupPlugin(pluginStr string, pluginDir string, setValue func(key string, value interface{})) error {
	for _, plugin := range strings.Split(pluginStr, ",") {
		if p := pm.GetPlugin(plugin); p == nil {
			return fmt.Errorf("Get Plugin %s failed, nil result", plugin)
		} else {
			klog.Infof("Start plugin: %s", plugin)
			err := p.Setup(filepath.Join(pluginDir, plugin+".conf"), setValue)
			if err != nil {
				return fmt.Errorf("Setup plugin %s failed: %s", p.Name(), err.Error())
			}
		}
	}
	return nil
}
