package options

import (
	"github.com/spf13/pflag"
)

type NodeAgentOptions struct {
	HostPath   string
	Upstream   string
	ConfigPath string
	CopyNum    int
	Plugins    string
}

func NewNodeAgentOptions() *NodeAgentOptions {
	return &NodeAgentOptions{}
}

func (brro *NodeAgentOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&brro.Plugins, "plugins", "customizationcheck,dnscheck,dockercheck,hwcheck,processcheck,diskcheck,netcheck", "plugins")

	fs.StringVar(&brro.HostPath, "hostPath", "/host", "hostPath")
	fs.StringVar(&brro.Upstream, "upstream", "incluster", "incluster, mysql")
	fs.StringVar(&brro.ConfigPath, "configPath", "/data/bcs/nodeagent/config", "/data/bcs/nodeagent/config")
	fs.IntVar(&brro.CopyNum, "copyNum", 5, "copyNum")
}
