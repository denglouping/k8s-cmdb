package options

import (
	"github.com/spf13/pflag"
)

type NodeAgentOptions struct {
	HostPath    string
	Upstream    string
	ConfigPath  string
	CopyNum     int
	Plugins     string
	RunMode     string
	PluginDir   string
	CMNamespace string
	Addr        string
}

func NewNodeAgentOptions() *NodeAgentOptions {
	return &NodeAgentOptions{}
}

func (brro *NodeAgentOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&brro.Plugins, "plugins", "customizationcheck,dnscheck,dockercheck,hwcheck,processcheck,diskcheck,netcheck", "plugins")

	fs.StringVar(&brro.HostPath, "hostPath", "/host", "hostPath")
	fs.StringVar(&brro.Upstream, "upstream", "cluster", "cluster, mysql")
	fs.StringVar(&brro.RunMode, "runMode", "daemon", "daemon, once")
	fs.StringVar(&brro.PluginDir, "pluginDir", "/data/bcs/nodeagent", "/data/bcs/nodeagent")
	fs.StringVar(&brro.ConfigPath, "configPath", "/data/bcs/nodeagent/config", "/data/bcs/nodeagent/config")
	fs.StringVar(&brro.CMNamespace, "cmNamespace", "nodeagent", "namespace to store nodeagent checkresult configmap")
	fs.IntVar(&brro.CopyNum, "copyNum", 5, "copyNum")
	fs.StringVar(&brro.Addr, "addr", "0.0.0.0:6216", "addr to bind listen")
}
