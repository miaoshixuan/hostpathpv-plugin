package quota_manager

import (
	"context"
	"errors"
)

type FakeQuotaManager struct{}

var (
	errUnimplemented = errors.New("function is not implemented")
)

func (fqm *FakeQuotaManager) ListQuotaDiskInfos() ([]DiskQuotaInfo, error) {
	return nil, errUnimplemented
}

func (fqm *FakeQuotaManager) ListPathQuotaInfos() ([]PathQuotaInfo, error) {
	return nil, errUnimplemented
}

func (fqm *FakeQuotaManager) GetQuotaInfoByVolumeId(volID VolumeId) (*PathQuotaInfo, error) {
	return nil, errUnimplemented
}

func (fqm *FakeQuotaManager) SetQuotaDiskDisabledState(diskMountPath string, disabled bool) error {
	return errUnimplemented
}

func (fqm *FakeQuotaManager) GetVolumeQuotaPath(ctx context.Context, volId VolumeId, capacity int64) (quotaPath string, err error) {
	return "", errUnimplemented
}

func (fqm *FakeQuotaManager) ExpandVolume(ctx context.Context, volID VolumeId, capacity int64) (err error) {
	return errUnimplemented
}

func (fqm *FakeQuotaManager) ReleaseVolumePath(volId VolumeId) (err error) { return errUnimplemented }
