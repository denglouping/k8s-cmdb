package timecheck

import (
	"fmt"
	"testing"
)

func TestKubernetes(t *testing.T) {
	offset, err := GetTimeOffset("0.pool.ntp.org")
	if err != nil {
		t.Errorf(err.Error())
	}
	fmt.Println(offset)
}
