package predicate

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

type PredicateError struct {
	name string
	desc string
}

func newPredicateError(name, desc string) *PredicateError {
	return &PredicateError{name: name, desc: desc}
}

func (e *PredicateError) Error() string {
	return fmt.Sprintf("Predicate %s failed because %s", e.name, e.desc)
}

type Interface interface {
	Name() string
	Init(k8sControl hostpathclient.HostPathController) error
	FilterNode(pod *api.Pod, node *api.Node) (bool, error)
}

type Predicate struct {
	Interface
}

func (p Predicate) Handler(args schedulerapi.ExtenderArgs) *schedulerapi.ExtenderFilterResult {
	pod := args.Pod
	nodes := args.Nodes.Items
	canSchedule := make([]api.Node, 0, len(nodes))
	canNotSchedule := make(map[string]string)

	for _, node := range nodes {
		result, err := p.FilterNode(pod, &node)
		if err != nil {
			canNotSchedule[node.Name] = err.Error()
		} else {
			if result {
				canSchedule = append(canSchedule, node)
			}
		}
	}

	result := schedulerapi.ExtenderFilterResult{
		Nodes: &api.NodeList{
			Items: canSchedule,
		},
		FailedNodes: canNotSchedule,
		Error:       "",
	}

	return &result
}

var predicateList []*Predicate
var predicateMu sync.Mutex

func Register(p *Predicate) error {
	if p.Name() == "" {
		return fmt.Errorf("predicate name should not be empty")
	}
	predicateMu.Lock()
	defer predicateMu.Unlock()
	for _, existPredicate := range predicateList {
		if existPredicate.Name() == p.Name() {
			return fmt.Errorf("Predicate %s is registed", p.Name)
		}
	}
	predicateList = append(predicateList, p)
	return nil
}

func Init(k8sControl hostpathclient.HostPathController) error {
	predicateMu.Lock()
	defer predicateMu.Unlock()
	for _, p := range predicateList {
		if err := p.Init(k8sControl); err != nil {
			return fmt.Errorf("init predicate %s error:%v", p.Name(), err)
		}
	}
	return nil
}

func predicateRoute(predicate *Predicate) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		if r.Body == nil {
			http.Error(w, "Please send a request body", 400)
			return
		}

		var buf bytes.Buffer
		body := io.TeeReader(r.Body, &buf)

		var extenderArgs schedulerapi.ExtenderArgs
		var extenderFilterResult *schedulerapi.ExtenderFilterResult

		if err := json.NewDecoder(body).Decode(&extenderArgs); err != nil {

			util.ErrorLogMsg("failed to parse request due to error %v", err)
			extenderFilterResult = &schedulerapi.ExtenderFilterResult{
				Nodes:       nil,
				FailedNodes: nil,
				Error:       err.Error(),
			}
		} else {
			util.DebugLogMsg("singleappfilter ExtenderArgs =%v", extenderArgs)
			extenderFilterResult = predicate.Handler(extenderArgs)
		}

		if resultBody, err := json.Marshal(extenderFilterResult); err != nil {
			// panic(err)
			util.ErrorLogMsg("Failed due to %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			errMsg := fmt.Sprintf("{'error':'%s'}", err.Error())
			w.Write([]byte(errMsg))
		} else {
			util.DebugLogMsg("info: ", predicate.Name, " extenderFilterResult = ", string(resultBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(resultBody)
		}
	}
}

func AddRoute(router *httprouter.Router, apiPrefix string) {
	predicateMu.Lock()
	defer predicateMu.Unlock()

	for _, p := range predicateList {
		path := apiPrefix + "/" + p.Name()
		util.DefaultLog(path)
		router.POST(path, predicateRoute(p))
	}
}
