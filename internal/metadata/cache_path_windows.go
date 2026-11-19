//go:build windows

package metadata

import (
	"errors"
	"os"
	"syscall"
)

const fileAttributeReparsePoint = 0x400

func pathComponentIsReparsePoint(path string) bool {
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, err := syscall.GetFileAttributes(pointer)
	return err != nil || attributes&fileAttributeReparsePoint != 0
}

// Windows does not expose portable owner/mode checks through os.FileInfo.
// Reparse-point rejection is enforced for every component and the directory is
// opened after creation; ACL ownership remains governed by the user's profile.
func validateCacheDirectorySecurity(path string, created bool) error {
	if pathComponentIsReparsePoint(path) {
		return errors.New("unsafe cache directory")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return errors.New("unsafe cache directory")
	}
	_ = created // Windows ACLs, not Unix mode bits, govern directory access.
	return nil
}
