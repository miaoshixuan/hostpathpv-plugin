package util

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ValidateNodeStageVolumeRequest validates the node stage request.
func ValidateNodeStageVolumeRequest(req *csi.NodeStageVolumeRequest) error {
	// hostPath pv no need stage ,do basic check only
	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	if len(req.GetVolumeId()) == 0 {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if len(req.GetStagingTargetPath()) == 0 {
		return status.Error(codes.InvalidArgument, "staging target path missing in request")
	}

	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	// validate stagingpath exists
	ok := CheckDirExists(req.GetStagingTargetPath())
	if !ok {
		return status.Errorf(codes.InvalidArgument, "staging path %s does not exists on node", req.GetStagingTargetPath())
	}
	return nil
}

// ValidateNodeUnstageVolumeRequest validates the node unstage request.
func ValidateNodeUnstageVolumeRequest(req *csi.NodeUnstageVolumeRequest) error {
	if len(req.GetVolumeId()) == 0 {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if len(req.GetStagingTargetPath()) == 0 {
		return status.Error(codes.InvalidArgument, "staging target path missing in request")
	}
	return nil
}

// ValidateNodePublishVolumeRequest validates the node publish request.
func ValidateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "volume capability missing in request")
	}

	if len(req.GetVolumeId()) == 0 {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if len(req.GetTargetPath()) == 0 {
		return status.Error(codes.InvalidArgument, "target path missing in request")
	}

	if len(req.GetStagingTargetPath()) == 0 {
		return status.Error(codes.InvalidArgument, "staging target path missing in request")
	}
	return nil
}

// ValidateNodeUnpublishVolumeRequest validates the node unpublish request.
func ValidateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if len(req.GetVolumeId()) == 0 {
		return status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	if len(req.GetTargetPath()) == 0 {
		return status.Error(codes.InvalidArgument, "target path missing in request")
	}

	return nil
}
