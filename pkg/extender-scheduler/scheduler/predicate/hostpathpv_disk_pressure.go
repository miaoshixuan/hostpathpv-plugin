package predicate

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	api "k8s.io/api/core/v1"
	"sort"
)

func init() {
	_ = Register(&Predicate{
		Interface: &HostPathPVDiskPressure{},
	})
}

type HostPathPVDiskPressure struct {
	k8sControl hostpathclient.HostPathController
}

func (p *HostPathPVDiskPressure) Name() string {
	return "hostpathpvdiskpressure"
}

func (p *HostPathPVDiskPressure) Init(k8sControl hostpathclient.HostPathController) error {
	p.k8sControl = k8sControl
	return nil
}

func (p *HostPathPVDiskPressure) FilterNode(pod *api.Pod, node *api.Node) (bool, error) {
	var podVolumeRequestSize int64
	podVolumeRequestList := make([]int64, 0, len(pod.Spec.Volumes))

	hpNode := hostpathclient.ToHostPathNode(node)
	for _, podVolume := range pod.Spec.Volumes {
		if podVolume.PersistentVolumeClaim == nil {
			// pod volume is not a persistentVolume, skip
			continue
		}

		claimName := podVolume.PersistentVolumeClaim.ClaimName
		// get pv by claim name
		pv := p.k8sControl.GetPersistentVolumeByClaim(claimName, pod.GetNamespace())
		if pv == nil {
			util.ErrorLogMsg("find no pv by claim %s/%s, perhaps it is not bound", pod.GetNamespace(), claimName)
			return false, nil
		}

		if !pv.IsHostPath() {
			util.DebugLogMsg("pv %s bounded by claim %s/%s is not a hostPath pv", pv.GetName(), pod.GetNamespace(), claimName)
			continue
		}

		if !hpNode.IsHostPathNode() {
			util.DefaultLog("pod %s/%s contains hostPath pv but node %s is not hostPath node", pod.GetNamespace(), pod.GetName(), hpNode.GetName())
			return false, nil
		}

		// pod use persistent volume must schedule to volume bind node when volume has bind
		if len(pv.GetBindNodeName()) > 0 {
			if pv.GetBindNodeName() != hpNode.GetName() {
				util.DebugLogMsg("pv %s used by pod %s/%s has bind to node %s not %s", pv.GetName(), pod.GetNamespace(), pod.GetName(), pv.GetBindNodeName(), hpNode.GetName())
				return false, nil
			}

			// no need allocate new quota path
			continue
		}

		capacity := pv.GetCapacity()
		podVolumeRequestList = append(podVolumeRequestList, capacity)
		podVolumeRequestSize += capacity

	}

	// all hostPath pv has exist path on node
	if podVolumeRequestSize == 0 || len(podVolumeRequestList) == 0 {
		util.DebugLogMsg("pod %s:%s for node %s run directly %d, %v", pod.Namespace, pod.Name, hpNode.GetName(), podVolumeRequestSize, podVolumeRequestList)
		return true, nil
	}

	if canNodeMatch(podVolumeRequestList, hpNode.GetDiskUsage().DiskStatus) {
		util.DebugLogMsg("pod %s/%s for node %s match", pod.GetNamespace(), pod.GetName(), hpNode.GetName())
		return true, nil
	}

	util.DefaultLog("node:%s, notMatch podRequest: %v, nodeAllocatableSize: %v ", node.GetName(), podVolumeRequestSize, hpNode.GetDiskUsage().DiskStatus)
	return false, nil
}

func canNodeMatch(requestList []int64, haveNow hostpathclient.DiskInfoList) bool {
	// sort reverse by request size
	sort.Slice(requestList, func(i, j int) bool {
		return requestList[i] > requestList[j]
	})

	var ir, ih int
	for {
		if ir >= len(requestList) || ih >= len(haveNow) {
			break
		}
		if requestList[ir] <= haveNow[ih].Allocatable {
			haveNow[ih].Allocatable -= requestList[ir]
			ir += 1
			continue
		} else {
			ih += 1
			continue
		}
	}
	// all capacity in requestList are matched
	if ir >= len(requestList) {
		return true
	}
	return false
}
