package bind

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"fmt"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

type HostPathPVBind struct {
	k8sControl hostpathclient.HostPathController
}

func init() {
	_ = Register(&Bind{
		Interface: &HostPathPVBind{},
	})
}

func (b *HostPathPVBind) Name() string {
	return "hostpathpvbind"
}

func (b *HostPathPVBind) Init(k8sControl hostpathclient.HostPathController) error {
	b.k8sControl = k8sControl
	return nil
}

func (b *HostPathPVBind) Bind(podName string, podNamespace string, nodeName string) *schedulerapi.ExtenderBindingResult {

	util.DebugLogMsg("Attempting to bind %s/%s to %s", podNamespace, podName, nodeName)

	pod := b.k8sControl.GetPod(podName, podNamespace)
	if pod == nil {
		return &schedulerapi.ExtenderBindingResult{
			Error: fmt.Sprintf("pod %s/%s not found,perhaps it do not use hostpathpv", podName, podNamespace),
		}
	}

	for _, claimName := range pod.GetPVClaimList() {

		pv := b.k8sControl.GetPersistentVolumeByClaim(claimName, pod.GetNamespace())
		if pv == nil {
			util.DebugLogMsg("find no pv by claim %s/%s, perhaps it is not bounded", pod.GetNamespace(), claimName)
			continue
		}
		if !pv.IsHostPath() {
			util.DebugLogMsg("pv %s bounded by claim %s/%s is not a hostPath pv", pv.GetName(), pod.GetNamespace(), claimName)
			continue
		}
		if pv.IsEphemeral() {
			util.DebugLogMsg(" pv %s bounded by claim %s/%s is ephemeral", pv.GetName(), pod.GetNamespace(), claimName)
			continue
		}

		if len(pv.GetBindNodeName()) == 0 {
			if err := b.k8sControl.BindHostPathPV(pv.GetName(), nodeName); err != nil {
				return &schedulerapi.ExtenderBindingResult{
					Error: err.Error(),
				}
			}
		} else if pv.GetBindNodeName() != nodeName {
			return &schedulerapi.ExtenderBindingResult{
				Error: fmt.Sprintf("pv %s has bind to node %s conflict", pv.GetName(), pv.GetBindNodeName()),
			}
		}
	}

	err := b.k8sControl.BindPod(pod, nodeName)
	if err != nil {
		return &schedulerapi.ExtenderBindingResult{
			Error: fmt.Sprintf("failed bind pod %s/%s to node %s . err: %v", podNamespace, podName, nodeName, err),
		}
	}

	util.DebugLogMsg("Successful bind %s/%s to %s", podNamespace, podName, nodeName)

	return &schedulerapi.ExtenderBindingResult{
		Error: "",
	}
}
