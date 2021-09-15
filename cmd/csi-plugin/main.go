package main

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/hostpath"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"flag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"os"
)

var (
	conf util.Config
)

func init() {
	// common flags
	flag.StringVar(&conf.KubeConfig, "kubeconfig", "", "kube config file path")
	flag.StringVar(&conf.Endpoint, "endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	flag.StringVar(&conf.NodeID, "nodeid", "", "node id")
	flag.StringVar(&conf.MetaConfigMapName, "metastorename", "hostpath-meta-store", "meta store cm name")
	flag.StringVar(&conf.MetaConfigMapNamespace, "metastorenamespace", "", "meta store cm namespace")

	flag.BoolVar(&conf.IsControllerServer, "controllerserver", false, "start hostpath csi k8scontroller server")
	flag.BoolVar(&conf.IsNodeServer, "nodeserver", false, "start hostpath csi node server")
	flag.StringVar(&conf.PluginPath, "pluginpath", "/var/lib/kubelet/plugins/", "the location of hostpathcsi plugin")
	flag.StringVar(&conf.HistogramOption, "histogramoption", "0.5,2,6",
		"Histogram option for grpc metrics, should be comma separated value, ex:= 0.5,2,6 where start=0.5 factor=2, count=6")

	klog.InitFlags(nil)
	if err := flag.Set("logtostderr", "true"); err != nil {
		klog.Exitf("failed to set logtostderr flag: %v", err)
	}
	if err := flag.Set("v", "2"); err != nil {
		klog.Exitf("failed to set default log level flag: %v", err)
	}
	flag.Parse()
}

func getK8sClient(kubeConfig string) (*kubernetes.Clientset, error) {
	var cfg *rest.Config
	var err error

	if kubeConfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			util.FatalLogMsg("Failed to get cluster config with error: %v\n", err)
			return nil, err
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			util.FatalLogMsg("Failed to get cluster config with error: %v\n", err)
			return nil, err
		}
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		util.FatalLogMsg("Failed to create client with error: %v\n", err)
		return nil, err
	}
	return client, nil
}

func main() {
	defer klog.Flush()

	clientSet, err := getK8sClient(conf.KubeConfig)
	if err != nil {
		klog.Fatalln(err.Error())
		os.Exit(1)
	}
	util.DefaultLog("Starting HostPath csi")
	driver := hostpath.NewDriver()
	driver.Run(&conf, clientSet)
	os.Exit(0)
}
