package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
	"net/http"
	"runtime/debug"
	"strings"
)

type GaugeVecSet struct {
	Labels []string
	Value  float64
}

type MetricManger struct {
}

func (mm *MetricManger) RunPrometheusMetricsServer() {
	// register prometheus server
	http.Handle("/metrics", promhttp.Handler())
	addr := "0.0.0.0:6216"
	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			klog.Fatalf("Failed to listen and serve metric server, err %s", err.Error())
		}
	}()
	klog.Infof("run prometheus server ok")
}

func Register(collector prometheus.Collector) {
	prometheus.MustRegister(collector)
}

func NewMetricManger() *MetricManger {
	return &MetricManger{}
}

func SetMetric(metricVec *prometheus.GaugeVec, gaugeVecSetList []*GaugeVecSet) {
	metricVec.Reset()

	for _, gaugeVecSet := range gaugeVecSetList {
		metricVec.WithLabelValues(gaugeVecSet.Labels...).Set(gaugeVecSet.Value)

		defer func() {
			if r := recover(); r != nil {
				klog.Errorf("SetMetric failed: %s, stack: %v\n", r, string(debug.Stack()))
				// klog.Errorf("SetMetric failed: %s", gaugeVecSet)
				for _, gaugeVecSet1 := range gaugeVecSetList {
					if gaugeVecSet1 != nil {
						klog.Errorf("SetMetric failed: %s", strings.Join(gaugeVecSet1.Labels, ";"))
						break
					}
				}
			}
		}()
	}

}
