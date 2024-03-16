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
	deviceStatusLabels  = []string{"id", "name", "node", "revision"}
	hardwareErrorLabels = []string{"type", "node"}
	deviceStatus        = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "device_status",
		Help: "device_status",
	}, deviceStatusLabels)

	hardwareError = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hardware_error_count",
		Help: "hardware_error_count",
	}, hardwareErrorLabels)
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
	ready     bool
}

type DeviceStatusResult struct {
	Address  string `yaml:"address"`
	Name     string `yaml:"name"`
	NodeName string `yaml:"nodeName"`
	Revision string `yaml:"revision"`
}

type HardwareErrorResult struct {
	Rule     string `yaml:"rule"`
	NodeName string `yaml:"nodeName"`
	Count    int    `yaml:"count"`
}

// Setup xxx
func (p *Plugin) Setup(configFilePath string, runMode string) error {
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

		p.opt.LogFileConfigList[index] = logFileConfig
	}

	// run as daemon
	if runMode == "daemon" {
		//p.GetLogError()
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
					continue
				}
			}
		}()
	} else if runMode == "once" {
		p.Check()
	}

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
	nodeName := util.GetNodeName()

	deviceList, err := GetDeviceStatus()
	if err != nil {
		klog.Errorf(err.Error())
		return
	}

	deviceStatusResultList := make([]DeviceStatusResult, 0, 0)
	hardwareErrorResultList := make([]HardwareErrorResult, 0, 0)
	deviceStatusGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
	for _, device := range deviceList {
		deviceStatusGaugeVecSetList = append(deviceStatusGaugeVecSetList, &metric.GaugeVecSet{
			Labels: []string{device.Address, strings.Replace(device.Vendor.Name, " ", "_", -1), nodeName, device.Revision},
			Value:  float64(1),
		})
		deviceStatusResultList = append(deviceStatusResultList, DeviceStatusResult{device.Address, strings.Replace(device.Vendor.Name, " ", "_", -1), nodeName, device.Revision})
	}

	metric.SetMetric(deviceStatus, deviceStatusGaugeVecSetList)

	hardwareErrorGVSList := make([]*metric.GaugeVecSet, 0, 0)
	for _, logFileConfig := range p.opt.LogFileConfigList {
		result, err := logFileConfig.logFile.CheckNewEntriesOnce()
		if err != nil {
			klog.Errorf(err.Error())
		} else {

			for _, key := range logFileConfig.KeyWordList {
				count := 0
				for _, line := range result {
					if strings.Contains(line, key) {
						count++
						hardwareError.WithLabelValues(logFileConfig.Rule, nodeName).Add(1)
						break
					}
				}

				hardwareErrorGVSList = append(hardwareErrorGVSList, &metric.GaugeVecSet{
					Labels: []string{logFileConfig.Rule, nodeName},
					Value:  float64(count),
				})
				hardwareErrorResultList = append(hardwareErrorResultList, HardwareErrorResult{logFileConfig.Rule, nodeName, count})
			}
		}
	}
	metric.SetMetric(hardwareError, hardwareErrorGVSList)

	p.result["deviceStatus"] = deviceStatusResultList
	p.result["hardwareError"] = hardwareErrorResultList

	if !p.ready {
		p.ready = true
	}
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
	nodeName := util.GetNodeName()
	for _, logFileConfig := range p.opt.LogFileConfigList {
		logFileConfig.logFile.Start()
		go func(logFile LogFileConfig, nodeName string) {
			for {
				select {
				case result, ok := <-logFile.logFile.LogChann:
					if !ok {
						klog.Error("Channel closed")
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

func (p *Plugin) Ready() bool {
	return p.ready
}

func (p *Plugin) GetResult() interface{} {
	return p.result
}

func (p *Plugin) Execute() {
	p.Check()
}
