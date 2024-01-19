package diskcheck

import (
	"k8s-cmdb/pkg/mount"
	"k8s.io/klog/v2"
	"testing"
)

func TestMount(t *testing.T) {
	mountInfoList, err := mount.GetProcessMountInfo(1)
	if err != nil {
		klog.Info(err.Error())
	}

	for _, mountInfo := range mountInfoList {
		klog.Infof("%s %s %s %d", mountInfo.Source, mountInfo.Root, mountInfo.Mountpoint, mountInfo.ID)
	}

}
