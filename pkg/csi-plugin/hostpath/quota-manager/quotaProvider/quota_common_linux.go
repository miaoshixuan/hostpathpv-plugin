package quotaProvider

/*
#include <stdlib.h>
#include <dirent.h>
#include <linux/fs.h>
#include <linux/quota.h>
#include <linux/dqblk_xfs.h>
#include <errno.h>

//For some definitions, check out the kernel tree.
//From linux/fs/xfs/libxfs/xfs_quota_defs.h
//https://github.com/torvalds/linux/blob/master/fs/xfs/libxfs/xfs_quota_defs.h
//#define XFS_DQ_PROJ          0x0002
#ifndef FS_XFLAG_PROJINHERIT
struct fsxattr {
	__u32		fsx_xflags;
	__u32		fsx_extsize;
	__u32		fsx_nextents;
	__u32		fsx_projid;
	unsigned char	fsx_pad[12];
};
#define FS_XFLAG_PROJINHERIT	0x00000200
#endif

#ifndef FS_IOC_FSGETXATTR
#define FS_IOC_FSGETXATTR		_IOR ('X', 31, struct fsxattr)
#endif

#ifndef FS_IOC_FSSETXATTR
#define FS_IOC_FSSETXATTR		_IOW ('X', 32, struct fsxattr)
#endif

#ifndef PRJQUOTA
#define PRJQUOTA	2
#endif

#ifndef XFS_PROJ_QUOTA
#define XFS_PROJ_QUOTA	2
#endif

#ifndef Q_XSETPQLIM
#define Q_XSETPQLIM QCMD(Q_XSETQLIM, PRJQUOTA)
#endif

#ifndef Q_XGETPQUOTA
#define Q_XGETPQUOTA QCMD(Q_XGETQUOTA, PRJQUOTA)
#endif

const int Q_XGETQSTAT_PRJQUOTA = QCMD(Q_XGETQSTAT, PRJQUOTA);
*/
import "C"
import (
	"fmt"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
	"syscall"
	"unsafe"
)

const (
	// Documented in man xfs_quota(8); not necessarily the same
	// as the filesystem blocksize
	BASIC_BLOCK_SIZE = 512
)

// supported check if the given path supports project quotas
func supported(mountPoint string, devicePath string, magic int64) bool {
	var buf syscall.Statfs_t
	err := syscall.Statfs(mountPoint, &buf)
	if err != nil {
		klog.V(3).Infof("Unable to statfs %s: %v", mountPoint, err)
		return false
	}
	if buf.Type != magic {
		return false
	}

	var qstat C.fs_quota_stat_t

	var cs = C.CString(devicePath)
	defer free(cs)

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, uintptr(C.Q_XGETQSTAT_PRJQUOTA), uintptr(unsafe.Pointer(cs)), 0, uintptr(unsafe.Pointer(&qstat)), 0, 0)
	return errno == 0 && qstat.qs_flags&C.FS_QUOTA_PDQ_ENFD > 0 && qstat.qs_flags&C.FS_QUOTA_PDQ_ACCT > 0
}

// GetProjectQuota retrieves the quotaProvider settings associated
// with a given directory
func getProjectQuota(devicePath string, projectId uint32) (quota *Quota, err error) {
	var (
		disk_quota C.fs_disk_quota_t
		cs         = C.CString(devicePath)
	)
	defer C.free(unsafe.Pointer(cs))

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, C.Q_XGETPQUOTA,
		uintptr(unsafe.Pointer(cs)), uintptr(C.__u32(projectId)),
		uintptr(unsafe.Pointer(&disk_quota)), 0, 0)
	if errno != 0 {
		return nil, fmt.Errorf("failed get project quotaProvider - prjId=%d dev=%s quotaPath=%s; error code: %d ", projectId, devicePath, errno)
	}
	return &Quota{
		HardQuota: uint64(disk_quota.d_blk_hardlimit * BASIC_BLOCK_SIZE),
		SoftQuota: uint64(disk_quota.d_blk_softlimit * BASIC_BLOCK_SIZE),
		Used:      uint64(disk_quota.d_bcount * BASIC_BLOCK_SIZE),
	}, nil
}

// SetProjectQuota sets quotaProvider settings associated with a given
// projectId controlled by a given block device.
//
// The values that are not prefixed with `Usage` of the supplied  `Quota`
// pointer are taken into account.
//
// 0 values are meant to indicate that there's no quotaProvider (i.e, no
// limits).
func setProjectQuota(devicePath string, projectId uint32, hardQuota, softQuota uint64) (err error) {

	var d C.fs_disk_quota_t
	d.d_version = C.FS_DQUOT_VERSION
	d.d_id = C.__u32(projectId)
	d.d_flags = C.XFS_PROJ_QUOTA

	d.d_fieldmask = C.FS_DQ_BHARD | C.FS_DQ_BSOFT
	d.d_blk_hardlimit = C.__u64(hardQuota / BASIC_BLOCK_SIZE)
	d.d_blk_softlimit = C.__u64(softQuota / BASIC_BLOCK_SIZE)

	var cs = C.CString(devicePath)
	defer C.free(unsafe.Pointer(cs))

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, C.Q_XSETPQLIM,
		uintptr(unsafe.Pointer(cs)), uintptr(d.d_id),
		uintptr(unsafe.Pointer(&d)), 0, 0)

	if errno != 0 {
		return fmt.Errorf("failed set project quotaProvider - prjId=%d dev=%s quotaPath=%s; error code: %d ", projectId, devicePath, errno)
	}
	return nil

}

// getProjectId retrieves the extended attribute projectid associated
// with a given directory.
func getProjectId(quotaPath string) (uint32, error) {
	dir, err := openDir(quotaPath)
	if err != nil {
		klog.V(3).Infof("Can't open directory %s: %#+v", quotaPath, err)
		return 0, err
	}
	defer closeDir(dir)

	var fsx C.struct_fsxattr
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, getDirFd(dir), C.FS_IOC_FSGETXATTR,
		uintptr(unsafe.Pointer(&fsx)))

	if errno != 0 {
		return 0, fmt.Errorf("failed to get quotaProvider ID for %s: %s", quotaPath, errno.Error())
	}
	return uint32(fsx.fsx_projid), nil
}

func setProjectId(quotaPath string, projectId uint32) error {
	dir, err := openDir(quotaPath)
	if err != nil {
		klog.V(3).Infof("Can't open directory %s: %#+v", quotaPath, err)
		return err
	}
	defer closeDir(dir)

	var fsx C.struct_fsxattr

	fsx.fsx_projid = C.__u32(projectId)
	fsx.fsx_xflags |= C.FS_XFLAG_PROJINHERIT
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, getDirFd(dir), C.FS_IOC_FSSETXATTR,
		uintptr(unsafe.Pointer(&fsx)))
	if errno != 0 {
		return fmt.Errorf("Failed to set quotaProvider ID for %s: %v", quotaPath, errno.Error())
	}
	return nil
}

func free(p *C.char) {
	C.free(unsafe.Pointer(p))
}

func openDir(path string) (*C.DIR, error) {
	Cpath := C.CString(path)
	defer free(Cpath)

	dir := C.opendir(Cpath)
	if dir == nil {
		return nil, fmt.Errorf("Can't open dir")
	}
	return dir, nil
}

func closeDir(dir *C.DIR) {
	if dir != nil {
		C.closedir(dir)
	}
}

func getDirFd(dir *C.DIR) uintptr {
	return uintptr(C.dirfd(dir))
}
