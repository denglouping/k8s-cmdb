/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.,
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under,
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package dnscheck
package netcheck

import (
	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/util"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"os"
	"os/exec"
	"runtime/debug"
	"sync"
	"time"
)

// Plugin xxx
type Plugin struct {
	stopChan  chan int
	opt       *Options
	checkLock sync.Mutex
	clientSet *kubernetes.Clientset
	dnsLock   sync.Mutex
}

var (
	netAvailability = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "net_availability",
		Help: "net_availability, 1 means OK",
	}, []string{"node", "status"})
)

func init() {
	metric.Register(netAvailability)
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

	p.stopChan = make(chan int)
	interval := p.opt.Interval
	if interval == 0 {
		interval = 60
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf(err.Error())
	}
	cs, _ := kubernetes.NewForConfig(inClusterConfig)
	if err != nil {
		klog.Fatalf("dnscheck get incluster config failed, only can run as incluster mode")
	}

	p.clientSet = cs

	go func() {
		for {
			if p.checkLock.TryLock() {
				p.checkLock.Unlock()
				go p.Check()
			} else {
				klog.V(3).Infof("the former dnscheck didn't over, skip in this loop")
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
	return "netcheck"
}

// Check xxx
func (p *Plugin) Check() {
	p.checkLock.Lock()
	klog.Infof("start %s", p.Name())
	defer func() {
		klog.Infof("end %s", p.Name())
		p.checkLock.Unlock()
	}()

	nodeName := os.Getenv("NODE_NAME")

	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("netcheck failed: %s, stack: %v\n", r, string(debug.Stack()))
		}
	}()

	podList, err := p.getPodList()
	if err != nil {
		klog.Errorf(err.Error())
		return
	}

	wg := sync.WaitGroup{}
	status := "ok"
	for _, pod := range podList {
		if status != "ok" {
			break
		}

		if pod.Status.Phase != "Running" {
			continue
		}

		if pod.Status.PodIP == "" {
			continue
		}

		util.DefaultRoutinePool.Add(1)
		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
				util.DefaultRoutinePool.Done()
			}()

			if status != "ok" {
				return
			}
			pingStatus := PINGCheck(pod.Status.PodIP)
			if pingStatus != "ok" {
				status = pingStatus
			}

		}()

	}

	wg.Wait()

	gaugeVecSet := &metric.GaugeVecSet{
		Labels: []string{nodeName, "ok"},
		Value:  float64(1),
	}
	metric.SetMetric(netAvailability, []*metric.GaugeVecSet{gaugeVecSet})

}

func (p *Plugin) getPodList() ([]corev1.Pod, error) {
	ctx, _ := util.GetCtx(time.Second * 10)
	podList, err := p.clientSet.CoreV1().Pods(os.Getenv("NAMESPACE")).List(ctx, v1.ListOptions{
		ResourceVersion: "0",
		LabelSelector:   "name=nodeagent",
	})

	if err != nil {
		return nil, err
	}

	return podList.Items, err
}

func PINGCheck(ip string) (status string) {
	pingCmd := exec.Command("ping", "-c1", "-W1", ip)
	output, err := pingCmd.CombinedOutput()
	if err != nil {
		status = "pingfailed"
		klog.Error(string(output))
		return
	}

	status = "ok"
	return
}

func PINGCheckbak(ip string) (status string) {
	pinger, err := ping.NewPinger(ip)
	if err != nil {
		klog.Errorf("Error creating pinger: %v\n", err)
		status = "error"
		return
	}

	pinger.Timeout = 5 * time.Second
	pinger.Count = 1 // 发送一个ping包
	pinger.OnRecv = func(pkt *ping.Packet) {
		//klog.Infof("%d bytes from %s: icmp_seq=%d time=%v\n",
		//	pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
	}

	var pingStat *ping.Statistics
	pinger.OnFinish = func(stats *ping.Statistics) {
		pingStat = stats
		//klog.Infof("\n--- %s ping statistics ---\n", stats.Addr)
		//klog.Infof("%d packets transmitted, %d packets received, %v%% packet loss\n",
		//	stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
		//klog.Infof("round-trip min/avg/max/stddev = %v/%v/%v/%v\n",
		//	stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt)
	}

	err = pinger.Run()
	if err != nil {
		status = "error"
		klog.Errorf("Error running pinger: %v\n", err)
		if pingStat != nil {
			klog.Infof("\n--- %s ping statistics ---\n", pingStat.Addr)
			klog.Infof("%d packets transmitted, %d packets received, %v%% packet loss\n",
				pingStat.PacketsSent, pingStat.PacketsRecv, pingStat.PacketLoss)
			klog.Infof("round-trip min/avg/max/stddev = %v/%v/%v/%v\n",
				pingStat.MinRtt, pingStat.AvgRtt, pingStat.MaxRtt, pingStat.StdDevRtt)
		}

		return
	}

	status = "ok"
	return
}
