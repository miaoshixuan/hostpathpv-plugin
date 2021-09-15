package hostpath

import (
	csicommon "ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/csi-common"
	quotamanager "ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/hostpath/quota-manager"
	hpClient "ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/mount"
	"time"
)

type Driver struct {
	cd *csicommon.CSIDriver

	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
}

func NewDriver() *Driver {
	return &Driver{}
}

// NewIdentityServer initialize a identity server for hostpath CSI driver
func NewIdentityServer(d *csicommon.CSIDriver) *IdentityServer {
	return &IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// NewControllerServer initialize a k8scontroller server for hostpath CSI driver
func NewControllerServer(d *csicommon.CSIDriver, mStore MetaStoreInterFace) *ControllerServer {
	return &ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		VolumeLocks:             util.NewVolumeLocks(),
		mStore:                  mStore,
	}
}

// NewNodeServer initialize a node server for hostpath CSI driver.
func NewNodeServer(d *csicommon.CSIDriver, hpClient hpClient.HostPathClientInterface, nodeName string) (*NodeServer, error) {
	mounter := mount.New("")
	return &NodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d, nodeName, nil),
		VolumeLocks:       util.NewVolumeLocks(),
		hpClient:          hpClient,
		quotaMng:          quotamanager.NewXfsQuotaManager("/xfs"),
		mounter:           mounter,
	}, nil
}

func (hp *Driver) Run(conf *util.Config, client kubernetes.Interface) {
	var err error
	util.DefaultLog("hostPath CSI Driver version: %v", util.DriverVersion)

	// Initialize default library driver
	hp.cd = csicommon.NewCSIDriver(hpClient.HostPathCsiDriverName, util.DriverVersion, conf.NodeID)
	if hp.cd == nil {
		util.FatalLogMsg("Failed to initialize CSI Driver.")
	}
	hp.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	})
	hp.cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	})

	// Create GRPC servers
	hp.ids = NewIdentityServer(hp.cd)

	var metaStore MetaStoreInterFace
	var hpclient hpClient.HostPathClientInterface
	if conf.IsNodeServer || conf.IsControllerServer {
		metaStore, err = NewConfigMapMetaStore(client, conf.MetaConfigMapName, conf.MetaConfigMapNamespace)
		if err != nil {
			util.FatalLogMsg("failed to start node server, err %v\n", err)
			return
		}
		hpclient = hpClient.NewHostPathClient(client)
	}

	if conf.IsNodeServer {
		hp.ns, err = NewNodeServer(hp.cd, hpclient, conf.NodeID)
		if err != nil {
			util.FatalLogMsg("failed to start node server, err %v\n", err)
			return
		}

		syncWork := &SyncWorker{
			hpClient:               hpclient,
			mStore:                 metaStore,
			quotaMng:               hp.ns.quotaMng,
			shouldDeleteVolIds:     make([]quotamanager.VolumeId, 0, 10),
			nodeName:               conf.NodeID,
			lastSyncQuotaInfoCache: make(map[quotamanager.VolumeId]quotamanager.PathQuotaInfo),
		}
		syncWork.Start(30 * time.Second)
	}

	if conf.IsControllerServer {
		hp.cs = NewControllerServer(hp.cd, metaStore)
	}

	s := csicommon.NewNonBlockingGRPCServer()

	s.Start(conf.Endpoint, conf.HistogramOption, hp.ids, hp.cs, hp.ns, false)
	s.Wait()
}
