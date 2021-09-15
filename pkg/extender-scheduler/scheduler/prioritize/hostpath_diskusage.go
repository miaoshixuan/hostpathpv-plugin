package prioritize

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"fmt"
	api "k8s.io/api/core/v1"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
	"sync"
)

const DefaultNodeScore = 100

func init() {
	_ = Register(&Prioritize{
		Interface: &HostPathPVDiskUse{},
	})
}

type HostPathPVDiskUse struct {
	k8sControl hostpathclient.HostPathController
}

func (p *HostPathPVDiskUse) Name() string {
	return "hostpathpvdiskuse"
}

func (p *HostPathPVDiskUse) Init(k8sControl hostpathclient.HostPathController) error {
	p.k8sControl = k8sControl
	return nil
}

func (p *HostPathPVDiskUse) ScoreNodes(pod *api.Pod, nodes []api.Node) (*schedulerapi.HostPriorityList, error) {
	priorityList := make(schedulerapi.HostPriorityList, len(nodes))

	hasHostPath := true
	for _, node := range nodes {
		//只要一个node不是hostpath node，说明pod不包含hostpath，调度不用根据使用量打分
		if !hostpathclient.IsHostPathNode(&node) {
			hasHostPath = false
			break
		}
	}

	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		node := &nodes[i]
		go p.mapScoringNode(&wg, pod, hasHostPath, node, &priorityList[i])
	}
	wg.Wait()

	if err := p.reduceScoringNode(pod, priorityList, hasHostPath); err != nil {
		return &priorityList, fmt.Errorf("reduceScoringNode for pod %s:%s err:%v", pod.Namespace, pod.Name, err)
	}
	return &priorityList, nil

}

func (p *HostPathPVDiskUse) mapScoringNode(wg *sync.WaitGroup, pod *api.Pod, hasHostPath bool, node *api.Node, priority *schedulerapi.HostPriority) {
	defer wg.Done()

	var count int64

	if !hasHostPath {
		count = DefaultNodeScore
	} else {
		hpNode := hostpathclient.ToHostPathNode(node)
		capacity := hpNode.GetDiskUsage().Capacity
		allocated := hpNode.GetDiskUsage().QuotaSize

		if capacity <= 0 || capacity <= allocated {
			count = 0
		} else {
			count = int64((float64(100) * float64(capacity-allocated)) / float64(capacity))
		}
	}
	priority.Host = node.GetName()
	priority.Score = count
	util.DebugLogMsg("HostPathPVDiskUse mapScoringNode pod %s:%s to node %s score %d", pod.Namespace, pod.Name, node.GetName(), count)
}

func (p *HostPathPVDiskUse) reduceScoringNode(pod *api.Pod, priorityList schedulerapi.HostPriorityList, hasHostPath bool) error {
	if !hasHostPath {
		// all node has default score ,no need reduce
		return nil
	}

	var maxCount int64
	for i := range priorityList {
		if priorityList[i].Score > maxCount {
			maxCount = priorityList[i].Score
		}
	}
	maxCountFloat := float64(maxCount)

	var fScore float64
	for i := range priorityList {
		if maxCount > 0 {
			fScore = 10 * (float64(priorityList[i].Score) / maxCountFloat)
		} else {
			fScore = 0
		}
		priorityList[i].Score = int64(fScore)
		util.DebugLogMsg("HostPathPVDiskUse reduceScoringNode pod %s:%s to node %s score:%d", pod.Namespace, pod.Name, priorityList[i].Host, priorityList[i].Score)
	}
	return nil
}
