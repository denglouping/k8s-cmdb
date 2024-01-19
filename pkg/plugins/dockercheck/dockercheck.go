package dockercheck

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/process"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"os"
	"strings"
	"sync"
	"time"
)

// inspection item
// 1. if there are contaienrs that cannot be inspected(timeout)
// 2. if there are contaienrs that are in D status

var (
	containerStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "container_status",
		Help: "container_status",
	}, []string{"id", "name", "node", "status"})
	containerPorcessStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "container_process_status",
		Help: "container_process_status",
	}, []string{"id", "name", "node", "status"})
)

func init() {
	metric.Register(containerStatus)
	metric.Register(containerPorcessStatus)
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
				setValue("dockercheck", p.result)
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
	return "dockercheck"
}
func (p *Plugin) Check() {
	cli, err := GetDockerCli(util.GetHostPath() + p.opt.SockPath)
	if err != nil {
		klog.Errorf("%s get docker cli failed: %s, stop check", util.GetHostPath()+p.opt.SockPath, err.Error())
		return
	}

	nodeName := os.Getenv("NODE_NAME")

	containerStatusGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
	containerPidStatusGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
	ctx, _ := util.GetCtx(time.Second * 10)
	containerList, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		klog.Errorf("Get container list failed: %s, stop check", err.Error())
		return
	}

	// check container status
	containerStatusResult := make([][]string, 0, 0)
	containerProcessStatusResult := make([][]string, 0, 0)
	wg := sync.WaitGroup{}
	for _, container := range containerList {
		util.DefaultRoutinePool.Add(1)
		wg.Add(1)
		go func(client *client.Client, container types.Container) {
			defer func() {
				util.DefaultRoutinePool.Done()
				wg.Done()
			}()
			status, processStatus, err := ContainerCheck(cli, container.ID)
			if err != nil {
				klog.Errorf(err.Error())
			} else if container.State != status {
				klog.Infof("container id: %s,inspect: %s, state: %s", container.ID, status, container.State)
				status = "inconsistent"
			}

			if container.State != status {
				containerStatusGaugeVecSetList = append(containerStatusGaugeVecSetList, &metric.GaugeVecSet{
					Labels: []string{container.ID, strings.Join(container.Names, "_"), nodeName, status}, Value: float64(1),
				})
				containerStatusResult = append(containerStatusResult, []string{container.ID, strings.Join(container.Names, "_"), status})
			}

			if processStatus != "S" {
				containerPidStatusGaugeVecSetList = append(containerPidStatusGaugeVecSetList, &metric.GaugeVecSet{
					Labels: []string{container.ID, strings.Join(container.Names, "_"), nodeName, processStatus}, Value: float64(1),
				})
				containerProcessStatusResult = append(containerProcessStatusResult, []string{container.ID, strings.Join(container.Names, "_"), processStatus})
			}
		}(cli, container)

	}

	wg.Wait()
	metric.SetMetric(containerStatus, containerStatusGaugeVecSetList)
	metric.SetMetric(containerPorcessStatus, containerPidStatusGaugeVecSetList)

	p.result["containerStatus"] = containerStatusResult
	p.result["containerPidStatus"] = containerProcessStatusResult
}

func ContainerCheck(cli *client.Client, containerID string) (string, string, error) {
	containerInfo, err := GetContainerInfo(cli, containerID)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") {
			return "notfound", "", err
		} else {
			return "inspecterror", "", err
		}
	}

	if containerInfo.State.Pid == 0 {
		return containerInfo.State.Status, "pidis0", nil
	}

	pidStatus, err := GetContainerPIDStatus(containerInfo.State.Pid)
	if err != nil {
		return containerInfo.State.Status, "getprocesserror", err
	}

	return containerInfo.State.Status, pidStatus, nil
}

func GetDockerCli(sockPath string) (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(fmt.Sprintf("unix://%s", sockPath)))
	return cli, err

}

func GetContainerInfo(cli *client.Client, containerID string) (types.ContainerJSON, error) {
	ctx, _ := util.GetCtx(time.Minute)
	containerInfo, err := cli.ContainerInspect(ctx, containerID)
	return containerInfo, err
}

func GetContainerPIDStatus(pid int) (string, error) {
	processInfo, err := process.GetProcessInfo("", int32(pid))
	if err != nil {
		return "", err
	} else {
		return processInfo.Status, nil
	}
}
