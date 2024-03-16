package timecheck

import (
	"github.com/beevik/ntp"
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/util"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Plugin xxx
type Plugin struct {
	stopChan  chan int
	opt       *Options
	checkLock sync.Mutex
	result    interface{}
	ready     bool
}

type NTPCheckResult struct {
	Node   string `yaml:"node"`
	Status string `yaml:"status"`
	Offset float64
}

var (
	ntpAvailabilityLabels = []string{"node", "status"}
	ntpAvailability       = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ntp_availability",
		Help: "ntp_availability, 1 means OK",
	}, ntpAvailabilityLabels)
)

func init() {
	metric.Register(ntpAvailability)
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

	p.stopChan = make(chan int)
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
	p.checkLock.Lock()
	p.stopChan <- 1
	klog.Infof("plugin %s stopped", p.Name())
	p.checkLock.Unlock()
	return nil
}

// Name xxx
func (p *Plugin) Name() string {
	return "timecheck"
}

// Check xxx
func (p *Plugin) Check() {
	p.checkLock.Lock()
	klog.Infof("start %s", p.Name())
	defer func() {
		klog.Infof("end %s", p.Name())
		p.checkLock.Unlock()
	}()

	nodeName := util.GetNodeName()

	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("ntpcheck failed: %s, stack: %v\n", r, string(debug.Stack()))
		}
	}()

	gaugeVecSet := &metric.GaugeVecSet{}
	result := NTPCheckResult{Node: nodeName}

	servers := strings.Split(p.opt.TimeServers, ",")
	offset, err := GetTimeOffset(servers[rand.Intn(len(servers)-1)])
	if err != nil {
		klog.Errorf(err.Error())
		gaugeVecSet = &metric.GaugeVecSet{
			Labels: []string{nodeName, "error"},
			Value:  0,
		}
		result.Status = "error"
	} else {
		gaugeVecSet = &metric.GaugeVecSet{
			Labels: []string{nodeName, "ok"},
			Value:  float64(offset) / float64(time.Second),
		}
		result.Status = "ok"
		result.Offset = float64(offset) / float64(time.Second)
	}

	metric.SetMetric(ntpAvailability, []*metric.GaugeVecSet{gaugeVecSet})
	p.result = result

	if !p.ready {
		p.ready = true
	}

}

func GetTimeOffset(timeserver string) (time.Duration, error) {
	localTime := time.Now()

	ntpServer := timeserver

	ntpTime, err := ntp.Time(ntpServer)
	if err != nil {
		return 0, err
	}

	diff := ntpTime.Sub(localTime)

	return diff, nil
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
