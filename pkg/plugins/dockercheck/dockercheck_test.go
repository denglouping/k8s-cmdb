package dockercheck

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"k8s-cmdb/pkg/util"
	"testing"
	"time"
)

func TestGetDockerCli(t *testing.T) {
	cli, err := GetDockerCli("/var/run/docker.sock")
	if err != nil {
		t.Errorf(err.Error())
	}

	ctx, _ := util.GetCtx(time.Second * 10)
	containerList, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		t.Errorf(err.Error())
	}

	for _, container := range containerList {
		ctx, _ = util.GetCtx(time.Second * 10)
		status, err := GetContainerPIDStatus(1)
		if err != nil {
			t.Errorf(err.Error())
		}
		fmt.Printf("%s: %s\n", container.ID, status)

	}

}
