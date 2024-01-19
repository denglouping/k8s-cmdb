package mount

import (
	"fmt"
	"github.com/moby/sys/mountinfo"
	"k8s-cmdb/pkg/util"
	"os"
)

type MountInfo struct {
	mountinfo.Info
}

func GetProcessMountInfo(pid int32) ([]*mountinfo.Info, error) {
	f, err := os.Open(fmt.Sprintf("%s/proc/%d/mountinfo", util.GetHostPath(), pid))
	if err != nil {
		return nil, err
	}

	mountInfoList, err := mountinfo.GetMountsFromReader(f, nil)
	if err != nil {
		return nil, err
	}

	return mountInfoList, nil
}
