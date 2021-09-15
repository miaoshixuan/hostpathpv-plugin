package hostpathclient

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"time"
)

const (
	podNamespaceIndex         = "PodNamespace"
	pvClaimNameNamespaceIndex = "PvClaimNameNamespace"
)

type HostPathController interface {

	// get pods with persistentVolumeClaims by name and namespace
	GetPod(name, namespace string) *Pod

	// get node by name
	GetNode(name string) *Node

	// get persistentVolume by bounded pvc
	GetPersistentVolumeByClaim(claimName, claimNamespace string) *PersistentVolume

	// bind pv to node
	BindHostPathPV(pvName string, nodeName string) error

	// bind pod to node
	BindPod(pod *Pod, nodeName string) error

	Run() error
}

type HostPathControl struct {
	k8sClient kubernetes.Interface
	stopCh    chan struct{}

	pvController   cache.Controller
	podController  cache.Controller
	nodeController cache.Controller

	pvLister   cache.Indexer
	podLister  cache.Indexer
	nodeLister cache.Indexer

	ctx context.Context
}

// NewHostPathControl creates a k8sController for extender-scheduler
func NewHostPathControl(client kubernetes.Interface) HostPathController {

	hpc := HostPathControl{
		k8sClient: client,
		stopCh:    make(chan struct{}),
		ctx:       context.TODO(),
	}

	hpc.podLister, hpc.podController = NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  podListFunc(hpc.ctx, hpc.k8sClient, corev1.NamespaceAll),
			WatchFunc: podWatchFunc(hpc.ctx, hpc.k8sClient, corev1.NamespaceAll),
		},
		&corev1.Pod{},
		cache.ResourceEventHandlerFuncs{AddFunc: hpc.Add, UpdateFunc: hpc.Update, DeleteFunc: hpc.Delete},
		cache.Indexers{podNamespaceIndex: podNamespaceIndexFunc},
		DefaultProcessor(ToPod()),
	)

	hpc.nodeLister, hpc.nodeController = NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  nodeListFunc(hpc.ctx, hpc.k8sClient),
			WatchFunc: nodeWatchFunc(hpc.ctx, hpc.k8sClient),
		},
		&corev1.Node{},
		cache.ResourceEventHandlerFuncs{AddFunc: hpc.Add, UpdateFunc: hpc.Update, DeleteFunc: hpc.Delete},
		cache.Indexers{},
		DefaultProcessor(ToNode()),
	)

	hpc.pvLister, hpc.pvController = NewIndexerInformer(
		&cache.ListWatch{
			ListFunc:  pvListFunc(hpc.ctx, hpc.k8sClient),
			WatchFunc: pvWatchFunc(hpc.ctx, hpc.k8sClient),
		},
		&corev1.PersistentVolume{},
		cache.ResourceEventHandlerFuncs{AddFunc: hpc.Add, UpdateFunc: hpc.Update, DeleteFunc: hpc.Delete},
		cache.Indexers{pvClaimNameNamespaceIndex: pvClaimNameNamespaceIndexFunc},
		DefaultProcessor(ToPersistentVolume()),
	)

	return &hpc
}

func (hpc *HostPathControl) Run() error {
	go hpc.podController.Run(hpc.stopCh)
	go hpc.nodeController.Run(hpc.stopCh)
	go hpc.pvController.Run(hpc.stopCh)

	if err := hpc.waitForResourceSynced(); err != nil {
		return err
	}
	return nil
}

func (hpc *HostPathControl) waitForResourceSynced() error {
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for initialization")
		case <-ticker.C:
			var unSyncedResources []string
			if !hpc.podController.HasSynced() {
				unSyncedResources = append(unSyncedResources, "pods")
			}
			if !hpc.nodeController.HasSynced() {
				unSyncedResources = append(unSyncedResources, "nodes")
			}
			if !hpc.pvController.HasSynced() {
				unSyncedResources = append(unSyncedResources, "persistentVolumes")
			}
			if len(unSyncedResources) > 0 {
				util.DebugLogMsg("Waiting for %v to be initialized...", unSyncedResources)
				continue
			}
			util.DefaultLog("Initialized pods and persistentVolumes")
			return nil
		}
	}
}

func (hpc *HostPathControl) GetPod(name, namespace string) (pod *Pod) {
	os, err := hpc.podLister.ByIndex(podNamespaceIndex, namespace)
	if err != nil {
		util.ErrorLog(hpc.ctx, "failed get Pod: %s/%s. err: %v", namespace, name, err)
		return nil
	}
	for _, o := range os {
		p, ok := o.(*Pod)
		if !ok {
			continue
		}
		if p.GetName() != name {
			continue
		}
		util.DebugLog(hpc.ctx, "found pod %s/%s ", namespace, name)
		return p
	}
	util.ErrorLog(hpc.ctx, "Pod: %s/%s not found", namespace, name)
	return nil
}

func (hpc *HostPathControl) GetNode(name string) (pod *Node) {
	os := hpc.nodeLister.List()
	for _, o := range os {
		p, ok := o.(*Node)
		if !ok {
			continue
		}
		if p.GetName() != name {
			continue
		}
		util.DebugLog(hpc.ctx, "found node %s ", name)
		return p
	}
	util.ErrorLog(hpc.ctx, "Node: %s not found", name)
	return nil
}

func (hpc *HostPathControl) GetPersistentVolumeByClaim(name, namespace string) (pv *PersistentVolume) {
	os, err := hpc.pvLister.ByIndex(pvClaimNameNamespaceIndex, PersistentVolumeKey(name, namespace))
	if err != nil {
		util.ErrorLog(hpc.ctx, "failed get HostPathPV by claim: %s/%s. err: %v", namespace, name, err)
		return nil
	}
	for _, o := range os {
		p, ok := o.(*PersistentVolume)
		if !ok {
			continue
		}
		util.DebugLog(hpc.ctx, "found persistentVolume %s by claim : %s/%s", p.name, namespace, name)
		return p
	}
	util.ErrorLog(hpc.ctx, "no persistentVolume found by claim : %s/%s", namespace, name)
	return nil
}

func (hpc *HostPathControl) BindHostPathPV(pvName string, nodeName string) error {
	buf := getHostPathPVBindNodePatchBytes(nodeName)

	_, err := hpc.k8sClient.CoreV1().PersistentVolumes().Patch(hpc.ctx, pvName, types.JSONPatchType, buf, metav1.PatchOptions{})

	return err
}

func (hpc *HostPathControl) BindPod(pod *Pod, nodeName string) error {
	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{Namespace: pod.GetNamespace(), Name: pod.GetName(), UID: pod.GetUID()},
		Target:     corev1.ObjectReference{Kind: "Node", Name: nodeName},
	}

	return hpc.k8sClient.CoreV1().Pods(pod.GetNamespace()).Bind(hpc.ctx, binding, metav1.CreateOptions{})
}

// this controller is only used by extender scheduler
// we only list and watch  pending pods
func podListFunc(ctx context.Context, c kubernetes.Interface, namespace string) func(metav1.ListOptions) (runtime.Object, error) {
	return func(options metav1.ListOptions) (runtime.Object, error) {
		if len(options.FieldSelector) > 0 {
			options.FieldSelector = options.FieldSelector + ","
		}
		options.FieldSelector = options.FieldSelector + "status.phase==Pending"
		return c.CoreV1().Pods(namespace).List(ctx, options)
	}
}

func podWatchFunc(ctx context.Context, c kubernetes.Interface, namespace string) func(options metav1.ListOptions) (watch.Interface, error) {
	return func(options metav1.ListOptions) (watch.Interface, error) {
		if len(options.FieldSelector) > 0 {
			options.FieldSelector = options.FieldSelector + ","
		}
		options.FieldSelector = options.FieldSelector + "status.phase==Pending"
		return c.CoreV1().Pods(namespace).Watch(ctx, options)
	}
}

func nodeListFunc(ctx context.Context, c kubernetes.Interface) func(metav1.ListOptions) (runtime.Object, error) {
	return func(options metav1.ListOptions) (runtime.Object, error) {
		return c.CoreV1().Nodes().List(ctx, options)
	}
}

func nodeWatchFunc(ctx context.Context, c kubernetes.Interface) func(options metav1.ListOptions) (watch.Interface, error) {
	return func(options metav1.ListOptions) (watch.Interface, error) {
		return c.CoreV1().Nodes().Watch(ctx, options)
	}
}

func pvListFunc(ctx context.Context, c kubernetes.Interface) func(metav1.ListOptions) (runtime.Object, error) {
	return func(options metav1.ListOptions) (runtime.Object, error) {
		return c.CoreV1().PersistentVolumes().List(ctx, options)
	}
}

func pvWatchFunc(ctx context.Context, c kubernetes.Interface) func(options metav1.ListOptions) (watch.Interface, error) {
	return func(options metav1.ListOptions) (watch.Interface, error) {
		return c.CoreV1().PersistentVolumes().Watch(ctx, options)
	}
}

func podNamespaceIndexFunc(obj interface{}) ([]string, error) {
	p, ok := obj.(*Pod)
	if !ok {
		return nil, fmt.Errorf("type assertion failed! expected 'hostPathPod', got %T ", p)
	}
	return []string{p.GetNamespace()}, nil
}

func pvClaimNameNamespaceIndexFunc(obj interface{}) ([]string, error) {
	p, ok := obj.(*PersistentVolume)

	if !ok {
		return nil, fmt.Errorf("type assertion failed! expected 'hostPathVolume', got %T ", p)
	}

	return []string{p.GetIndex()}, nil
}

func (hpc *HostPathControl) Add(obj interface{}) {
	util.DebugLogMsg("successful add obj %v", obj)
	return
}

func (hpc *HostPathControl) Delete(obj interface{}) {
	util.DebugLogMsg("successful delete obj %v", obj)
	return
}

func (hpc *HostPathControl) Update(oldObj, newObj interface{}) {
	util.DebugLogMsg("successful update obj from %v to %v", oldObj, newObj)
	return
}
