package hostpath

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type MetaStoreInterFace interface {
	// get meta configmap
	ListMetaInfos() (map[string]string, error)

	// add volume to meta store config map
	AddMetaInfo(volumeId string) error

	// remove volume to meta store config map
	UnsetMetaInfo(volumeId string) error
}

type ConfigMapMetaStore struct {
	client      kubernetes.Interface
	ctx         context.Context
	cmName      string
	cmNamespace string
}

func NewConfigMapMetaStore(client kubernetes.Interface, cmName, cmNamespace string) (MetaStoreInterFace, error) {
	mStore := &ConfigMapMetaStore{
		client:      client,
		ctx:         context.TODO(),
		cmName:      cmName,
		cmNamespace: cmNamespace,
	}
	if err := mStore.Init(); err != nil {
		return nil, err
	}
	return mStore, nil
}

func (c ConfigMapMetaStore) Init() error {
	cm, err := c.client.CoreV1().ConfigMaps(c.cmNamespace).Get(c.ctx, c.cmName, metav1.GetOptions{})
	if err != nil && apierrs.IsNotFound(err) {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.cmName,
			},
		}
		cm, err = c.client.CoreV1().ConfigMaps(c.cmNamespace).Create(c.ctx, cm, metav1.CreateOptions{})
		return err
	}
	return err
}

func (c ConfigMapMetaStore) ListMetaInfos() (map[string]string, error) {
	cm, err := c.client.CoreV1().ConfigMaps(c.cmNamespace).Get(c.ctx, c.cmName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return cm.Data, nil
}

func (c ConfigMapMetaStore) AddMetaInfo(volumeId string) error {
	cm, err := c.client.CoreV1().ConfigMaps(c.cmNamespace).Get(c.ctx, c.cmName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[volumeId] = ""
	_, err = c.client.CoreV1().ConfigMaps(c.cmNamespace).Update(c.ctx, cm, metav1.UpdateOptions{})
	return err
}

func (c ConfigMapMetaStore) UnsetMetaInfo(volumeId string) error {
	cm, err := c.client.CoreV1().ConfigMaps(c.cmNamespace).Get(c.ctx, c.cmName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	delete(cm.Data, volumeId)
	_, err = c.client.CoreV1().ConfigMaps(c.cmNamespace).Update(c.ctx, cm, metav1.UpdateOptions{})
	return err
}
