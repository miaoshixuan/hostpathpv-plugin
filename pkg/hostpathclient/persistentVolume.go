package hostpathclient

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PersistentVolume stripped down api.PersistentVolume with only the items we need.we also do some convert
type PersistentVolume struct {
	version      string
	name         string
	namespace    string
	index        string
	volId        string
	isHostPathPv bool
	bindNode     string
	ephemeral    bool
	capacity     int64
	volumePath   string
	usage        int64

	*Empty
}

// PersistentVolumeKey returns a string using for the index.
func PersistentVolumeKey(claimName, claimNamespace string) string {
	return claimNamespace + "/" + claimName
}

// ToPersistentVolume returns a function that converts an api.ToPersistentVolume to a *ToPersistentVolume.
func ToPersistentVolume() ToFunc {
	return func(obj interface{}) (interface{}, error) {
		pv, ok := obj.(*corev1.PersistentVolume)
		if !ok {
			return nil, fmt.Errorf("unexpected object %v", obj)
		}
		// skip unbound volume
		if pv.Status.Phase != corev1.VolumeBound {
			return nil, fmt.Errorf("persistentVolume %s is not bound,skip", pv.Name)
		}

		return toPersistentVolume(pv), nil
	}
}

func toPersistentVolume(pv *corev1.PersistentVolume) *PersistentVolume {
	hpv := &PersistentVolume{
		version:      pv.GetResourceVersion(),
		name:         pv.GetName(),
		namespace:    pv.GetNamespace(),
		index:        PersistentVolumeKey(pv.Spec.ClaimRef.Name, pv.Spec.ClaimRef.Namespace),
		volId:        pv.Spec.CSI.VolumeHandle,
		isHostPathPv: isHostPathPV(pv),
	}

	if hpv.isHostPathPv {
		hpv.bindNode = getHostPathPVBindNode(pv)
		hpv.ephemeral = isEphemeral(pv)
		hpv.capacity = getHostPathPVCapacity(pv)
		if usageInfo, err := getHostPathPVUsageInfo(pv); err != nil {
			util.ErrorLogMsg("failed unmarshal usage info of pv %s", pv.GetName())
		} else {
			hpv.volumePath = usageInfo.Path
			hpv.usage = usageInfo.Used
		}
	}

	*pv = corev1.PersistentVolume{}

	return hpv
}

func ToHostPathVolume(pv *corev1.PersistentVolume) *PersistentVolume {
	hpv := &PersistentVolume{
		version:      pv.GetResourceVersion(),
		name:         pv.GetName(),
		namespace:    pv.GetNamespace(),
		index:        PersistentVolumeKey(pv.Spec.ClaimRef.Name, pv.Spec.ClaimRef.Namespace),
		volId:        pv.Spec.CSI.VolumeHandle,
		isHostPathPv: isHostPathPV(pv),
	}

	if hpv.isHostPathPv {
		hpv.bindNode = getHostPathPVBindNode(pv)
		hpv.ephemeral = isEphemeral(pv)
		hpv.capacity = getHostPathPVCapacity(pv)
		if usageInfo, err := getHostPathPVUsageInfo(pv); err != nil {
			util.ErrorLogMsg("failed unmarshal usage info of pv %s", pv.GetName())
		} else {
			hpv.volumePath = usageInfo.Path
			hpv.usage = usageInfo.Used
		}
	}
	return hpv
}

var _ runtime.Object = &PersistentVolume{}

func (p PersistentVolume) DeepCopyObject() runtime.Object {
	h1 := &PersistentVolume{
		version:    p.version,
		name:       p.name,
		namespace:  p.namespace,
		index:      p.index,
		volId:      p.volId,
		bindNode:   p.bindNode,
		volumePath: p.volumePath,
		ephemeral:  p.ephemeral,
		capacity:   p.capacity,
		usage:      p.usage,
	}
	return h1
}

func isHostPathPV(pv *corev1.PersistentVolume) bool {
	if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == HostPathCsiDriverName {
		return true
	}
	return false
}

func isEphemeral(pv *corev1.PersistentVolume) bool {
	//decode volume info form volId
	var vi util.CSIIdentifier

	if err := vi.DecomposeCSIID(pv.Spec.CSI.VolumeHandle); err == nil {
		return vi.Ephemeral
	}
	return false
}

func getHostPathPVCapacity(pv *corev1.PersistentVolume) int64 {
	storage := pv.Spec.Capacity[corev1.ResourceStorage]
	return storage.Value()
}

func getHostPathPVBindNode(pv *corev1.PersistentVolume) string {
	// parse bind Node attr
	return pv.Annotations[HostPathPvBindInfoAnn]
}

func getHostPathPVBindNodePatchBytes(nodeName string) []byte {
	var b bytes.Buffer
	b.WriteString("[{")
	b.WriteString(`"op":"add"`)
	b.WriteString(fmt.Sprintf(`,"path":"/metadata/annotations/%s"`, rfc6901Encoder.Replace(HostPathPvBindInfoAnn)))
	b.WriteString(fmt.Sprintf(`,"value": "%s"`, nodeName))
	b.WriteString("}]")
	return b.Bytes()
}

func getHostPathPVUsageInfo(pv *corev1.PersistentVolume) (PVUsageInfo, error) {
	usageInfo := PVUsageInfo{}
	if len(pv.Annotations[HostPathPVUsageInfoAnn]) > 0 {
		decoded, err := base64.StdEncoding.DecodeString(pv.Annotations[HostPathPVUsageInfoAnn])
		if err != nil {
			return PVUsageInfo{}, err
		}
		err = json.Unmarshal(decoded, &usageInfo)
		if err != nil {
			return usageInfo, err
		}
	}
	return usageInfo, nil
}

func getHostPathPVUsageInfoPatchBytes(hpv *PersistentVolume) ([]byte, error) {
	usageInfo := PVUsageInfo{
		NodeName: hpv.GetBindNodeName(),
		Path:     hpv.GetVolumePath(),
		Capacity: hpv.GetCapacity(),
		Used:     int64(hpv.GetUsage()),
	}
	buf, err := json.Marshal(usageInfo)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	b.WriteString("[{")
	b.WriteString(`"op":"add"`)
	b.WriteString(fmt.Sprintf(`,"path":"/metadata/annotations/%s"`, rfc6901Encoder.Replace(HostPathPVUsageInfoAnn)))
	b.WriteString(fmt.Sprintf(`,"value": "%s"`, base64.StdEncoding.EncodeToString(buf)))
	b.WriteString("}]")
	return b.Bytes(), nil
}

// GetNamespace implements the metav1.Object interface.
func (p *PersistentVolume) GetNamespace() string { return p.namespace }

// SetNamespace implements the metav1.Object interface.
func (p *PersistentVolume) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (p *PersistentVolume) GetName() string { return p.name }

// SetName implements the metav1.Object interface.
func (p *PersistentVolume) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (p *PersistentVolume) GetResourceVersion() string { return p.version }

// SetResourceVersion implements the metav1.Object interface.
func (p *PersistentVolume) SetResourceVersion(version string) {}

// GetVolumeId return persistentVolume CSI VolumeHandle
func (p *PersistentVolume) GetVolumeId() string { return p.volId }

// GetIndex return persistentVolume cache index key
func (p *PersistentVolume) GetIndex() string { return p.index }

func (p *PersistentVolume) IsEphemeral() bool { return p.ephemeral }

func (p *PersistentVolume) IsHostPath() bool { return p.isHostPathPv }

func (p *PersistentVolume) GetBindNodeName() string { return p.bindNode }

func (p *PersistentVolume) SetBindNodeName(nodeName string) {
	p.bindNode = nodeName
}

func (p *PersistentVolume) GetVolumePath() string { return p.volumePath }

func (p *PersistentVolume) SetVolumePath(volumePath string) {
	p.volumePath = volumePath
}

func (p *PersistentVolume) GetCapacity() int64 { return p.capacity }

func (p *PersistentVolume) GetUsage() uint64 { return uint64(p.usage) }

func (p *PersistentVolume) SetUsage(usage uint64) {
	p.usage = int64(usage)
}

func (p *PersistentVolume) GetHardQuota() uint64 { return uint64(p.capacity) }

func (p *PersistentVolume) GetSoftQuota() uint64 { return uint64(p.capacity) }
