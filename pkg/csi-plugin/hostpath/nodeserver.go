package hostpath

import (
	csicommon "ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/csi-common"
	quotaMng "ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/hostpath/quota-manager"
	hpClient "ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
	"os"
)

type NodeServer struct {
	*csicommon.DefaultNodeServer
	hpClient hpClient.HostPathClientInterface
	quotaMng quotaMng.QuotaManagerInterface
	mounter  mount.Interface

	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error
	VolumeLocks *util.VolumeLocks
}

// NodeGetCapabilities returns the supported capabilities of the node server
func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	util.DebugLog(ctx, "get node Capabilities")
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

// NodeStageVolume mounts the volume to a staging path on the node.
func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	// hostPath pv no need stage ,do basic check only
	if err := util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}
	util.DebugLog(ctx, "finished node stageVolume: %s", req.VolumeId)
	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path
func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// hostPath pv no need stage ,do basic check only
	if err := util.ValidateNodeUnstageVolumeRequest(req); err != nil {
		return nil, err
	}
	util.DebugLog(ctx, "finished node unStageVolume: %s", req.VolumeId)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume mounts the volume mounted to the device path to the target
// path
func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	// Check arguments
	if err := util.ValidateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	targetPath := req.GetTargetPath()
	volID := req.GetVolumeId()

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	// Check if that target path exists properly
	notMnt, err := mount.IsNotMountPoint(ns.mounter, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create a directory
			if err = util.CreateNewDir(targetPath); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		util.DebugLog(ctx, "hostPath: volume %s is already mounted to %s, skipping", volID, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	volume := ns.hpClient.GetHostPathPVByVolumeId(volID)
	if volume == nil {
		return nil, status.Errorf(codes.Internal, "volume %s not found", volID)
	}

	quotaPath, err := ns.quotaMng.GetVolumeQuotaPath(ctx, quotaMng.VolumeId(volume.GetVolumeId()), volume.GetCapacity())

	if err != nil {
		util.ErrorLog(ctx, "field to get a quotaPath for volume %s , err: %v", volume.GetName(), err)
		return nil, err
	}

	if err := ns.mountVolume(ctx, quotaPath, req); err != nil {
		return nil, err
	}

	util.DebugLog(ctx, "hostPath: successfully mounted stagingPath %s to targetPath %s", quotaPath, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	// Check arguments
	if err := util.ValidateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, err
	}
	targetPath := req.GetTargetPath()
	volID := req.GetVolumeId()

	var vi util.CSIIdentifier
	if err := vi.DecomposeCSIID(volID); err != nil {
		util.ErrorLog(ctx, "hostPath: failed decode volId %s ,%v", volID, err)
		return nil, status.Errorf(codes.Internal, "failed decode volId %s ", volID)
	}

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	notMnt, err := mount.IsNotMountPoint(ns.mounter, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// targetPath has already been deleted
			util.DebugLog(ctx, "targetPath: %s has already been deleted", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if !notMnt {
		if err = ns.mounter.Unmount(targetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// remove ephemeral volume path when unbound from targetPath
	if vi.Ephemeral {
		if err := ns.quotaMng.ReleaseVolumePath(quotaMng.VolumeId(volID)); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err = os.RemoveAll(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	util.DebugLog(ctx, "hostPath: successfully unbound volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil

}

func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	volID := req.GetVolumeId()
	if len(volID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume path must be provided")
	}

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "capacityRange cannot be empty")
	}

	if err := ns.quotaMng.ExpandVolume(ctx, quotaMng.VolumeId(volID), capRange.GetRequiredBytes()); err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("failed expand volume %s ,err %v", volID, err))
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *NodeServer) mountVolume(ctx context.Context, quotaPath string, req *csi.NodePublishVolumeRequest) error {
	// Publish Path
	readOnly := req.GetReadonly()
	mountOptions := []string{"bind"}
	targetPath := req.GetTargetPath()

	mountOptions = csicommon.ConstructMountOptions(mountOptions, req.GetVolumeCapability())

	util.DebugLog(ctx, "target %v\nquotaPath %v\nreadonly %v\nmountflags %v\n",
		targetPath, quotaPath, readOnly, mountOptions)

	if readOnly {
		mountOptions = append(mountOptions, "ro")
	}

	if err := ns.mounter.Mount(quotaPath, targetPath, "", mountOptions); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}
