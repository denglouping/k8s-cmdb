package uploader

import (
	"gopkg.in/yaml.v2"
	"k8s-cmdb/pkg/node"
	"k8s-cmdb/pkg/util"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v12 "k8s.io/client-go/applyconfigurations/core/v1"
	v13 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sort"
	"strconv"
	"strings"
	"time"
)

type KubernetesUploader struct {
	cs           *kubernetes.Clientset
	historyLimit int
	name         string
	namespace    string
}

// TODO 优化K8S api操作次数
func (u *KubernetesUploader) Upload(info *node.NodeInfo) error {
	if u.historyLimit == 0 {
		return nil
	}

	ctx, _ := util.GetCtx(time.Second * 10)
	cmList, err := u.cs.CoreV1().ConfigMaps(u.namespace).List(ctx, v1.ListOptions{
		ResourceVersion: "0", LabelSelector: "nodeagent=" + u.name})
	if err != nil {
		return err
	}

	sort.Slice(cmList.Items, func(i, j int) bool {
		return cmList.Items[i].Name > cmList.Items[j].Name
	})

	versionCMList := make(map[int]corev1.ConfigMap)
	for _, cm := range cmList.Items {
		if !strings.Contains(cm.Name, u.name+"-v") {
			continue
		}

		if len(strings.Split(cm.Name, u.name+"-v")) != 2 {
			continue
		}

		version, err := strconv.Atoi(strings.Split(cm.Name, u.name+"-v")[1])
		if err != nil {
			return err
		}

		if version > u.historyLimit {
			ctx, _ := util.GetCtx(time.Second * 10)
			err = u.cs.CoreV1().ConfigMaps("nodeagent").Delete(ctx, cm.Name, v1.DeleteOptions{})
			if err != nil {
				return err
			}

		}
		versionCMList[version] = cm
	}

	for version, cm := range versionCMList {
		if version >= u.historyLimit {
			continue
		}
		if targetCM, ok := versionCMList[version+1]; ok {
			apiVersion := "v1"
			kind := "ConfigMap"

			ctx, _ := util.GetCtx(time.Second * 10)
			_, err = u.cs.CoreV1().ConfigMaps("nodeagent").Apply(ctx, &v12.ConfigMapApplyConfiguration{
				TypeMetaApplyConfiguration: v13.TypeMetaApplyConfiguration{
					Kind:       &kind,
					APIVersion: &apiVersion,
				},
				ObjectMetaApplyConfiguration: &v13.ObjectMetaApplyConfiguration{
					Name:      &targetCM.Name,
					Namespace: &targetCM.Namespace,
				},
				Data: cm.Data,
			}, v1.ApplyOptions{
				Force:        true,
				FieldManager: "application/apply-patch",
			})
			if err != nil {
				return err
			}

		} else {
			newCm := corev1.ConfigMap{
				ObjectMeta: v1.ObjectMeta{
					Name:      u.name + "-v" + strconv.Itoa(version+1),
					Namespace: "nodeagent",
					Labels: map[string]string{
						"nodeagent": u.name,
					},
				},
				Data: cm.Data,
			}

			ctx, _ := util.GetCtx(time.Second * 10)
			_, err := u.cs.CoreV1().ConfigMaps("nodeagent").Create(ctx, &newCm, v1.CreateOptions{})
			if err != nil {
				return err
			}
		}

	}

	_, ok := versionCMList[1]
	if len(versionCMList) == 0 || !ok {
		newCm := corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      u.name + "-v1",
				Namespace: "nodeagent",
				Labels: map[string]string{
					"nodeagent": u.name,
				},
			},
		}

		info.Lock()
		nodeinfoData, _ := yaml.Marshal(info)
		info.Unlock()

		newCm.Data = map[string]string{
			"nodeinfo": string(nodeinfoData) + time.Now().String(),
		}

		ctx, _ := util.GetCtx(time.Second * 10)
		_, err = u.cs.CoreV1().ConfigMaps("nodeagent").Create(ctx, &newCm, v1.CreateOptions{})
		if err != nil {
			return err
		}
	} else {
		apiVersion := "v1"
		kind := "ConfigMap"
		targetCM := versionCMList[1]
		nodeinfoData, _ := yaml.Marshal(info)

		ctx, _ := util.GetCtx(time.Second * 10)
		_, err = u.cs.CoreV1().ConfigMaps("nodeagent").Apply(ctx, &v12.ConfigMapApplyConfiguration{
			TypeMetaApplyConfiguration: v13.TypeMetaApplyConfiguration{
				Kind:       &kind,
				APIVersion: &apiVersion,
			},
			ObjectMetaApplyConfiguration: &v13.ObjectMetaApplyConfiguration{
				Name:      &targetCM.Name,
				Namespace: &targetCM.Namespace,
			},
			Data: map[string]string{
				"nodeinfo": string(nodeinfoData) + time.Now().String(),
			},
		}, v1.ApplyOptions{
			Force:        true,
			FieldManager: "application/apply-patch",
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func NewKubernetesUploader(name string, cs *kubernetes.Clientset, historyLimit int, namespace string) *KubernetesUploader {
	return &KubernetesUploader{name: name, cs: cs, historyLimit: historyLimit, namespace: namespace}
}
