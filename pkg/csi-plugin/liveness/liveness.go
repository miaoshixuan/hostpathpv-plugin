package liveness

import (
	"context"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"time"

	"ShixuanMiao/hostpathpv-plugin/pkg/util"

	connlib "github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

var (
	liveness = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "csi",
		Name:      "liveness",
		Help:      "Liveness Probe",
	})
)

func getLiveness(timeout time.Duration, csiConn *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	klog.Info("Sending probe request to CSI driver")
	ready, err := rpc.Probe(ctx, csiConn)
	if err != nil {
		liveness.Set(0)
		klog.Errorf("health check failed: %v", err)
		return
	}

	if !ready {
		liveness.Set(0)
		klog.Error("driver responded but is not ready")
		return
	}
	liveness.Set(1)
	klog.Infof("Health check succeeded")
}

func recordLiveness(endpoint, drivername string, pollTime, timeout time.Duration) {
	liveMetricsManager := metrics.NewCSIMetricsManager(drivername)
	// register prometheus metrics
	err := prometheus.Register(liveness)
	if err != nil {
		klog.Fatalln(err)
	}

	csiConn, err := connlib.Connect(endpoint, liveMetricsManager)
	if err != nil {
		// connlib should retry forever so a returned error should mean
		// the grpc client is misconfigured rather than an error on the network
		klog.Fatalf("failed to establish connection to CSI driver: %v", err)
	}

	// get liveness periodically
	ticker := time.NewTicker(pollTime)
	defer ticker.Stop()
	for range ticker.C {
		getLiveness(timeout, csiConn)
	}
}

// Run starts liveness collection and prometheus endpoint.
func Run(conf *util.Config) {
	util.ExtendedLogMsg("Liveness Running")

	// start liveness collection
	go recordLiveness(conf.Endpoint, conf.DriverName, conf.PollTime, conf.PoolTimeout)
}
