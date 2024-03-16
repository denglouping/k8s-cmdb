package customizationcheck

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/customization"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

// inspection item
// 1. if there are contaienrs that cannot be inspected(timeout)
// 2. if there are contaienrs that are in D status

var (
	fileCheckResultLabels = []string{"name", "result", "exist", "match", "node"}
	fileCheckResult       = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "file_check_result",
		Help: "file_check_result",
	}, fileCheckResultLabels)
)

func init() {
	metric.Register(fileCheckResult)
}

type Plugin struct {
	stopChan  chan int
	opt       *Options
	checkLock sync.Mutex
	result    interface{}
	ready     bool
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

	// run as daemon
	if runMode == "daemon" {
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
	return "customizationcheck"
}
func (p *Plugin) Check() {
	result := make([]interface{}, 0, 0)
	nodeName := util.GetNodeName()
	fileCheckGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
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
			fileCheckGaugeVecSetList = append(fileCheckGaugeVecSetList, &metric.GaugeVecSet{
				Labels: []string{fileConfirmation.Name, fmt.Sprintf("%t", fileConfirmation.Result), fmt.Sprintf("%t", fileConfirmation.Exist), fmt.Sprintf("%t", fileConfirmation.ContentMatched), nodeName},
				Value:  float64(1),
			})
		}
	}
	p.result = result
	metric.SetMetric(fileCheckResult, fileCheckGaugeVecSetList)

	if !p.ready {
		p.ready = true
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
