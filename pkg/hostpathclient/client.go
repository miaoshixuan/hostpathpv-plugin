package hostpathclient

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"strings"
)

var rfc6901Encoder = strings.NewReplacer("~", "~0", "/", "~1")

type HostPathClientInterface interface {
	// get hostPath type pv by name
	GetHostPathPVByVolumeId(volId string) *PersistentVolume

	// get node bu node name
	GetNodeByName(name string) (*Node, error)

	// update hostPath pv Usage info
	UpdateHostPathPVUsageInfo(pvName string, hpv *PersistentVolume) error

	// update hostPath node quota disk usage info
	UpdateNodeHostPathPVUsageInfo(nodeName string, nodeQuotaInfo NodeUsageInfo) error

	// set hostPath node disk disable state
	SetHostPathDiskDisableState(nodeName string, diskPath string, state bool) error
}

type HostPathClient struct {
	client kubernetes.Interface
	ctx    context.Context
}

func NewHostPathClient(client kubernetes.Interface) HostPathClientInterface {
	return &HostPathClient{
		client: client,
		ctx:    context.Background(),
	}
}

func (h HostPathClient) GetHostPathPVByVolumeId(volumeId string) *PersistentVolume {
	var vi util.CSIIdentifier
	if err := vi.DecomposeCSIID(volumeId); err != nil {
		util.ErrorLogMsg("failed decode volume id %s", volumeId)
		return nil
	}
	pv, err := h.client.CoreV1().PersistentVolumes().Get(h.ctx, vi.VolumeName, metav1.GetOptions{})
	if err != nil {
		util.ErrorLogMsg("failed get pv by name: %s. err: %v", vi.VolumeName, err)
		return nil
	}
	return toPersistentVolume(pv)
}

func (h HostPathClient) GetNodeByName(name string) (*Node, error) {
	node, err := h.client.CoreV1().Nodes().Get(h.ctx, name, metav1.GetOptions{})
	return ToHostPathNode(node), err
}

func (h HostPathClient) UpdateHostPathPVUsageInfo(pvName string, hpv *PersistentVolume) error {
	buf, err := getHostPathPVUsageInfoPatchBytes(hpv)
	if err != nil {
		return fmt.Errorf("failed set usageInfo for pv %s. err %v", pvName, err)
	}

	if buf == nil {
		return nil
	}
	_, err = h.client.CoreV1().PersistentVolumes().Patch(h.ctx, pvName, types.JSONPatchType, buf, metav1.PatchOptions{})
	return err
}

func (h HostPathClient) UpdateNodeHostPathPVUsageInfo(nodeName string, nodeQuotaInfo NodeUsageInfo) error {
	buf, err := getNodeHostPathUsageInfoPatchBytes(nodeQuotaInfo)
	if err != nil {
		return err
	}

	if buf == nil {
		return nil
	}

	_, err = h.client.CoreV1().Nodes().Patch(h.ctx, nodeName, types.JSONPatchType, buf, metav1.PatchOptions{})

	return err
}

func (h HostPathClient) SetHostPathDiskDisableState(nodeName string, diskPath string, state bool) error {
	node, err := h.client.CoreV1().Nodes().Get(h.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	buf := getNodeHostPathDisabledDisksPatchBytes(node, diskPath, state)

	_, err = h.client.CoreV1().Nodes().Patch(h.ctx, nodeName, types.JSONPatchType, buf, metav1.PatchOptions{})
	return err
}
