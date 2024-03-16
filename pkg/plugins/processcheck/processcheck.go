package processcheck

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/process"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"os"
	"path"
	"sync"
	"time"
)

var (
	processStatusLabels = []string{"name", "status", "node"}
	processStatus       = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "process_status",
		Help: "process_status",
	}, processStatusLabels)
	abnormalProcessStatusMap = make(map[int32]process.ProcessStatus)
)

func init() {
	metric.Register(processStatus)
}

type Plugin struct {
	stopChan  chan int
	opt       *Options
	checkLock sync.Mutex
	result    map[string]interface{}
	ready     bool
}

type ProcessCheckResult struct {
	Node   string `yaml:"node"`
	Status string `yaml:"status"`
	Name   string `yaml:"name"`
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

	if runMode == "daemon" {
		go func() {
			for {
				if p.checkLock.TryLock() {
					p.checkLock.Unlock()
					go p.Check()
				} else {
					klog.V(3).Infof("the former %s didn't over, skip in this loop", p.Name())
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
	return "processcheck"
}
func (p *Plugin) Check() {
	if p.opt.Processes == nil {
		return
	}
	nodeName := util.GetNodeName()

	processInfoList := make([]*process.ProcessInfo, 0, 0)
	processGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
	// check process and generate process configuration
	for _, pcc := range p.opt.Processes {
		if pcc.ConfigFile != "" {
			_, err := os.Stat(path.Join(os.Getenv("HOST_PATH"), pcc.ConfigFile))
			if err != nil {
				if os.IsNotExist(err) {
					klog.Errorf("%s configfile %s not exist, continue", pcc.Name, pcc.ConfigFile)
					//processGaugeVecSetList = append(processGaugeVecSetList, &metric.GaugeVecSet{
					//	Labels: []string{pcc.Name, "notexist"}, Value: float64(1),
					//})
				} else {
					klog.Errorf(err.Error())
				}
				continue
			}
		}

		processInfo, err := process.GetProcessInfo(pcc.Name, 0)
		if err != nil {
			klog.Errorf("Get process %s info failed: %s", pcc.Name, err.Error())
			continue
		}
		processInfoList = append(processInfoList, processInfo)

		processGaugeVecSetList = append(processGaugeVecSetList, &metric.GaugeVecSet{
			Labels: []string{pcc.Name, processInfo.Status, nodeName}, Value: float64(1),
		})
	}

	p.result["processInfo"] = processInfoList

	processStatusList, err := process.GetProcessStatus()
	if err != nil {
		klog.Errorf("Get process status failed: %s", err.Error())
	}

	newAbnormalProcessStatusMap := make(map[int32]process.ProcessStatus)
	for _, pstatus := range processStatusList {
		if pstatus.Status != "S" && pstatus.Status != "I" && pstatus.Status != "R" && pstatus.Status != "T" {
			newAbnormalProcessStatusMap[pstatus.Pid] = pstatus

			if abnormalProcessStatus, ok := abnormalProcessStatusMap[pstatus.Pid]; ok && pstatus.Status == "D" {
				if abnormalProcessStatus.Pid == pstatus.Pid && abnormalProcessStatus.CreateTime == pstatus.CreateTime {

					// cputime didn't increase, means process stayed in D status in this interval
					if abnormalProcessStatus.CpuTime == pstatus.CpuTime {
						processGaugeVecSetList = append(processGaugeVecSetList, &metric.GaugeVecSet{
							Labels: []string{pstatus.Name, pstatus.Status, nodeName}, Value: float64(1),
						})
					}

				}
			} else if pstatus.Status != "D" {
				processGaugeVecSetList = append(processGaugeVecSetList, &metric.GaugeVecSet{
					Labels: []string{pstatus.Name, pstatus.Status, nodeName}, Value: float64(1),
				})
			}
		}
	}

	abnormalProcessStatusMap = newAbnormalProcessStatusMap
	p.result["processStatus"] = abnormalProcessStatusMap
	// return result
	metric.SetMetric(processStatus, processGaugeVecSetList)

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
