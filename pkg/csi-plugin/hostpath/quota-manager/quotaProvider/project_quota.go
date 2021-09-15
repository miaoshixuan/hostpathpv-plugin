package quotaProvider

type Quota struct {
	HardQuota uint64

	SoftQuota uint64

	Used uint64
}

type LinuxVolumeQuotaProvider interface {
	// check if disk is support quotaProvider
	Supported(mountPoint string, devicePath string) bool

	// get project quotaProvider and usage info of quotaProvider path
	GetProjectQuota(quotaPath string, devicePath string) (*Quota, error)

	// set project quotaProvider info for quotaProvider path
	SetProjectQuota(quotaPath, devicePath string, hardQuota, softQuota uint64) error

	// release project quotaProvider
	DeleteProjectQuota(quotaPath string) (err error)

	// get quotaProvider disk usage info
	GetQuotaDiskInfo(targetPath string) (quota *Quota, err error)
}
