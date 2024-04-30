package nodeagent

import (
	"gopkg.in/yaml.v2"
	"k8s-cmdb/cmd/options"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/node"
	"k8s-cmdb/pkg/uploader"
	"k8s-cmdb/pkg/util"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func NAInteractive(nao *options.NodeAgentOptions) {
	if nao.ConfigPath == "" {
		klog.Fatalf("Must set ConfigPath, now is blank")
	} else if !filepath.IsAbs(nao.ConfigPath) && !filepath.IsLocal(nao.ConfigPath) {
		klog.Fatalf("%s is not a valid path", nao.ConfigPath)
	}

	naMain(nao)
}

func naMain(nao *options.NodeAgentOptions) {
	startTime := time.Now()

	//config default config
	_, err := os.Stat(nao.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			_, err = os.Stat(filepath.Dir(nao.ConfigPath))
			if os.IsNotExist(err) {
				// 创建文件路径
				err = os.MkdirAll(filepath.Dir(nao.ConfigPath), os.ModePerm)
				if err != nil {
					klog.Fatalf(err.Error())
					return
				}
			} else {
				klog.Fatalf(err.Error())
			}

			err = util.WriteConfigIfNotExist(nao.ConfigPath, `interval: 86400
pluginDir: /data/bcs/nodeagent`)
			if err != nil {
				klog.Fatalf(err.Error())
			}

			if nao.Plugins != "" {
				for _, pluginName := range strings.Split(nao.Plugins, ",") {
					filePath := filepath.Dir(nao.ConfigPath) + "/" + pluginName + ".conf"
					switch pluginName {
					case "customizationcheck":
						err = util.WriteConfigIfNotExist(filePath, `interval: 60
customizitions:
  - name: sa
    fileConfirmations:
      - name: ntpdate
        path: /usr/local/ieod-public/ntpdate/bin/ntpdate.cache
        content:
          - '{"conn_fail_flag":1}'
        expectedexist: true
        expectedcontentmatched: false`)
						if err != nil {
							klog.Fatalf(err.Error())
						}
					case "diskcheck":
						err = util.WriteConfigIfNotExist(filePath, `interval: 60`)
						if err != nil {
							klog.Fatalf(err.Error())
						}
					case "dnscheck":
						err = util.WriteConfigIfNotExist(filePath, `checkDomain:
  - tke.internal.tencentcloudapi.com
  - www.baidu.com
interval: 60`)
						if err != nil {
							klog.Fatalf(err.Error())
						}
					case "containercheck":
						err = util.WriteConfigIfNotExist(filePath, `interval: 60
sockPath: /var/run/docker.sock`)
						if err != nil {
							klog.Fatalf(err.Error())
						}
					case "hwcheck":
						err = util.WriteConfigIfNotExist(filePath, `interval: 60
logFileConfigs:
  - path: "/var/log/messages"
    keyWordList:
      - "mce: [Hardware Error]"
    rule: "mce error"`)
						if err != nil {
							klog.Fatalf(err.Error())
						}
					case "netcheck":
						err = util.WriteConfigIfNotExist(filePath, `LabelSelector: name=nodeagent
interval: 600`)
						if err != nil {
							klog.Fatalf(err.Error())
						}
					case "processcheck":
						err = util.WriteConfigIfNotExist(filePath, `interval: 60
processes:
  - name: kube-apiserver
    configFile: /etc/kubernetes/manifests/kube-apiserver.yaml
  - name: etcd
    configFile: /etc/kubernetes/manifests/etcd.yaml
  - name: kube-scheduler
    configFile: /etc/kubernetes/manifests/kube-scheduler.yaml
  - name: kube-controller-manager
    configFile: /etc/kubernetes/manifests/kube-controller-manager.yaml
  - name: kubelet
  - name: dockerd
    configFile: /etc/docker/daemon.json
  - name: lighthouse
    configFile: /etc/lighthouse/config.yaml
  - name: plugin-server
    configFile: /etc/plugin-server/config`)
						if err != nil {
							klog.Fatalf(err.Error())
						}
					}

				}
			}
		} else {
			klog.Fatalf("open file %s failed: %s", nao.ConfigPath, err.Error())
		}
	}

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
		// refresh config
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

	mm := metric.NewMetricManger()
	mm.RunPrometheusMetricsServer()

	if nao.Upstream == "cluster" {
		config, err := util.GetKubeConfig()
		if err != nil {
			klog.Fatalf(err.Error())
		}

		cs, _ := kubernetes.NewForConfig(config)

		ul := uploader.NewKubernetesUploader(util.GetNodeName(), cs, nao.CopyNum, nao.CMNamespace)
		nodeInfo = node.NewNodeInfo(naConfig, ul, nao.RunMode, nao.Plugins, nao.PluginDir)
		nodeInfo.Start()
	}

	// listening OS shutdown singal
	if nao.RunMode == "daemon" {
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
		<-signalChan
	}

}
