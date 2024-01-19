package main

import (
	"github.com/spf13/pflag"
	"k8s-cmdb/cmd/nodeagent"
	"k8s-cmdb/cmd/options"
	_ "k8s-cmdb/pkg/plugins/customizationcheck"
	_ "k8s-cmdb/pkg/plugins/diskcheck"
	_ "k8s-cmdb/pkg/plugins/dnscheck"
	_ "k8s-cmdb/pkg/plugins/dockercheck"
	_ "k8s-cmdb/pkg/plugins/hwcheck"
	_ "k8s-cmdb/pkg/plugins/netcheck"
	_ "k8s-cmdb/pkg/plugins/processcheck"
	_ "net/http/pprof" // pprof
)

func main() {
	na := options.NewNodeAgentOptions()
	na.AddFlags(pflag.CommandLine)

	pflag.Parse()

	nodeagent.NAInteractive(na)
}
