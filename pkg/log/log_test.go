package log

import (
	"k8s.io/klog/v2"
	"testing"
	"time"
)

func TestGetDeviceStatus(t *testing.T) {
	l := NewLogFile("/var/log/messages")
	l.Start()
	for {
		select {
		case result, ok := <-l.LogChann:
			if !ok {
				klog.Info("Channel closed")
				return
			}
			klog.Info(result)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
