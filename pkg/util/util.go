package util

import (
	"bufio"
	"fmt"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"k8s.io/utils/mount"
	"math"
	"os"
	"path"
	"strings"
)

const (
	// MiB - MebiByte size
	MiB               = 1024 * 1024
	GiB               = MiB * 1024
	LINUX_MOUNTS_FILE = "/proc/self/mountinfo"
)

// RoundOffBytes converts roundoff the size
// 1.1Mib will be round off to 2Mib same for GiB
// size less than 1MiB will be round off to 1MiB
func RoundOffBytes(bytes int64) int64 {
	var num int64
	floatBytes := float64(bytes)
	// round off the value if its in decimal
	if floatBytes < GiB {
		num = int64(math.Ceil(floatBytes / MiB))
		num *= MiB
	} else {
		num = int64(math.Ceil(floatBytes / GiB))
		num *= GiB
	}
	return num
}

//GetSubDirs read all sub dirs
func GetSubDirs(parentDir string) []string {
	parentDir = path.Clean(parentDir)
	subDirs := make([]string, 0)
	dir, err := ioutil.ReadDir(parentDir)
	if err != nil {
		return subDirs
	}
	for _, fi := range dir {
		if fi.IsDir() {
			subDirs = append(subDirs, path.Clean(parentDir+string(os.PathSeparator)+fi.Name()))
		}
	}
	return subDirs
}

//SetFilesystemXattr set extended attributes of filesystem objects
func SetFilesystemXattr(path string, attrKey string, attrValue string) error {
	return unix.Setxattr(path, attrKey, []byte(attrValue), 0)
}

//GetFilesystemXattr get extended attributes of filesystem objects
func GetFilesystemXattr(path string, attrKey string) (string, error) {

	// find size
	size, err := unix.Getxattr(path, attrKey, nil)
	if err != nil || size <= 0 {
		return "", err
	}
	xattrDataGet := make([]byte, size)
	_, err = unix.Getxattr(path, attrKey, xattrDataGet)
	if err != nil {
		return "", err
	}
	return string(xattrDataGet), nil
}

// CreateNewDir creates the directory with given path
func CreateNewDir(mountPath string) error {
	return os.MkdirAll(mountPath, 0750)
}

func DeleteFile(path string) error {
	if IsPathExist(path) == true {
		return os.RemoveAll(path)
	}
	return nil
}

// CheckDirExists checks directory  exists or not
func CheckDirExists(p string) bool {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false
	}
	return true
}

// IsMountPoint checks if the given path is mountpoint or not
func IsMountPoint(p string) (bool, error) {
	dummyMount := mount.New("")
	notMnt, err := dummyMount.IsLikelyNotMountPoint(p)
	if err != nil {
		return false, status.Error(codes.Internal, err.Error())
	}

	return !notMnt, nil
}

func IsPathExist(dir string) bool {
	var exist = true
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		exist = false
	}
	return exist
}

func MkDir(dir string) error {
	if IsPathExist(dir) == true {
		return nil
	}
	if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
		return fmt.Errorf("mkdir %s err:%v", dir, mkdirErr)
	}
	return nil
}

// Mount mounts the source to target path
func Mount(source, target, fstype string, options []string) error {
	dummyMount := mount.New("")
	return dummyMount.Mount(source, target, fstype, options)
}

// getDevForPath get device name for a path
func GetDevForPath(mountPoint string) (string, error) {
	mountPoint = path.Clean(mountPoint)
	// Get major/minor
	var stat unix.Stat_t
	err := unix.Lstat(mountPoint, &stat)
	if err != nil {
		return "", err
	}

	devMajor := unix.Major(stat.Dev)
	devMinor := unix.Minor(stat.Dev)

	// Parse mountinfo for it
	mountinfo, err := os.Open(LINUX_MOUNTS_FILE)
	if err != nil {
		return "", err
	}
	defer mountinfo.Close()

	scanner := bufio.NewScanner(mountinfo)
	for scanner.Scan() {
		line := scanner.Text()

		tokens := strings.Fields(line)
		if len(tokens) < 5 {
			continue
		}
		// device and mountPoint must be exactly equal to ensure path is a mountPoint
		if tokens[2] == fmt.Sprintf("%d:%d", devMajor, devMinor) && tokens[4] == mountPoint {
			return tokens[len(tokens)-2], nil
		}
	}

	return "", fmt.Errorf("couldn't find backing device for mountpoint")
}
