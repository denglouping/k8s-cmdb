package main

import (
	"github.com/spf13/pflag"
	"k8s-cmdb/cmd/nodeagent"
	"k8s-cmdb/cmd/options"
	_ "k8s-cmdb/pkg/plugins/containercheck"
	_ "k8s-cmdb/pkg/plugins/customizationcheck"
	_ "k8s-cmdb/pkg/plugins/diskcheck"
	_ "k8s-cmdb/pkg/plugins/dnscheck"
	_ "k8s-cmdb/pkg/plugins/hwcheck"
	_ "k8s-cmdb/pkg/plugins/netcheck"
	_ "k8s-cmdb/pkg/plugins/processcheck"
	_ "k8s-cmdb/pkg/plugins/timecheck"
	"k8s.io/apimachinery/pkg/util/rand"
	_ "net/http/pprof" // pprof
	"time"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	na := options.NewNodeAgentOptions()
	na.AddFlags(pflag.CommandLine)

	pflag.Parse()

	nodeagent.NAInteractive(na)
}
