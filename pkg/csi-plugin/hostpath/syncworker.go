package hostpath

import (
	quotaMng "ShixuanMiao/hostpathpv-plugin/pkg/csi-plugin/hostpath/quota-manager"
	hpClient "ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"ShixuanMiao/hostpathpv-plugin/pkg/util"
	"sort"
	"time"
)

type SyncWorker struct {
	hpClient               hpClient.HostPathClientInterface
	mStore                 MetaStoreInterFace
	quotaMng               quotaMng.QuotaManagerInterface
	shouldDeleteVolIds     []quotaMng.VolumeId
	nodeName               string
	lastSyncQuotaInfoCache map[quotaMng.VolumeId]quotaMng.PathQuotaInfo
	lastNodeUsageInfo      hpClient.NodeUsageInfo
}

// resetOrReuseTimer avoids allocating a new timer if one is already in use.
// Not safe for multiple threads.
func resetOrReuseTimer(t *time.Timer, d time.Duration, sawTimeout bool) *time.Timer {
	if t == nil {
		return time.NewTimer(d)
	}
	if !t.Stop() && !sawTimeout {
		<-t.C
	}
	t.Reset(d)
	return t
}

// JitterUntil loops until stop channel is closed, running f every period.
//
// If jitterFactor is positive, the period is jittered before every run of f.
// If jitterFactor is not positive, the period is unchanged and not jittered.
//
// If sliding is true, the period is computed after f runs. If it is false then
// period includes the runtime for f.
//
// Close stopCh to stop. f may not be invoked if stop channel is already
// closed. Pass NeverStop to if you don't want it stop.
func JitterUntil(f func(), period time.Duration, stopCh <-chan struct{}) {
	var t *time.Timer
	var sawTimeout bool

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		jitteredPeriod := period
		f()
		t = resetOrReuseTimer(t, jitteredPeriod, sawTimeout)

		// NOTE: b/c there is no priority selection in golang
		// it is possible for this to race, meaning we could
		// trigger t.C and stopCh, and t.C select falls through.
		// In order to mitigate we re-check stopCh at the beginning
		// of every loop to prevent extra executions of f().
		select {
		case <-stopCh:
			return
		case <-t.C:
			sawTimeout = true
		}
	}
}

func (r *SyncWorker) Start(period time.Duration) {
	stopCh := make(chan struct{})
	go func() {
		JitterUntil(r.populatorLoopFunc(), period, stopCh)
	}()
}

func (r *SyncWorker) populatorLoopFunc() func() {
	return func() {
		util.DebugLogMsg("populatorLoopFunc start")
		quotaInfo, err := r.quotaMng.ListPathQuotaInfos()
		if err != nil {
			util.ErrorLogMsg("ListPathQuotaInfos err:%v", err)
		}
		util.DebugLogMsg("populatorLoopFunc start cleanOrphanQuotaPath")
		if err := r.cleanOrphanQuotaPath(quotaInfo); err != nil {
			util.ErrorLogMsg("cleanOrphanQuotaPath err:%v", err)
		}
		util.DebugLogMsg("populatorLoopFunc finish cleanOrphanQuotaPath")

		util.DebugLogMsg("syncQuotaPathUsage start syncQuotaPathUsage")
		if err := r.syncQuotaPathUsage(quotaInfo); err != nil {
			util.ErrorLogMsg("syncQuotaPathUsage err:%v", err)
		}
		util.DebugLogMsg("syncQuotaPathUsage finish syncQuotaPathUsage")

		diskInfo, err := r.quotaMng.ListQuotaDiskInfos()
		if err != nil {
			util.ErrorLogMsg("ListQuotaDiskInfos err:%v", err)
		}
		util.DebugLogMsg("syncNodeQuotaStatus start syncNodeQuotaStatus")
		if err := r.syncNodeQuotaStatus(diskInfo); err != nil {
			util.ErrorLogMsg("syncNodeQuotaStatus err:%v", err)
		}
		util.DebugLogMsg("syncNodeQuotaStatus finish syncNodeQuotaStatus")
	}
}

func (r *SyncWorker) cleanOrphanQuotaPath(quotaInfos []quotaMng.PathQuotaInfo) error {
	nextLoopShouldDeleteVolId := make([]quotaMng.VolumeId, 0, len(r.shouldDeleteVolIds))
	for _, volId := range r.shouldDeleteVolIds {
		util.DebugLogMsg("try to re clean quotaPath: %s", volId)
		if err := r.quotaMng.ReleaseVolumePath(volId); err != nil {
			nextLoopShouldDeleteVolId = append(nextLoopShouldDeleteVolId, volId)
		}
		util.DefaultLog("success clean quotaPath: %s", volId)
	}

	// clear not need sync disk current usage info
	metaInfos, err := r.mStore.ListMetaInfos()
	if err != nil {
		util.ErrorLogMsg("failed get metaStore metaInfos , %v", err)
		return err
	}
	for _, quotaInfo := range quotaInfos {
		if _, ok := metaInfos[string(quotaInfo.VolumeId)]; ok {
			continue
		}
		util.DefaultLog("volume %s has been deleted,try to clean quotaPath: %s", quotaInfo.VolumeId, quotaInfo.Path)
		if err := r.quotaMng.ReleaseVolumePath(quotaInfo.VolumeId); err != nil {
			nextLoopShouldDeleteVolId = append(nextLoopShouldDeleteVolId, quotaInfo.VolumeId)
			util.ErrorLogMsg("failed clean quotaPath: %s of volume %s err: %v", quotaInfo.Path, quotaInfo.VolumeId, err)
			continue
		}
		util.DefaultLog("success clean quotaPath: %s of volume %s ", quotaInfo.Path, quotaInfo.VolumeId)
	}
	r.shouldDeleteVolIds = nextLoopShouldDeleteVolId
	return nil
}

func (r *SyncWorker) syncQuotaPathUsage(quotaInfos []quotaMng.PathQuotaInfo) error {
	// sync all hostPath usage
	for _, info := range quotaInfos {
		if r.shouldUpdate(info) {
			hpv := r.hpClient.GetHostPathPVByVolumeId(string(info.VolumeId))
			if hpv == nil {
				continue
			}
			hpv.SetBindNodeName(r.nodeName)
			hpv.SetVolumePath(info.Path)
			hpv.SetUsage(info.Quota.Used)

			if err := r.hpClient.UpdateHostPathPVUsageInfo(hpv.GetName(), hpv); err != nil {
				return err
			}
			r.lastSyncQuotaInfoCache[info.VolumeId] = info
			util.DefaultLog("success update quota info of quotaPath: %s ", hpv.GetName())
		}
	}

	return nil
}

func (r *SyncWorker) syncNodeQuotaStatus(diskInfo []quotaMng.DiskQuotaInfo) error {
	node, err := r.hpClient.GetNodeByName(r.nodeName)
	if err != nil {
		return err
	}
	disableDisks := node.GetDisabledDisk()
	// first we sync disk enable/disable state to node
	for _, disk := range diskInfo {
		if _, ok := disableDisks[disk.MountPoint]; ok {
			if disk.Disabled == false {
				err := r.quotaMng.SetQuotaDiskDisabledState(disk.MountPoint, true)
				if err != nil {
					util.ErrorLogMsg("set disk %s disable fail:%v", disk.MountPoint, err)
				}
				util.DefaultLog("success set disk: %s,mountPoint: %s disable state to false", disk.DeviceName, disk.MountPoint)
			}
		} else {
			if disk.Disabled == true {
				err := r.quotaMng.SetQuotaDiskDisabledState(disk.MountPoint, false)
				if err != nil {
					util.ErrorLogMsg("set disk %s enable fail:%v", disk.MountPoint, err)
				}
				util.DefaultLog("success set disk: %s,mountPoint: %s disable state to true", disk.DeviceName, disk.MountPoint)
			}
		}
	}

	// sync disk info
	nodeDiskInfo := hpClient.NodeUsageInfo{
		Capacity:      0,
		QuotaSize:     0,
		Used:          0,
		AvailableSize: 0,
		Allocatable:   0,
		DiskStatus:    make([]hpClient.DiskInfo, 0, len(diskInfo)),
	}

	for _, disk := range diskInfo {
		nodeDiskInfo.Capacity += int64(disk.Capacity)
		nodeDiskInfo.QuotaSize += int64(disk.QuotaSize)
		nodeDiskInfo.Used += int64(disk.Used)
		nodeDiskInfo.Allocatable += int64(disk.AllocatableSize)
		nodeDiskInfo.DiskStatus = append(nodeDiskInfo.DiskStatus, hpClient.DiskInfo{
			MountPoint:  disk.MountPoint,
			Capacity:    int64(disk.Capacity),
			QuotaSize:   int64(disk.QuotaSize),
			Used:        int64(disk.Used),
			Allocatable: int64(disk.AllocatableSize),
			Disabled:    disk.Disabled,
		})
	}
	nodeDiskInfo.AvailableSize = nodeDiskInfo.Capacity - nodeDiskInfo.Used
	sort.Sort(nodeDiskInfo.DiskStatus)

	if r.shouldNodeUpdate(nodeDiskInfo) {
		if err := r.hpClient.UpdateNodeHostPathPVUsageInfo(r.nodeName, nodeDiskInfo); err != nil {
			return err
		}
		r.lastNodeUsageInfo = nodeDiskInfo
		util.DefaultLog("success update disk info of node: %s", r.nodeName)
	}

	return nil
}

func (r *SyncWorker) shouldUpdate(new quotaMng.PathQuotaInfo) bool {
	if old, ok := r.lastSyncQuotaInfoCache[new.VolumeId]; ok {
		return new.Path != old.Path || new.Quota.HardQuota != old.Quota.HardQuota || new.Quota.Used != old.Quota.Used
	}
	return true
}

func (r *SyncWorker) shouldNodeUpdate(new hpClient.NodeUsageInfo) bool {
	old := r.lastNodeUsageInfo
	if old.Capacity != new.Capacity || old.QuotaSize != new.QuotaSize || util.RoundOffBytes(old.Used) != util.RoundOffBytes(new.Used) {
		return true
	}

	for index, disk := range old.DiskStatus {
		if disk.MountPoint != new.DiskStatus[index].MountPoint || util.RoundOffBytes(disk.Used) != util.RoundOffBytes(new.DiskStatus[index].Used) || disk.Allocatable != disk.Allocatable {
			return true
		}
	}
	return false
}
