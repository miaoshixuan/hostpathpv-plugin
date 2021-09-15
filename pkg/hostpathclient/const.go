package hostpathclient

const (
	// hsotapth csi driver name
	HostPathCsiDriverName = "xfsquota.hostpath.csi"

	// hostpath node attr
	// node label
	HostPathPvNodeLabelKey = "node-role.kubernetes.io/hostpath"
	// node disk state annotation
	NodeDiskQuotaInfoAnn = "storage.hostpathpv.kubelet/quota-disk-info"
	// node disk disable state annotation
	NodeDiskQuotaDisabledDisksAnn = "storage.hostpathpv.kubelet/disable-disks"

	// hostpath volume attr
	// hostpath pv info annotation
	HostPathPVUsageInfoAnn = "storage.hostpathpv.volume/allocate"
	// hostpath pv bind node annotation
	HostPathPvBindInfoAnn = "storage.hostpathpv.volume/bind-node"
)
