package uploader

import (
	"k8s-cmdb/pkg/node"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
)

func TestKubernetes(t *testing.T) {
	kubeconfig, _ := clientcmd.BuildConfigFromFlags("", "/root/.kube/config")
	cs, _ := kubernetes.NewForConfig(kubeconfig)

	u := NewKubernetesUploader("testnode", cs, 5)
	err := u.Upload(&node.NodeInfo{})
	if err != nil {
		t.Errorf(err.Error())
	}
}
