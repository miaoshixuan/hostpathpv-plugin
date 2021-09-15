package quota_manager

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/hostpath/quota-manager/quotaProvider"
	"context"
	"time"
)

type DiskQuotaInfo struct {
	// disk dev path
	DeviceName DeviceName
	// disk mountPoint
	MountPoint string
	// disk capacity
	Capacity uint64
	// disk usage
	Used uint64
	// disk allocated quotaProvider
	QuotaSize uint64
	// disk available quotaProvider
	AllocatableSize uint64
	// is disk disable
	Disabled bool
	// disk info last sync time
	lastSync time.Time
}

type DiskQuotaInfoSortItem struct {
	DeviceName    DeviceName
	MountPoint    string
	AvailableSize uint64
}

type DiskQuotaInfoList []DiskQuotaInfoSortItem

func (qdil DiskQuotaInfoList) Len() int { return len(qdil) }

// sort reverse by disk allocatable size
func (qdil DiskQuotaInfoList) Less(i, j int) bool {
	return qdil[i].AvailableSize > qdil[j].AvailableSize
}

func (qdil DiskQuotaInfoList) Swap(i, j int) {
	qdil[i], qdil[j] = qdil[j], qdil[i]
}

type DeviceName string

type VolumeId string

type PathQuotaInfo struct {
	// quotaProvider path dir
	Path string
	// quotaProvider path disk
	DeviceName DeviceName
	// quotaProvider path quoto info
	Quota quotaProvider.Quota
	// quotaProvider path last sync time
	lastSync time.Time

	// quotaProvider path owner volume id
	VolumeId VolumeId
}

type QuotaManagerInterface interface {
	ListQuotaDiskInfos() ([]DiskQuotaInfo, error)
	ListPathQuotaInfos() ([]PathQuotaInfo, error)
	SetQuotaDiskDisabledState(mountPoint string, disabled bool) error
	GetVolumeQuotaPath(ctx context.Context, volID VolumeId, capacity int64) (quotaPath string, err error)
	ExpandVolume(ctx context.Context, volID VolumeId, capacity int64) error
	ReleaseVolumePath(volId VolumeId) (err error)
}
