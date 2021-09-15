package bind

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
	"net/http"
	"sync"
)

type Interface interface {
	Name() string
	Init(k8sControl hostpathclient.HostPathController) error
	Bind(podName string, podNamespace string, nodeName string) *schedulerapi.ExtenderBindingResult
}

type Bind struct {
	Interface
}

func (b Bind) Handler(args schedulerapi.ExtenderBindingArgs) *schedulerapi.ExtenderBindingResult {
	return b.Bind(args.PodName, args.PodNamespace, args.Node)
}

var bindList []*Bind
var bindMu sync.Mutex

func Register(p *Bind) error {
	if p.Name() == "" {
		return fmt.Errorf("Prioritize name should not be empty")
	}
	bindMu.Lock()
	defer bindMu.Unlock()

	for _, existPrioritize := range bindList {
		if existPrioritize.Name() == p.Name() {
			return fmt.Errorf("prioritize %s is registed", p.Name())
		}
	}
	bindList = append(bindList, p)
	return nil
}

func Init(k8sControl hostpathclient.HostPathController) error {
	bindMu.Lock()
	defer bindMu.Unlock()
	for _, p := range bindList {
		if err := p.Init(k8sControl); err != nil {
			return fmt.Errorf("init prioritize %s error:%v", p.Name(), err)
		}
	}
	return nil
}

func bindsRoute(bind *Bind) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		if r.Body == nil {
			http.Error(w, "Please send a request body", 400)
			return
		}

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)

		var extenderBindingArgs schedulerapi.ExtenderBindingArgs
		var extenderBindingResult *schedulerapi.ExtenderBindingResult
		failed := false

		if err := json.NewDecoder(body).Decode(&extenderBindingArgs); err != nil {
			extenderBindingResult = &schedulerapi.ExtenderBindingResult{
				Error: err.Error(),
			}
			failed = true
		} else {
			util.DebugLogMsg("singleappBind ExtenderArgs =%v", extenderBindingArgs)
			extenderBindingResult = bind.Handler(extenderBindingArgs)
		}

		if len(extenderBindingResult.Error) > 0 {
			failed = true
		}

		if resultBody, err := json.Marshal(extenderBindingResult); err != nil {
			util.ErrorLogMsg("warn: Failed due to %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			errMsg := fmt.Sprintf("{'error':'%s'}", err.Error())
			w.Write([]byte(errMsg))
		} else {
			util.DebugLogMsg("extenderBindingResult = %s, bind failed: %t", string(resultBody), failed)
			w.Header().Set("Content-Type", "application/json")
			if failed {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}

			w.Write(resultBody)
		}
	}
}

func AddRoute(router *httprouter.Router, apiPrefix string) {
	bindMu.Lock()
	defer bindMu.Unlock()

	for _, p := range bindList {
		path := apiPrefix + "/" + p.Name()
		util.DefaultLog(path)
		router.POST(path, bindsRoute(p))
	}
}
