package quota_manager

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"fmt"
	"path"
	"strconv"
	"strings"
)

const (

	//xfs projectId start
	ProjectIdStart int = 1000000

	//xfs quotaId num
	ProjectIdNum int = 5000

	//xfs quota path name prefix
	XfsQuotaDirPrefix = "k8squota"

	// attr key to set quota disk disable
	DiskQuotaEnableFattr = "user.storage.hostpathpv.xfsquota/disable"
)

func getQuotaPathName(volumeId VolumeId) string {
	return fmt.Sprintf("%s_%s", XfsQuotaDirPrefix, volumeId)
}

func parseQuotaPathInfoFromDirName(filepath string) (volumeId VolumeId, err error) {
	filepath = path.Clean(filepath)
	pos := strings.Index(filepath, XfsQuotaDirPrefix)
	if pos <= 0 {
		util.ErrorLogMsg("path %v is not a valid hostpath quota path", err)
		return "", err
	}
	// XfsQuotaDirPrefix + "_" length is 9
	return VolumeId(filepath[pos+9:]), nil
}

func setQuotaDiskDisableState(mountPoint string, disabled bool) error {
	return util.SetFilesystemXattr(mountPoint, DiskQuotaEnableFattr, strconv.FormatBool(disabled))
}

func getDiskQuotaDisableState(mountPoint string) bool {
	attrString, err := util.GetFilesystemXattr(mountPoint, DiskQuotaEnableFattr)
	if err != nil {
		util.ErrorLogMsg("failed get fattr %s. err %v", DiskQuotaEnableFattr, err)
		//means this fattr has not set ,set default value
		if err.Error() == "no data available" {
			err = setQuotaDiskDisableState(mountPoint, false)
			if err != nil {
				util.ErrorLogMsg("failed set default value of fattr %s. err %v", DiskQuotaEnableFattr, err)
			}
			return false
		}
		return false
	}
	if disable, err := strconv.ParseBool(attrString); err != nil {
		util.ErrorLogMsg("failed parseBool from value %s. err %v", attrString, err)
	} else {
		return disable
	}
	return false
}
