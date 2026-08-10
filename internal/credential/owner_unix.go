//go:build unix

package credential

import (
	"os"
	"syscall"
)

type fileOwner struct {
	uid int
	gid int
}

// ownerOf extracts the file's owning uid and gid on Unix-like platforms.
func ownerOf(info os.FileInfo) (fileOwner, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileOwner{}, false
	}
	return fileOwner{uid: int(stat.Uid), gid: int(stat.Gid)}, true
}

// currentEUID returns the effective uid of the process.
func currentEUID() int {
	return os.Geteuid()
}

func filePolicySupported() bool {
	return true
}
