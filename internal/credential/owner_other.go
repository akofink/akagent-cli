//go:build !unix

package credential

import "os"

type fileOwner struct {
	uid int
	gid int
}

// ownerOf cannot resolve ownership on non-Unix platforms, so ownership checks
// are skipped there; permission checks still apply.
func ownerOf(os.FileInfo) (fileOwner, bool) {
	return fileOwner{}, false
}

// currentEUID reports no usable euid on non-Unix platforms.
func currentEUID() int {
	return -1
}

func filePolicySupported() bool {
	return false
}
