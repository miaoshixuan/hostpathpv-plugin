package hostpathclient

import (
	"fmt"
	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Pod is custom type of  api.Pod for hostPathPv plugin.
type Pod struct {
	UID       types.UID
	version   string
	name      string
	namespace string
	pvClaims  []string

	*Empty
}

func ToPod() ToFunc {
	return func(obj interface{}) (interface{}, error) {
		pod, ok := obj.(*api.Pod)
		if !ok {
			return nil, fmt.Errorf("unexpected object %v", obj)
		}

		return toPod(pod), nil
	}
}

func toPod(pod *api.Pod) *Pod {
	p := &Pod{
		UID:       pod.GetUID(),
		version:   pod.GetResourceVersion(),
		name:      pod.GetName(),
		namespace: pod.GetNamespace(),
		pvClaims:  make([]string, 0, len(pod.Spec.Volumes)),
	}

	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			p.pvClaims = append(p.pvClaims, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	*pod = api.Pod{}

	return p

}

var _ runtime.Object = &Pod{}

func (p Pod) DeepCopyObject() runtime.Object {
	p1 := &Pod{
		version:   p.version,
		name:      p.name,
		namespace: p.namespace,
		pvClaims:  make([]string, len(p.pvClaims)),
	}

	copy(p1.pvClaims, p.pvClaims)
	return p1
}

// GetNamespace implements the metav1.Object interface.
func (p *Pod) GetNamespace() string { return p.namespace }

// SetNamespace implements the metav1.Object interface.
func (p *Pod) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (p *Pod) GetName() string { return p.name }

// SetName implements the metav1.Object interface.
func (p *Pod) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (p *Pod) GetResourceVersion() string { return p.version }

// SetResourceVersion implements the metav1.Object interface.
func (p *Pod) SetResourceVersion(version string) {}

// GetUID implements the metav1.Object interface.
func (p *Pod) GetUID() types.UID { return p.UID }

// SetUID implements the metav1.Object interface.
func (p *Pod) SetUID(uid types.UID) {}

// GetPVClaimList returns all persistentVolumeClaims used by pod
func (p *Pod) GetPVClaimList() []string { return p.pvClaims }
