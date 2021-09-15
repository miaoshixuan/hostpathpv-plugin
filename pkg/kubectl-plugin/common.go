package kubectl_plugin

import "fmt"

func maxint(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func getPercentStr(used, capacity int64) string {
	if capacity <= 0 {
		return "(00.00%)"
	}
	return fmt.Sprintf("(%.2f%%)", (float64(used)/float64(capacity))*100.0)
}

func convertIntToString(size int64) string {
	var ret string
	if size < 1024 {
		ret = fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		ret = fmt.Sprintf("%.2fKB", float64(size)/1024.0)
	} else if size < 1024*1024*1024 {
		ret = fmt.Sprintf("%.2fMB", float64(size)/(1024.0*1024.0))
	} else if size < 1024*1024*1024*1024 {
		ret = fmt.Sprintf("%.2fGB", float64(size)/(1024.0*1024.0*1024.0))
	} else if size < 1024*1024*1024*1024*1024 {
		ret = fmt.Sprintf("%.2fTB", float64(size)/(1024.0*1024.0*1024.0*1024.0))
	} else {
		ret = fmt.Sprintf("%.2fPB", float64(size)/(1024.0*1024.0*1024.0*1024.0*1024.0))
	}
	if len(ret) < 9 {
		ret = getNumSpace(9-len(ret), " ") + ret
	}
	return ret
}
