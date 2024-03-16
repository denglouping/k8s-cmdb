package util

import (
	"context"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v2"
	"k8s-cmdb/pkg/metric"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"os"
	"time"
)

func GetHostPath() string {
	hostPath := os.Getenv("HOST_PATH")
	if hostPath == "" {
		hostPath = "/"
	}

	return hostPath
}

func GetNodeName() string {
	name := os.Getenv("NODE_NAME")
	var err error
	if name == "" {
		name, err = os.Hostname()
		if err != nil {
			klog.Fatal(err.Error())
		}
	}

	return name
}

func GetCtx(duration time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	return ctx, cancel
}

func ReadConf(configFilePath string, obj interface{}) error {
	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(configFileBytes, obj); err != nil {
		if err = yaml.Unmarshal(configFileBytes, obj); err != nil {
			return fmt.Errorf("decode clustercheck config file %s failed, err %s", configFilePath, err.Error())
		}
	}

	return nil
}

func PrintCheckResult(labels []string, gaugeVecSetList []*metric.GaugeVecSet) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(append(labels, "value"))
	for _, gaugeVecSet := range gaugeVecSetList {
		table.Append(append(gaugeVecSet.Labels, "true"))
	}

	table.Render()
}

func GetKubeConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", "/root/.kube/config")
	}
	return config, err
}

func WriteConfigIfNotExist(filePath, content string) error {
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				return err
			}

			defer file.Close()

			// 写入文本信息
			_, err = file.WriteString(content)
			if err != nil {
				return err
			}

			return nil
		} else {
			return err
		}

	} else {
		return nil
	}
}
