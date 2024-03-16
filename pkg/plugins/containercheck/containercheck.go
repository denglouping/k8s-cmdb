package containercheck

import (
	"context"
	"fmt"
	"github.com/containerd/containerd/namespaces"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/process"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// inspection item
// 1. if there are contaienrs that cannot be inspected(timeout)
// 2. if there are contaienrs that are in D status

var (
	containerStatusLabels        = []string{"id", "name", "node", "status"}
	containerPorcessStatusLabels = []string{"id", "name", "node", "status"}
	containerStatus              = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "container_status",
		Help: "container_status",
	}, containerStatusLabels)
	containerPorcessStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "container_process_status",
		Help: "container_process_status",
	}, containerPorcessStatusLabels)

	sockPaths = []string{
		"/run/docker.sock",
		"/run/containerd/containerd.sock",
	}
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
	ready     bool
}

type ContainerStatusResult struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	NodeName string `yaml:"nodeName"`
	Status   string `yaml:"status"`
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
					klog.V(3).Infof("the former dockercheck didn't over, skip in this loop")
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
	return "containercheck"
}
func (p *Plugin) Check() {
	containerStatusResultList := make([]ContainerStatusResult, 0, 0)
	containerProcessStatusResultList := make([]ContainerStatusResult, 0, 0)

	containerStatusGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)
	containerPidStatusGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)

	nodeName := util.GetNodeName()

	defer func() {
		if !p.ready {
			p.ready = true
		}
	}()

	socketPath := p.opt.SockPath
	if socketPath == "" {
		for _, socketPath = range sockPaths {
			_, err := os.Stat(socketPath)
			if err != nil {
				break
			}
		}
	}

	if strings.Contains(socketPath, "docker.sock") {
		cli, err := GetDockerCli(util.GetHostPath() + p.opt.SockPath)
		if err != nil {
			klog.Errorf("%s get docker cli failed: %s, stop check", util.GetHostPath()+p.opt.SockPath, err.Error())
			return
		}

		ctx, _ := util.GetCtx(time.Second * 10)
		containerList, err := cli.ContainerList(ctx, types.ContainerListOptions{})
		if err != nil {
			klog.Errorf("Get container list failed: %s, stop check", err.Error())
			return
		}

		// check container status
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
					containerStatusResultList = append(containerStatusResultList, ContainerStatusResult{
						ID:       container.ID,
						NodeName: nodeName,
						Status:   status,
						Name:     strings.Join(container.Names, "_"),
					})
				}

				if processStatus != "S" {
					containerPidStatusGaugeVecSetList = append(containerPidStatusGaugeVecSetList, &metric.GaugeVecSet{
						Labels: []string{container.ID, strings.Join(container.Names, "_"), nodeName, processStatus}, Value: float64(1),
					})
					containerProcessStatusResultList = append(containerProcessStatusResultList, ContainerStatusResult{
						ID:       container.ID,
						NodeName: nodeName,
						Status:   processStatus,
						Name:     strings.Join(container.Names, "_"),
					})
				}
			}(cli, container)
		}
		wg.Wait()

		metric.SetMetric(containerStatus, containerStatusGaugeVecSetList)
		metric.SetMetric(containerPorcessStatus, containerPidStatusGaugeVecSetList)

		p.result["containerStatus"] = containerStatusResultList
		p.result["containerPidStatus"] = containerProcessStatusResultList
	} else if strings.Contains(p.opt.SockPath, "containerd.sock") {
		ctx := context.Background()

		// 连接到 containerd
		cli, err := containerd.New(util.GetHostPath() + p.opt.SockPath)
		if err != nil {
			klog.Errorf("%s get containerd cli failed: %s, stop check", util.GetHostPath()+p.opt.SockPath, err.Error())
			return
		}

		defer cli.Close()

		ctx, _ = util.GetCtx(time.Second * 10)

		namespaceList, err := cli.NamespaceService().List(context.Background())
		if err != nil {
			log.Fatal(err)
		}

		for _, ns := range namespaceList {
			ctx = namespaces.WithNamespace(context.Background(), ns)

			containerList, err := cli.Containers(ctx)
			if err != nil {
				klog.Errorf("Get container list failed: %s, stop check", err.Error())
				return
			}

			// check container status
			wg := sync.WaitGroup{}
			for _, container := range containerList {
				util.DefaultRoutinePool.Add(1)
				wg.Add(1)
				go func(client *containerd.Client, container containerd.Container) {
					defer func() {
						util.DefaultRoutinePool.Done()
						wg.Done()
					}()

					info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
					if err != nil {
						klog.Errorf(err.Error())
						return
					}

					task, err := container.Task(ctx, nil)
					if err != nil {
						if !errdefs.IsNotFound(err) {
							klog.Errorf(err.Error())
						}

						return
					}
					pid := task.Pid()
					processStatus, err := GetContainerPIDStatus(int(pid))
					if err != nil {
						klog.Errorf(err.Error())
						return
					}

					podName := ""
					if name, ok := info.Labels["io.kubernetes.pod.name"]; ok {
						podName = name
					}

					if processStatus != "S" && processStatus != "I" && processStatus != "R" && processStatus != "T" {
						containerPidStatusGaugeVecSetList = append(containerPidStatusGaugeVecSetList, &metric.GaugeVecSet{
							Labels: []string{container.ID(), podName, nodeName, processStatus}, Value: float64(1),
						})
						containerProcessStatusResultList = append(containerProcessStatusResultList, ContainerStatusResult{
							ID:       container.ID(),
							NodeName: nodeName,
							Status:   processStatus,
							Name:     podName,
						})
					}

				}(cli, container)
			}
			wg.Wait()
		}

		metric.SetMetric(containerStatus, containerStatusGaugeVecSetList)
		metric.SetMetric(containerPorcessStatus, containerPidStatusGaugeVecSetList)

		p.result["containerPidStatus"] = containerProcessStatusResultList
	}
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
	processInfo, err := process.GetProcess(int32(pid))
	if err != nil {
		return "", err
	} else {
		return processInfo.Status()
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
