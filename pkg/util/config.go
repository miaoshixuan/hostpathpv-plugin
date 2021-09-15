package util

import "time"

// variables which will be set during the build time
var (
	// GitCommit tell the latest git commit image is built from
	GitCommit string
	// DriverVersion which will be driver version
	DriverVersion string
)

// Config holds the parameters list which can be configured
type Config struct {
	KubeConfig             string // kube config file path
	Endpoint               string // CSI endpoint
	DriverName             string // name of the driver
	NodeID                 string // node id
	PluginPath             string // location of cephcsi plugin
	MetaConfigMapName      string // name of meta
	MetaConfigMapNamespace string // namespace of meta

	// metrics related flags
	MetricsPath     string        // path of prometheus endpoint where metrics will be available
	HistogramOption string        // Histogram option for grpc metrics, should be comma separated value, ex:= "0.5,2,6" where start=0.5 factor=2, count=6
	PollTime        time.Duration // time interval in seconds between each poll
	PoolTimeout     time.Duration // probe timeout in seconds

	IsControllerServer bool // if set to true start provisoner server
	IsNodeServer       bool // if set to true start node server

}
