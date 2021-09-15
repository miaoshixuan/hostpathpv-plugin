package quota_manager

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/hostpath/quota-manager/quotaProvider"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"context"
	"fmt"
	"path"
	"sort"
	"sync"
	"time"
)

type XFSQuotaManager struct {
	// project root path
	quotaRootPath string

	mu sync.Mutex

	diskQuotaInfos map[DeviceName]*DiskQuotaInfo

	quotas map[VolumeId]*PathQuotaInfo

	maxCacheTime time.Duration

	quotaProvider quotaProvider.LinuxVolumeQuotaProvider
}

func NewXfsQuotaManager(quotaRootPath string) QuotaManagerInterface {
	ret := &XFSQuotaManager{
		quotaRootPath:  path.Clean(quotaRootPath),
		quotas:         make(map[VolumeId]*PathQuotaInfo),
		diskQuotaInfos: make(map[DeviceName]*DiskQuotaInfo),
		maxCacheTime:   60 * time.Second,
		quotaProvider:  quotaProvider.NewXFSVolumeQuotaProvider(ProjectIdStart, ProjectIdNum),
	}
	if ok := util.CheckDirExists(quotaRootPath); !ok {
		util.ErrorLogMsg("QuotaManager init failed: quotaRootPath is not exist")
		return &FakeQuotaManager{}
	}

	if err := ret.restoreQuotaInfo(); err != nil {
		util.ErrorLogMsg("QuotaManager init failed when restoreQuotaInfo. err: %v ", err)
		return &FakeQuotaManager{}
	}
	util.UsefulLog(context.TODO(), "successful init quotaManager")
	return ret
}

func (m *XFSQuotaManager) ListQuotaDiskInfos() ([]DiskQuotaInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := make([]DiskQuotaInfo, 0, len(m.diskQuotaInfos))
	for _, disk := range m.diskQuotaInfos {
		if time.Since(disk.lastSync) > m.maxCacheTime {
			quota, err := m.quotaProvider.GetQuotaDiskInfo(disk.MountPoint)
			if err != nil {
				util.ErrorLogMsg("failed to load disk usage info of disk: %s mountPoint: %s .", disk.DeviceName, disk.MountPoint)
				return nil, err
			} else {
				disk.Used = quota.Used
				disk.lastSync = time.Now()
			}
		}
		ret = append(ret, *disk)
		util.DebugLogMsg("finish load disk usage info of disk: %s", disk.DeviceName)
	}
	return ret, nil
}

func (m *XFSQuotaManager) ListPathQuotaInfos() ([]PathQuotaInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := make([]PathQuotaInfo, 0, len(m.quotas))
	for _, quotaInfo := range m.quotas {
		if time.Since(quotaInfo.lastSync) > m.maxCacheTime {
			quota, err := m.quotaProvider.GetProjectQuota(quotaInfo.Path, string(quotaInfo.DeviceName))
			if err != nil {
				util.ErrorLogMsg("failed to load quota path usage info of path: %s", quotaInfo.Path)
				return nil, err
			} else {
				quotaInfo.Quota.Used = quota.Used
				quotaInfo.lastSync = time.Now()
			}
		}
		ret = append(ret, *quotaInfo)
		util.DebugLogMsg("finish load  usage info of quota path: %s", quotaInfo.VolumeId)
	}
	return ret, nil
}

func (m *XFSQuotaManager) SetQuotaDiskDisabledState(mountPoint string, disabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, diskInfo := range m.diskQuotaInfos {
		if diskInfo.MountPoint == mountPoint {
			if err := setQuotaDiskDisableState(diskInfo.MountPoint, disabled); err != nil {
				return fmt.Errorf("failed to set device: %s disabled state to %t. err: %v", mountPoint, disabled, err)
			}
			// update local cache
			diskInfo.Disabled = disabled
			util.DefaultLog("successful set disk : %s to %t", mountPoint, disabled)
			return nil
		}
	}
	return fmt.Errorf("disk of mountPoint %s not found in local cache", mountPoint)
}

func (m *XFSQuotaManager) GetVolumeQuotaPath(ctx context.Context, volId VolumeId, capacity int64) (quotaPath string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// find allocated volume path
	if quotaInfo, ok := m.quotas[volId]; ok {
		return quotaInfo.Path, nil
	}
	util.DefaultLog("do not find volume %s in local cache,try to allocate new path", volId)

	quotaSize := uint64(capacity)

	// try to allocate and create a new quotaProvider path
	deviceName, quotaPath, err := m.allocateNewQuotaDir(ctx, volId, quotaSize)
	if err != nil {
		util.ErrorLog(ctx, "failed allocate quotaPath %v", err)
		return "", fmt.Errorf("failed allocate quotaPath, err %s", err)
	}
	util.DefaultLog("allocate path %s on disk %s for volume %s", quotaPath, deviceName, volId)

	diskInfo, ok := m.diskQuotaInfos[deviceName]
	if !ok {
		return "", fmt.Errorf("disk %s not found in local cache", deviceName)
	}

	//set setProjectQuota for quotaProvider volume path
	if err := m.quotaProvider.SetProjectQuota(quotaPath, string(deviceName), quotaSize, quotaSize); err != nil {
		defer util.DeleteFile(quotaPath)
		return "", fmt.Errorf("set project quotaProvider with deviceName: %s for path: %s  err: %v", deviceName, quotaPath, err)
	}

	util.DefaultLog("success set project quota for volume %s ", volId)

	// add local cache
	m.quotas[volId] = &PathQuotaInfo{
		Path:       quotaPath,
		VolumeId:   volId,
		DeviceName: deviceName,
		Quota: quotaProvider.Quota{
			HardQuota: quotaSize,
			SoftQuota: quotaSize,
			Used:      0,
		},
		lastSync: time.Now(),
	}
	diskInfo.QuotaSize += quotaSize
	diskInfo.AllocatableSize = diskInfo.Capacity - diskInfo.QuotaSize
	return quotaPath, nil
}

func (m *XFSQuotaManager) ExpandVolume(ctx context.Context, volID VolumeId, capacity int64) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	quotaInfo, ok := m.quotas[volID]
	if !ok {
		return fmt.Errorf("volume %s not found in local cache", volID)
	}

	diskInfo, ok := m.diskQuotaInfos[quotaInfo.DeviceName]
	if !ok {
		return fmt.Errorf("disk %s not found in local cache", quotaInfo.DeviceName)
	}

	oldSize := quotaInfo.Quota.HardQuota
	quotaSize := uint64(capacity)

	if oldSize > quotaSize {
		return fmt.Errorf("reduce path: %s quota from %d to %d is not support ", quotaInfo.Path, oldSize, quotaSize)
	}

	if diskInfo.AllocatableSize < (quotaSize - oldSize) {
		return fmt.Errorf("disk %s has not enough quotaProvider to set", quotaInfo.DeviceName)
	}

	if err := m.quotaProvider.SetProjectQuota(quotaInfo.Path, string(quotaInfo.DeviceName), quotaSize, quotaSize); err != nil {
		return fmt.Errorf("faild to setProjectQuota %d for path %s err:%v", quotaSize, quotaInfo.Path, err)
	}

	//set local cache
	diskInfo.QuotaSize = diskInfo.QuotaSize + quotaSize - oldSize
	diskInfo.AllocatableSize = diskInfo.Capacity - diskInfo.QuotaSize
	quotaInfo.Quota.SoftQuota = quotaSize
	quotaInfo.Quota.HardQuota = quotaSize
	util.DefaultLog("success expand project quota from %d to %d of volume %s", oldSize, quotaSize, volID)
	return nil
}

func (m *XFSQuotaManager) ReleaseVolumePath(volId VolumeId) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if quotaPath, ok := m.quotas[volId]; ok {
		diskInfo, ok := m.diskQuotaInfos[quotaPath.DeviceName]
		if !ok {
			return fmt.Errorf("disk %s for path: %s not found in local cache", quotaPath.DeviceName, quotaPath.Path)
		}
		if err := m.quotaProvider.DeleteProjectQuota(quotaPath.Path); err != nil {
			return err
		}

		delete(m.quotas, volId)
		diskInfo.QuotaSize = diskInfo.QuotaSize - quotaPath.Quota.HardQuota
		diskInfo.AllocatableSize = diskInfo.Capacity - diskInfo.QuotaSize
	}
	util.DefaultLog("success release quota path of volume %s", volId)
	return nil
}

func (m *XFSQuotaManager) allocateNewQuotaDir(ctx context.Context, volId VolumeId, hardQuota uint64) (diskName DeviceName, quotaPath string, err error) {
	dirName := getQuotaPathName(volId)
	availableDisk := make(DiskQuotaInfoList, 0, len(m.diskQuotaInfos))
	for _, disk := range m.diskQuotaInfos {
		// skip disabled disk
		if disk.Disabled == true {
			continue
		}

		// skip disk does not have enough allocatable size
		if disk.AllocatableSize < hardQuota {
			continue
		}

		availableDisk = append(availableDisk, DiskQuotaInfoSortItem{
			DeviceName:    disk.DeviceName,
			MountPoint:    disk.MountPoint,
			AvailableSize: disk.AllocatableSize,
		})
	}

	if len(availableDisk) == 0 {
		return "", "", fmt.Errorf("has no disk can match the hardquota %d", hardQuota)
	}

	// sort reverse by disk allocatable size
	sort.Sort(availableDisk)

	diskName = availableDisk[0].DeviceName
	quotaPath = path.Join(availableDisk[0].MountPoint, dirName)

	if util.IsPathExist(quotaPath) == false {
		if err := util.MkDir(quotaPath); err != nil {
			util.ErrorLog(ctx, "failed to create quotaPath %s %v", quotaPath, err)
			return "", "", fmt.Errorf("failed allocate quotaPath: %s , err: %v", quotaPath, err)
		}
	}
	return diskName, quotaPath, nil
}

//restore all quotaProvider info from quotaPath when start
func (m *XFSQuotaManager) restoreQuotaInfo() error {

	//load quotaProvider disks info
	for _, mountPoint := range util.GetSubDirs(m.quotaRootPath) {
		if isMnt, _ := util.IsMountPoint(mountPoint); !isMnt {
			util.DefaultLog("path: %s looks like not a mountPoint,skip", mountPoint)
			continue
		}
		deviceName, err := util.GetDevForPath(mountPoint)
		if err != nil {
			util.ErrorLogMsg("failed get dev for path: %s. err: %v", mountPoint, err)
			return err
		}
		if !m.quotaProvider.Supported(mountPoint, deviceName) {
			util.WarningLogMsg("mountPoint: %s for device %s not support project quota ,skip", mountPoint, deviceName)
		}

		device := DeviceName(deviceName)
		if quota, err := m.quotaProvider.GetQuotaDiskInfo(mountPoint); err == nil {
			m.diskQuotaInfos[device] = &DiskQuotaInfo{
				DeviceName:      device,
				MountPoint:      mountPoint,
				Capacity:        quota.HardQuota,
				Used:            quota.Used,
				QuotaSize:       0,
				AllocatableSize: quota.HardQuota,
				Disabled:        getDiskQuotaDisableState(mountPoint),
			}
		} else {
			util.ErrorLogMsg("failed load quota info of dev: %s mount point: %s. err: %v", device, mountPoint, err)
			return err
		}
		util.DefaultLog("finish load disk quota info of device %s. detail: %v", device, m.diskQuotaInfos[device])
	}

	// load quotaProvider path
	for deviceName, diskInfo := range m.diskQuotaInfos {
		for _, quotaPath := range util.GetSubDirs(diskInfo.MountPoint) {
			volID, err := parseQuotaPathInfoFromDirName(quotaPath)
			if err != nil {
				util.ErrorLogMsg("failed parse volume from path %s skip.err: %v", quotaPath, err)
				continue
			}

			quota, err := m.quotaProvider.GetProjectQuota(quotaPath, string(deviceName))
			if err != nil {
				util.ErrorLogMsg("failed to read quotaProvider info from path: %s ,deviceName: %s . err: %v", quotaPath, deviceName, err)
				return err
			}

			// update disk quota info
			diskInfo.QuotaSize += quota.HardQuota

			// set quota path info
			m.quotas[volID] = &PathQuotaInfo{
				Path:       quotaPath,
				DeviceName: deviceName,
				Quota:      *quota,
				lastSync:   time.Now(),
				VolumeId:   volID,
			}
			util.DefaultLog("successful load quotaPath quota info of path %s. detail: %v", quotaPath, m.quotas[volID])
		}
		// update disk quota sync time
		diskInfo.lastSync = time.Now()
	}
	util.DefaultLog("finish load all quota info ")
	util.DebugLogMsg("diskQuota info %v , quotaPath info %v", m.diskQuotaInfos, m.quotas)
	return nil
}
