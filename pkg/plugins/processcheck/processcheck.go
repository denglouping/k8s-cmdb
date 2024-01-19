package processcheck

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/process"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

// inspection item
// 1. if there are contaienrs that cannot be inspected(timeout)
// 2. if there are contaienrs that are in D status

var (
	processStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "process_status",
		Help: "process_status",
	}, []string{"name", "status"})
)

func init() {
	metric.Register(processStatus)
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

	go func() {
		for {
			if p.checkLock.TryLock() {
				p.checkLock.Unlock()
				go p.Check()
			} else {
				klog.V(3).Infof("the former processcheck didn't over, skip in this loop")
			}
			select {
			case result := <-p.stopChan:
				klog.V(3).Infof("stop plugin %s by signal %d", p.Name(), result)
				return
			case <-time.After(time.Duration(interval) * time.Second):
				setValue("processcheck", p.result)
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
	return "processcheck"
}
func (p *Plugin) Check() {
	if p.opt.Processes == nil {
		return
	}

	processInfoList := make([]*process.ProcessInfo, 0, 0)
	processGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
	for _, pName := range p.opt.Processes {
		processInfo, err := process.GetProcessInfo(pName, 0)
		if err != nil {
			klog.Errorf(err.Error())
			processGaugeVecSetList = append(processGaugeVecSetList, &metric.GaugeVecSet{
				Labels: []string{pName, "error"}, Value: float64(1),
			})
			continue
		}
		processInfoList = append(processInfoList, processInfo)

		processGaugeVecSetList = append(processGaugeVecSetList, &metric.GaugeVecSet{
			Labels: []string{pName, processInfo.Status}, Value: float64(1),
		})
	}

	p.result["processes"] = processInfoList
}
