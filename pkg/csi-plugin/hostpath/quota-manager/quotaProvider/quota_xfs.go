package quotaProvider

import (
	"fmt"
	"os"
	"path"
	"syscall"
)

const (
	LinuxXfsMagic = 0x58465342
)

type QuotaVolumeProvider struct {
	// project quotaProvider id allocator
	projectIdAllocator AllocatorInterface

	// quotaProvider path and project id local cache
	quotaPathAndId map[string]uint32
}

func NewXFSVolumeQuotaProvider(startId, idNumber int) LinuxVolumeQuotaProvider {
	return &QuotaVolumeProvider{
		projectIdAllocator: NewAllocator(startId, idNumber),
		quotaPathAndId:     make(map[string]uint32),
	}
}

func (v QuotaVolumeProvider) Supported(mountPoint string, devicePath string) bool {
	mountPoint = path.Clean(mountPoint)
	return supported(mountPoint, devicePath, LinuxXfsMagic)
}

func (v QuotaVolumeProvider) GetProjectQuota(quotaPath string, devicePath string) (quota *Quota, err error) {
	// check local cache
	if projectId, ok := v.quotaPathAndId[quotaPath]; ok {
		return getProjectQuota(devicePath, projectId)
	}

	// not in local cache, bootstrap load project id by system call
	projectId, err := getProjectId(quotaPath)
	if err != nil {
		return nil, err
	}
	if err := v.projectIdAllocator.Allocate(projectId); err != nil {
		return nil, err
	}

	// update local cache
	v.quotaPathAndId[quotaPath] = projectId

	return getProjectQuota(devicePath, projectId)
}

func (v QuotaVolumeProvider) SetProjectQuota(quotaPath, devicePath string, hardQuota, softQuota uint64) (err error) {
	// check local cache
	projectId, ok := v.quotaPathAndId[quotaPath]

	if ok {
		return nil
	}

	// allocate new projectId
	projectId, err = v.projectIdAllocator.AllocateNext()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = v.projectIdAllocator.Release(projectId)
			delete(v.quotaPathAndId, quotaPath)
		}
	}()

	if err = setProjectId(quotaPath, projectId); err == nil {
		if err = setProjectQuota(devicePath, projectId, hardQuota, softQuota); err == nil {
			// set local cache
			v.quotaPathAndId[quotaPath] = projectId
		}
	}

	return err
}

func (v QuotaVolumeProvider) DeleteProjectQuota(quotaPath string) (err error) {
	// check local cache
	projectId, ok := v.quotaPathAndId[quotaPath]
	if ok {
		// release allocated projectId
		err = v.projectIdAllocator.Release(projectId)
		if err != nil {
			return err
		}
		delete(v.quotaPathAndId, quotaPath)
	}

	// delete file when exist
	if _, err := os.Stat(quotaPath); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(quotaPath)
}

func (v QuotaVolumeProvider) GetQuotaDiskInfo(mountPoint string) (quota *Quota, err error) {
	fs := syscall.Statfs_t{}

	if err := syscall.Statfs(mountPoint, &fs); err != nil {
		return nil, fmt.Errorf("failed to get quotaProvider for mountPoint: %s . err: %v", mountPoint, err)
	}
	return &Quota{
		HardQuota: fs.Blocks * uint64(fs.Bsize),
		SoftQuota: fs.Blocks * uint64(fs.Bsize),
		Used:      (fs.Blocks - fs.Bfree) * uint64(fs.Bsize),
	}, nil
}
