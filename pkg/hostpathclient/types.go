package hostpathclient

type PVUsageInfo struct {
	NodeName string
	Path     string
	Capacity int64
	Used     int64
}

type DiskInfo struct {
	// disk mount point
	MountPoint string
	// disk total size
	Capacity int64
	//disk allocated quota size
	QuotaSize int64
	//disk current usage
	Used int64
	// disk allocatable quota size
	Allocatable int64
	// is disk disabled
	Disabled bool
}

type NodeUsageInfo struct {
	// node disk total size
	Capacity int64
	// node  quota total allocated size
	QuotaSize int64
	// node disk  total used size
	Used int64
	// node disk  total free size
	AvailableSize int64
	// node allocatable quota size
	Allocatable int64
	// node disk quota info
	DiskStatus DiskInfoList
}

// disk quota info sort struct
type DiskInfoList []DiskInfo

func (l DiskInfoList) Len() int { return len(l) }

// sort reverse by disk allocatable size
func (l DiskInfoList) Less(i, j int) bool {
	if l[i].Allocatable != l[j].Allocatable {
		return l[i].Allocatable > l[j].Allocatable
	} else {
		return l[i].MountPoint < l[j].MountPoint
	}
}

func (l DiskInfoList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
