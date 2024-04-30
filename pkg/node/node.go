package node

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"k8s-cmdb/pkg/plugins"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

type NodeAgentConfig struct {
	//Name string `yaml:"name"`
	//PluginDir string `yaml:"pluginDir"`
	interval int `yaml:"interval"`
}

type NodeInfo struct {
	Name string

	// result
	Result     map[string]interface{} `yaml:"result"`
	resultLock sync.Mutex

	// netcheck
	// dnscheck

	uploader  Uploader
	nac       *NodeAgentConfig
	plugins   string
	runMode   string
	pluginDir string
	sync.Mutex
}

type Uploader interface {
	Upload(*NodeInfo) error
}

func (n *NodeInfo) Start() {
	var err error

	err = plugins.Pm.SetupPlugin(n.plugins, n.pluginDir, n.runMode)
	if err != nil {
		klog.Fatalf(err.Error())
	}

	plugins.Pm.Ready(n.plugins)
	n.Result = plugins.Pm.GetResult(n.plugins)

	if n.runMode == "once" {
		content, _ := yaml.Marshal(n.Result)
		fmt.Println(string(content))

		err = n.uploader.Upload(n)
		if err != nil {
			klog.Errorf(err.Error())
		}

		time.Sleep(time.Duration(n.nac.interval) * time.Second)
		n.Lock()
		n.Result = plugins.Pm.GetResult(n.plugins)
		n.Unlock()
		return
	}

	go func() {
		for {
			err = n.uploader.Upload(n)
			if err != nil {
				klog.Errorf(err.Error())
			}

			time.Sleep(time.Duration(n.nac.interval) * time.Second)
			n.Lock()
			n.Result = plugins.Pm.GetResult(n.plugins)
			n.Unlock()
			n.Renew()
		}
	}()
}

func (n *NodeInfo) SetResult(key string, value interface{}) {
	n.resultLock.Lock()
	n.Result[key] = value
	n.resultLock.Unlock()
}

func (n *NodeInfo) Stop() {

}

func (n *NodeInfo) Renew() {
	if n.nac.interval < 60 {
		n.nac.interval = 60
	}
}

func NewNodeInfo(nac *NodeAgentConfig, uploader Uploader, runMode string, pluginStr string, pluginDir string) NodeInfo {
	n := NodeInfo{
		uploader:  uploader,
		nac:       nac,
		plugins:   pluginStr,
		runMode:   runMode,
		pluginDir: pluginDir,
	}
	return n
}
