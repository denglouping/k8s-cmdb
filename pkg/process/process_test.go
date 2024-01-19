package process

import (
	"github.com/moby/sys/mountinfo"
	"k8s.io/klog/v2"
	"os"
	"testing"
)

func TestMount(t *testing.T) {
	f, _ := os.Open("/proc/1/mountinfo")

	mountInfoList, _ := mountinfo.GetMountsFromReader(f, nil)

	for _, mountInfo := range mountInfoList {
		klog.Infof("source: %s, root: %s, mountpoint: %s, fstype: %s ", mountInfo.Source, mountInfo.Root, mountInfo.Mountpoint, mountInfo.FSType)
	}
}
