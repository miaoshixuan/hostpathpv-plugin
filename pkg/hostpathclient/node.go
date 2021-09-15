package hostpathclient

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"strings"
)

type void struct{}

// Node is custom type of  api.node for hostPathPv plugin.
type Node struct {
	version      string
	name         string
	namespace    string
	hostPathNode bool
	diskUsage    NodeUsageInfo
	disabledDisk map[string]void
	ready        string
	*Empty
}

func ToNode() ToFunc {
	return func(obj interface{}) (interface{}, error) {
		node, ok := obj.(*corev1.Node)
		if !ok {
			return nil, fmt.Errorf("unexpected object %v", obj)
		}

		return toHostPathNode(node), nil
	}
}

func toHostPathNode(node *corev1.Node) *Node {
	n := &Node{
		version:      node.GetResourceVersion(),
		name:         node.GetName(),
		namespace:    node.GetNamespace(),
		hostPathNode: IsHostPathNode(node),
	}

	if n.hostPathNode {
		n.disabledDisk = getNodeHostPathDisabledDisks(node)

		if nodeUsage, err := getNodeHostPathUsageInfo(node); err == nil {
			n.diskUsage = nodeUsage
		} else {
			util.ErrorLogMsg("failed unmarshal usage info of pv %s, err: %v", node.GetName(), err)
		}
	}

	*node = corev1.Node{}

	return n
}

var _ runtime.Object = &Node{}

func (h Node) DeepCopyObject() runtime.Object {
	h1 := &Node{
		version:   h.version,
		name:      h.name,
		namespace: h.namespace,
		diskUsage: h.diskUsage,
	}
	return h1
}

func ToHostPathNode(node *corev1.Node) *Node {
	n := &Node{
		version:      node.GetResourceVersion(),
		name:         node.GetName(),
		namespace:    node.GetNamespace(),
		hostPathNode: IsHostPathNode(node),
		ready:        getNodeReadyStatus(node.Status.Conditions),
	}

	if n.hostPathNode {
		n.disabledDisk = getNodeHostPathDisabledDisks(node)

		if nodeUsage, err := getNodeHostPathUsageInfo(node); err == nil {
			n.diskUsage = nodeUsage
		} else {
			util.ErrorLogMsg("failed unmarshal usage info of pv %s, err: %v", node.GetName(), err)
		}
	}
	return n
}

func IsHostPathNode(node *corev1.Node) bool {
	if _, ok := node.GetLabels()[HostPathPvNodeLabelKey]; ok {
		return true
	}
	return false
}

func getNodeHostPathUsageInfo(node *corev1.Node) (NodeUsageInfo, error) {
	nodeUsage := NodeUsageInfo{}

	if len(node.Annotations[NodeDiskQuotaInfoAnn]) > 0 {
		decoded, err := base64.StdEncoding.DecodeString(node.Annotations[NodeDiskQuotaInfoAnn])
		if err != nil {
			return NodeUsageInfo{}, err
		}
		if err := json.Unmarshal(decoded, &nodeUsage); err != nil {
			return nodeUsage, err
		}
	}
	return nodeUsage, nil
}

func getNodeHostPathUsageInfoPatchBytes(nodeUsage NodeUsageInfo) ([]byte, error) {
	buf, err := json.Marshal(nodeUsage)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	b.WriteString("[{")
	b.WriteString(`"op":"add"`)
	b.WriteString(fmt.Sprintf(`,"path":"/metadata/annotations/%s"`, rfc6901Encoder.Replace(NodeDiskQuotaInfoAnn)))
	b.WriteString(fmt.Sprintf(`,"value": "%s"`, base64.StdEncoding.EncodeToString(buf)))
	b.WriteString("}]")
	return b.Bytes(), nil
}

func getNodeReadyStatus(conditions []corev1.NodeCondition) string {
	var nodeCondition corev1.NodeCondition
	for _, condition := range conditions {
		if condition.Type == corev1.NodeReady {
			nodeCondition = condition
			break
		}
	}
	if nodeCondition.Status == corev1.ConditionTrue {
		return "Ready"
	} else {
		return "unReady"
	}
}

func getNodeHostPathDisabledDisks(node *corev1.Node) map[string]void {
	ret := make(map[string]void)
	if len(node.Annotations[NodeDiskQuotaDisabledDisksAnn]) > 0 {
		decoded, err := base64.StdEncoding.DecodeString(node.Annotations[NodeDiskQuotaDisabledDisksAnn])
		if err == nil {
			util.ErrorLogMsg("failed parse disabled disk info")
		}
		disableDisk := strings.Split(string(decoded), ",")
		for _, disk := range disableDisk {
			ret[disk] = void{}
		}
	}
	return ret
}

func getNodeHostPathDisabledDisksPatchBytes(node *corev1.Node, diskPath string, enable bool) []byte {
	disks := getNodeHostPathDisabledDisks(node)
	if enable {
		delete(disks, diskPath)
	} else {
		disks[diskPath] = void{}
	}

	diskSlice := make([]string, 0, len(disks))
	for k := range disks {
		diskSlice = append(diskSlice, k)
	}
	var b bytes.Buffer
	b.WriteString("[{")
	b.WriteString(`"op":"add"`)
	b.WriteString(fmt.Sprintf(`,"path":"/metadata/annotations/%s"`, rfc6901Encoder.Replace(NodeDiskQuotaDisabledDisksAnn)))
	b.WriteString(fmt.Sprintf(`,"value": "%s"`, base64.StdEncoding.EncodeToString([]byte(strings.Join(diskSlice, ",")))))
	b.WriteString("}]")
	return b.Bytes()
}

// GetNamespace implements the metav1.Object interface.
func (h *Node) GetNamespace() string { return h.namespace }

// SetNamespace implements the metav1.Object interface.
func (h *Node) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (h *Node) GetName() string { return h.name }

// SetName implements the metav1.Object interface.
func (h *Node) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (h *Node) GetResourceVersion() string { return h.version }

// SetResourceVersion implements the metav1.Object interface.
func (h *Node) SetResourceVersion(version string) {}

// IsHostPathNode returns if node is hostpath node
func (h *Node) IsHostPathNode() bool { return h.hostPathNode }

// GetDiskUsage returns node hostpath disk usage state
func (h *Node) GetDiskUsage() NodeUsageInfo { return h.diskUsage }

// GetDisabledDisk returns node hostpath disabled disk
func (h *Node) GetDisabledDisk() map[string]void { return h.disabledDisk }

// GetReady returns node ready state
func (h *Node) GetReady() string { return h.ready }
