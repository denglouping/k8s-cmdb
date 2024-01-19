package node

import (
	"k8s-cmdb/pkg/customization"
	"k8s-cmdb/pkg/plugins"
	"k8s.io/klog/v2"
	"os"
	"sync"
	"time"
)

type NodeAgentConfig struct {
	Name                  string                                      `yaml:"name"`
	Processes             []string                                    `yaml:"processes"`
	Customizeconfirmation []customization.CustomizeconfirmationConfig `yaml:"customizeconfirmation"`
	PluginDir             string                                      `yaml:"pluginDir"`
	//Plugins               map[string]interface{}                      `yaml:"plugins"`
	interval int `yaml:"interval"`
}

type NodeInfo struct {
	Name string

	// result
	Result     map[string]interface{} `yaml:"result"`
	resultLock sync.Mutex

	// netcheck
	// dnscheck

	uploader Uploader
	nac      *NodeAgentConfig
	plugins  string
}

type Uploader interface {
	Upload(*NodeInfo) error
}

func (n *NodeInfo) Start() {
	var err error

	err = plugins.Pm.SetupPlugin(n.plugins, n.nac.PluginDir, n.SetResult)
	if err != nil {
		klog.Fatalf(err.Error())
	}

	go func() {
		// plugins
		if n.Result == nil {
			n.Result = make(map[string]interface{})
		}

		time.Sleep(time.Minute)

		for {
			n.Renew()
			err = n.uploader.Upload(n)
			if err != nil {
				klog.Errorf(err.Error())
			}

			time.Sleep(time.Duration(n.nac.interval) * time.Second)
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
	n.Name = n.nac.Name
	if n.Name == "" {
		n.Name = os.Getenv("NODE_NAME")
	}

	if n.nac.interval < 60 {
		n.nac.interval = 60
	}
}

func NewNodeInfo(nac *NodeAgentConfig, uploader Uploader, pluginStr string) NodeInfo {
	n := NodeInfo{
		uploader: uploader,
		nac:      nac,
		plugins:  pluginStr,
	}
	return n
}
