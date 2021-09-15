package main

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/extender-scheduler/scheduler/bind"
	"ShixuanMiao/hostpathpv-plugin/pkg/extender-scheduler/scheduler/predicate"
	"ShixuanMiao/hostpathpv-plugin/pkg/extender-scheduler/scheduler/prioritize"
	"ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"flag"
	"fmt"
	"github.com/julienschmidt/httprouter"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"net/http"
	_ "net/http/pprof"
	"os"
)

const (
	version        = 0.1
	versionPath    = "/version"
	apiPrefix      = "/scheduler"
	bindsPath      = apiPrefix + "/bind"
	preemptionPath = apiPrefix + "/preemption"
	predicatesPath = apiPrefix + "/predicates"
	prioritiesPath = apiPrefix + "/priorities"
)

var (
	address    string
	kubeConfig string
)

func init() {
	// common flags
	flag.StringVar(&address, "address", ":6600", "The address to expose server.")
	flag.StringVar(&kubeConfig, "kubeconfig", "", "kube config file path")

	klog.InitFlags(nil)
	if err := flag.Set("logtostderr", "true"); err != nil {
		klog.Exitf("failed to set logtostderr flag: %v", err)
	}
	if err := flag.Set("v", "2"); err != nil {
		klog.Exitf("failed to set default log level flag: %v", err)
	}
	flag.Parse()
}

func getK8sClient() *k8s.Clientset {
	var cfg *rest.Config
	var err error

	if kubeConfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			util.FatalLogMsg("Failed to get cluster config with error: %v\n", err)
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			util.FatalLogMsg("Failed to get cluster config with error: %v\n", err)
		}
	}
	client, err := k8s.NewForConfig(cfg)
	if err != nil {
		util.FatalLogMsg("Failed to create client with error: %v\n", err)
	}
	return client
}

func initAll(k8sControl hostpathclient.HostPathController) error {
	if err := predicate.Init(k8sControl); err != nil {
		return err
	}
	util.DefaultLog("Initialized all predicate plugins")
	if err := prioritize.Init(k8sControl); err != nil {
		return err
	}
	util.DefaultLog("Initialized all prioritize plugins")
	if err := bind.Init(k8sControl); err != nil {
		return err
	}
	util.DefaultLog("Initialized all bind plugins")
	return nil
}

func main() {
	defer klog.Flush()

	clientSet := getK8sClient()

	util.DefaultLog("hostPath CSI Driver version: %v", util.DriverVersion)

	util.DefaultLog("start init all")

	cacheControl := hostpathclient.NewHostPathControl(clientSet)
	if err := cacheControl.Run(); err != nil {
		util.FatalLogMsg(err.Error())
		os.Exit(1)
	}

	if err := initAll(cacheControl); err != nil {
		util.FatalLogMsg(err.Error())
		os.Exit(1)
	}

	util.DefaultLog("finish init")

	router := httprouter.New()
	router.GET(versionPath, func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		_, _ = fmt.Fprint(writer, fmt.Sprint(version))
	})
	predicate.AddRoute(router, predicatesPath)
	prioritize.AddRoute(router, prioritiesPath)
	bind.AddRoute(router, bindsPath)

	if err := http.ListenAndServe(address, router); err != nil {
		util.FatalLogMsg(err.Error())
		os.Exit(1)
	}

}
