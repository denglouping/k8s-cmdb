package process

import (
	"fmt"
	"testing"
)

//func TestMount(t *testing.T) {
//	f, _ := os.Open("/proc/1/mountinfo")
//
//	mountInfoList, _ := mountinfo.GetMountsFromReader(f, nil)
//
//	for _, mountInfo := range mountInfoList {
//		klog.Infof("source: %s, root: %s, mountpoint: %s, fstype: %s ", mountInfo.Source, mountInfo.Root, mountInfo.Mountpoint, mountInfo.FSType)
//	}
//}

func TestGetProcessStatus(t *testing.T) {
	result, _ := GetProcessStatus()
	for _, status := range result {
		fmt.Println(status)
	}
}
