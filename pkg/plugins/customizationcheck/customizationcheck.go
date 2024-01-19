package customizationcheck

import (
	"k8s-cmdb/pkg/customization"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

// inspection item
// 1. if there are contaienrs that cannot be inspected(timeout)
// 2. if there are contaienrs that are in D status

var (
//	containerStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
//		Name: "container_status",
//		Help: "container_status",
//	}, []string{"id", "name", "status"})
)

func init() {
	//metric.Register(containerStatus)
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
				klog.V(3).Infof("the former clustercheck didn't over, skip in this loop")
			}
			select {
			case result := <-p.stopChan:
				klog.V(3).Infof("stop plugin %s by signal %d", p.Name(), result)
				return
			case <-time.After(time.Duration(interval) * time.Second):
				setValue("customizationcheck", p.result)
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
	return "customizationcheck"
}
func (p *Plugin) Check() {

	result := make([]interface{}, 0, 0)
	for _, confirmation := range p.opt.CustomizeconList {
		for _, fc := range confirmation.FileConfirmations {
			fileConfirmation := customization.FileConfirmation{
				FileConfirmationConfig: fc,
			}
			err := fileConfirmation.Check()
			if err != nil {
				klog.Errorf(err.Error())
			}
			result = append(result, fileConfirmation)
		}
	}
	p.result["confirmation"] = result
}
