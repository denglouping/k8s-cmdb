package hwcheck

import (
	"fmt"
	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/option"
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/log"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	deviceStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "device_status",
		Help: "device_status",
	}, []string{"id", "name", "node", "revision"})

	hardwareError = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hardware_error_count",
		Help: "hardware_error_count",
	}, []string{"type", "node"})
)

func init() {
	metric.Register(deviceStatus)
	metric.Register(hardwareError)
}

type Plugin struct {
	stopChan  chan int
	opt       *Options
	checkLock sync.Mutex
	result    map[string]interface{}
}

// Setup xxx
func (p *Plugin) Setup(configFilePath string, setValue func(key string, value interface{})) error {
	p.opt = &Options{}
	err := util.ReadConf(configFilePath, p.opt)
	if err != nil {
		return err
	}

	if err = p.opt.Validate(); err != nil {
		return err
	}

	p.result = make(map[string]interface{})

	interval := p.opt.Interval
	if interval == 0 {
		interval = 60
	}

	for index, logFileConfig := range p.opt.LogFileConfigList {
		logFileConfig.logFile = log.NewLogFile(util.GetHostPath() + logFileConfig.Path)
		if logFileConfig.logFile == nil {
			klog.Fatalf("%s no such file or directory", logFileConfig.Path)
		}

		logFileConfig.logFile.SetSearchKey(logFileConfig.KeyWordList)
		logFileConfig.logFile.Start()

		p.opt.LogFileConfigList[index] = logFileConfig
	}

	p.GetLogError()

	go func() {
		for {
			if p.checkLock.TryLock() {
				p.checkLock.Unlock()
				go p.Check()
			} else {
				klog.V(3).Infof("the former hwcheck didn't over, skip in this loop")
			}
			select {
			case result := <-p.stopChan:
				klog.V(3).Infof("stop plugin %s by signal %d", p.Name(), result)
				return
			case <-time.After(time.Duration(interval) * time.Second):
				setValue("hwcheck", p.result)
				continue
			}
		}
	}()

	return nil
}

// Stop xxx
func (p *Plugin) Stop() error {
	p.stopChan <- 1
	klog.Infof("plugin %s stopped", p.Name())
	return nil
}

// Name xxx
func (p *Plugin) Name() string {
	return "hwcheck"
}
func (p *Plugin) Check() {
	klog.Infof("start %s", p.Name())
	nodeName := os.Getenv("NODE_NAME")

	deviceList, err := GetDeviceStatus()
	if err != nil {
		klog.Errorf(err.Error())
		return
	}

	deviceStatusGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
	for _, device := range deviceList {
		deviceStatusGaugeVecSetList = append(deviceStatusGaugeVecSetList, &metric.GaugeVecSet{
			Labels: []string{device.Address, strings.Replace(device.Vendor.Name, " ", "_", -1), nodeName, device.Revision},
			Value:  float64(1),
		})
	}

	metric.SetMetric(deviceStatus, deviceStatusGaugeVecSetList)
}

func GetDeviceStatus() ([]*ghw.PCIDevice, error) {
	hostPath := util.GetHostPath()
	pciInfo, err := ghw.PCI(&option.Option{
		Chroot: &hostPath,
	})
	if err != nil {
		return nil, err
	}

	deviceList := make([]*ghw.PCIDevice, 0, 0)
	for _, device := range pciInfo.Devices {
		file, err := os.Open(fmt.Sprintf("/sys/bus/pci/devices/%s/config", device.Address))
		if err != nil {
			klog.Errorf("Error opening file:", err)
			continue
		}
		defer file.Close()

		revision := make([]byte, 1)
		_, err = file.ReadAt(revision, 8) // Revision ID is at offset 8
		if err != nil {
			klog.Errorf("Error reading file:", err)
			continue
		}

		if fmt.Sprintf("%x", revision[0]) == "ff" {
			device.Revision = fmt.Sprintf("%x", revision[0])
			deviceList = append(deviceList, device)
		}
	}

	return deviceList, nil
}

func (p *Plugin) GetLogError() {
	nodeName := os.Getenv("NODE_NAME")
	for _, logFileConfig := range p.opt.LogFileConfigList {
		go func(logFile LogFileConfig, nodeName string) {
			for {
				select {
				case result, ok := <-logFile.logFile.LogChann:
					if !ok {
						klog.Info("Channel closed")
						return
					}

					for _, key := range logFile.KeyWordList {
						if strings.Contains(result, key) {
							hardwareError.WithLabelValues(logFile.Rule, nodeName).Add(1)
						}
					}
				default:
					time.Sleep(5 * time.Millisecond)
				}
			}
		}(logFileConfig, nodeName)

	}
}
