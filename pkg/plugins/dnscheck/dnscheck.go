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
package dnscheck

import (
	"context"
	"fmt"
	"k8s-cmdb/pkg/metric"
	"k8s-cmdb/pkg/util"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes"
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
	dnsAvailability = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dns_availability",
		Help: "dns_availability, 1 means OK",
	}, []string{"type", "node", "status"})

	dnsLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dns_latency",
		Help:    "dns_latency",
		Buckets: []float64{0.001, 0.01, 0.1, 0.2, 0.4, 0.8, 1.6, 3.2},
	}, []string{})
)

func init() {
	metric.Register(dnsAvailability)
	metric.Register(dnsLatency)
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
	return "dnscheck"
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
			klog.Errorf("dnscheck failed: %s, stack: %v\n", r, string(debug.Stack()))
		}
	}()

	dnsStatusGaugeVecSetList := make([]*metric.GaugeVecSet, 0, 0)

	ctx, _ := util.GetCtx(time.Second * 10)
	status := p.checkDNS(ctx, p.opt.CheckDomain, "")
	dnsStatusGaugeVecSetList = append(dnsStatusGaugeVecSetList, &metric.GaugeVecSet{
		Labels: []string{"pod", nodeName, status},
		Value:  float64(1),
	})

	ctx, _ = util.GetCtx(time.Second * 10)
	status = p.checkDNS(ctx, p.opt.CheckDomain, fmt.Sprintf("%s/etc/resolv.conf", util.GetHostPath()))
	dnsStatusGaugeVecSetList = append(dnsStatusGaugeVecSetList, &metric.GaugeVecSet{
		Labels: []string{"host", nodeName, status},
		Value:  float64(1),
	})

	metric.SetMetric(dnsAvailability, dnsStatusGaugeVecSetList)
}

func (p *Plugin) checkDNS(ctx context.Context, domainList []string, path string) string {
	status := "ok"
	select {
	case <-ctx.Done():
		status = "timeout"
		break

	default:
		ipList := make([]string, 0, 0)

		if path != "" {
			config, _ := dns.ClientConfigFromFile(path)
			ipList = config.Servers
		} else {
			ep, err := p.clientSet.CoreV1().Endpoints("kube-system").Get(context.Background(), "kube-dns", v1.GetOptions{ResourceVersion: "0"})
			if err != nil {
				klog.Errorf(err.Error())
				status = "getepfailed"
			}

			for _, subset := range ep.Subsets {
				for _, address := range subset.Addresses {
					ipList = append(ipList, address.IP)
				}
			}
		}

		if len(ipList) > 0 {
			for ip := 0; ip < len(ipList); ip++ {
				r, err := createResolver(ipList[ip])
				if err != nil {
					klog.Errorf(err.Error())
					status = "setresolverfailed"
				}

				for _, domain := range domainList {
					latency, err := dnsLookup(r, domain)
					if err != nil {
						klog.Errorf(err.Error())
						status = "resolvefailed"
					}

					dnsLatency.WithLabelValues().Observe(float64(latency) / float64(time.Second))
				}
			}
		} else {
			status = "noserver"
			klog.Errorf("No available dns server")
		}
	}

	return status
}

func createResolver(ip string) (*net.Resolver, error) {
	r := &net.Resolver{}
	// if we're supplied a null string, return an error
	if len(ip) < 1 {
		return r, fmt.Errorf("Need a valid ip to create Resolver")
	}
	// attempt to create the resolver based on the string
	r = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address2 string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Millisecond * time.Duration(10000),
			}
			return d.DialContext(ctx, "udp", ip+":53")
		},
	}
	return r, nil
}

func dnsLookup(r *net.Resolver, host string) (time.Duration, error) {
	start := time.Now()
	addrs, err := r.LookupHost(context.Background(), host)
	if err != nil {
		errorMessage := "DNS Status check determined that " + host + " is DOWN: " + err.Error()
		return 0, fmt.Errorf(errorMessage)
	}

	if len(addrs) == 0 {
		return 0, fmt.Errorf("No host was found")
	}

	return time.Since(start), nil
}
