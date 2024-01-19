package util

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/json"
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
