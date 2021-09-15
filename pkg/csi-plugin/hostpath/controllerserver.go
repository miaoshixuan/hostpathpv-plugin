package hostpath

import (
	csicommon "ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/csi-common"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ControllerServer struct of hostpath CSI driver with supported methods of CSI
// k8scontroller server spec.
type ControllerServer struct {
	*csicommon.DefaultControllerServer

	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID/volume name) return an Aborted error
	VolumeLocks *util.VolumeLocks

	mStore MetaStoreInterFace
}

func (cs *ControllerServer) validateCreateVolumeRequest(ctx context.Context, req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		util.ErrorLog(ctx, "invalid create volume req: %v", protosanitizer.StripSecrets(req))
		return err
	}
	// Check sanity of request Name, Volume Capabilities
	if req.Name == "" {
		return status.Error(codes.InvalidArgument, "volume Name cannot be empty")
	}
	if req.VolumeCapabilities == nil {
		return status.Error(codes.InvalidArgument, "volume Capabilities cannot be empty")
	}
	for _, caps := range req.VolumeCapabilities {
		if caps.GetBlock() != nil {
			return status.Error(codes.Unimplemented, "block volume not supported")
		}
	}
	return nil
}

// CreateVolume creates the volume in backend
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := cs.validateCreateVolumeRequest(ctx, req); err != nil {
		return nil, err
	}

	if acquired := cs.VolumeLocks.TryAcquire(req.GetName()); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, req.GetName())
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetName())
	}
	defer cs.VolumeLocks.Release(req.GetName())

	var volumeId string
	var ephemeral bool

	// Volume Size - Default is 100 MiB
	volSizeBytes := int64(util.MiB * 100)
	if req.GetCapacityRange() != nil {
		volSizeBytes = req.GetCapacityRange().GetRequiredBytes()
	}

	// always round up the request size in bytes to the nearest MiB/GiB
	volSizeBytes = util.RoundOffBytes(volSizeBytes)

	volOptions := req.GetParameters()
	ephemeral, err := strconv.ParseBool(volOptions["ephemeral"])
	if err != nil {
		util.ErrorLog(ctx, "failed parse volume ephemeral options. %v", err)
		return nil, status.Error(codes.InvalidArgument, "invalid ephemeral volume options in request")
	}

	vi := util.CSIIdentifier{
		EncodingVersion: util.VolIDVersion,
		VolumeName:      req.GetName(),
		Ephemeral:       ephemeral,
	}

	if volumeId, err = vi.ComposeCSIID(); err != nil {
		util.ErrorLog(ctx, "failed get volumeId. %v", err)
		return nil, status.Error(codes.Internal, "failed get volumeId")
	}

	err = cs.mStore.AddMetaInfo(volumeId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed add volumeId to meta store")
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeId,
			CapacityBytes: volSizeBytes,
			VolumeContext: req.GetParameters(),
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

// DeleteVolume deletes the volume in backend and removes the volume metadata
// from store
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		util.WarningLog(ctx, "invalid delete volume req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	err := cs.mStore.UnsetMetaInfo(volumeID)
	if err != nil {
		util.ErrorLog(ctx, "failed remove volumeId from meta store, %v", err)
		return nil, status.Error(codes.InvalidArgument, "failed remove from meta store")
	}

	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerExpandVolume expand HostPath Volumes on demand based on resizer request.
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		util.ErrorLog(ctx, "invalid expand volume req: %v", protosanitizer.StripSecrets(req))
		return nil, err
	}

	volID := req.GetVolumeId()
	if volID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID cannot be empty")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "capacityRange cannot be empty")
	}

	// always round up the request size in bytes to the nearest MiB/GiB
	volSize := util.RoundOffBytes(req.GetCapacityRange().GetRequiredBytes())

	// lock out parallel requests against the same volume ID
	if acquired := cs.VolumeLocks.TryAcquire(volID); !acquired {
		util.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer cs.VolumeLocks.Release(volID)

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         volSize,
		NodeExpansionRequired: true,
	}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested
// are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	if len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "empty volume capabilities in request")
	}

	for _, capability := range req.VolumeCapabilities {
		if capability.GetBlock() != nil {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: ""}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}
