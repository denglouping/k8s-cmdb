package nodeagent

import (
	"gopkg.in/yaml.v2"
	"k8s-cmdb/cmd/options"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/node"
	"k8s-cmdb/pkg/uploader"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func NAInteractive(nao *options.NodeAgentOptions) {
	termCh := make(chan error, 1)
	defer close(termCh)

	if nao.ConfigPath == "" {
		klog.Fatalf("Must set ConfigPath, now is blank")
	} else if !filepath.IsAbs(nao.ConfigPath) && !filepath.IsLocal(nao.ConfigPath) {
		klog.Fatalf("%s is not a valid path", nao.ConfigPath)
	}

	naMain(nao, termCh)
}

func naMain(nao *options.NodeAgentOptions, termCh <-chan error) {
	// get Config
	startTime := time.Now()
	configFileBytes, err := os.ReadFile(nao.ConfigPath)
	if err != nil {
		klog.Fatalf("read config file %s failed, err %s", nao.ConfigPath, err.Error())
	}

	naConfig := &node.NodeAgentConfig{}
	if err = json.Unmarshal(configFileBytes, naConfig); err != nil {
		if err = yaml.Unmarshal(configFileBytes, naConfig); err != nil {
			klog.Fatalf("decode config file %s failed, err %s", nao.ConfigPath, err.Error())
		}
	}
	go func() {
		for {
			time.Sleep(time.Minute)
			fileInfo, err := os.Stat(nao.ConfigPath)
			if err != nil {
				klog.Errorf(err.Error())
				continue
			}
			if fileInfo.ModTime().After(startTime) {
				configFileBytes, err = os.ReadFile(nao.ConfigPath)
				if err != nil {
					klog.Errorf("read config file %s failed, err %s", nao.ConfigPath, err.Error())
					continue
				}

				naConfig = &node.NodeAgentConfig{}
				if err = json.Unmarshal(configFileBytes, naConfig); err != nil {
					if err = yaml.Unmarshal(configFileBytes, naConfig); err != nil {
						klog.Errorf("decode config file %s failed, err %s", nao.ConfigPath, err.Error())
						continue
					}
				}
			}
		}
	}()

	var nodeInfo node.NodeInfo

	if nao.Upstream == "incluster" {
		config, err := rest.InClusterConfig()
		if err != nil {
			klog.Fatalf(err.Error())
		}
		cs, _ := kubernetes.NewForConfig(config)

		ul := uploader.NewKubernetesUploader(os.Getenv("NODE_NAME"), cs, nao.CopyNum)
		nodeInfo = node.NewNodeInfo(naConfig, ul, nao.Plugins)
		nodeInfo.Start()
	}

	mm := metric.NewMetricManger()
	mm.RunPrometheusMetricsServer()

	klog.Info("start work...")

	// listening OS shutdown singal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

}
