package prioritize

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io"
	"net/http"
	"sync"

	api "k8s.io/api/core/v1"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

type Interface interface {
	Name() string
	Init(k8sControl hostpathclient.HostPathController) error
	ScoreNodes(pod *api.Pod, nodes []api.Node) (*schedulerapi.HostPriorityList, error)
}

type Prioritize struct {
	Interface
}

func (p Prioritize) Handler(args schedulerapi.ExtenderArgs) (*schedulerapi.HostPriorityList, error) {
	return p.ScoreNodes(args.Pod, args.Nodes.Items)
}

var prioritizeList []*Prioritize
var prioritizeMu sync.Mutex

func Register(p *Prioritize) error {
	if p.Name() == "" {
		return fmt.Errorf("Prioritize name should not be empty")
	}
	prioritizeMu.Lock()
	defer prioritizeMu.Unlock()

	for _, existPrioritize := range prioritizeList {
		if existPrioritize.Name() == p.Name() {
			return fmt.Errorf("prioritize %s is registed", p.Name())
		}
	}
	prioritizeList = append(prioritizeList, p)
	return nil
}

func Init(k8sControl hostpathclient.HostPathController) error {
	prioritizeMu.Lock()
	defer prioritizeMu.Unlock()
	for _, p := range prioritizeList {
		if err := p.Init(k8sControl); err != nil {
			return fmt.Errorf("init prioritize %s error:%v", p.Name(), err)
		}
	}
	return nil
}

func prioritiesRoute(prioritize *Prioritize) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		if r.Body == nil {
			http.Error(w, "Please send a request body", 400)
			return
		}

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)

		var extenderArgs schedulerapi.ExtenderArgs
		var hostPriorityList *schedulerapi.HostPriorityList

		if err := json.NewDecoder(body).Decode(&extenderArgs); err != nil {
			util.ErrorLogMsg("failed to parse request due to error %v", err)
			hostPriorityList = &schedulerapi.HostPriorityList{}
		}

		if list, err := prioritize.Handler(extenderArgs); err != nil {
			util.ErrorLogMsg("failed to parse request due to error %v", err)
			hostPriorityList = &schedulerapi.HostPriorityList{}
		} else {
			hostPriorityList = list
		}

		if resultBody, err := json.Marshal(hostPriorityList); err != nil {
			util.ErrorLogMsg("Failed due to %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			errMsg := fmt.Sprintf("{'error':'%s'}", err.Error())
			w.Write([]byte(errMsg))
		} else {
			util.DebugLogMsg("info: ", prioritize.Name, " extenderFilterResult = ", string(resultBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(resultBody)
		}
	}
}

func AddRoute(router *httprouter.Router, apiPrefix string) {
	prioritizeMu.Lock()
	defer prioritizeMu.Unlock()

	for _, p := range prioritizeList {
		path := apiPrefix + "/" + p.Name()
		util.DefaultLog(path)
		router.POST(path, prioritiesRoute(p))
	}
}
