//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package metadata

import (
	"errors"
	"os"
	"syscall"
)

func pathComponentIsReparsePoint(string) bool { return false }

// Unix can enforce both ownership and mode. Existing directories are rejected
// rather than repaired; a newly created directory is chmodded through a
// no-follow directory descriptor so chmod never traverses a symlink.
func validateCacheDirectorySecurity(path string, created bool) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return errors.New("cache directory open failure")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		return errors.New("unsafe cache directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("cache directory ownership mismatch")
	}
	if created {
		if err := file.Chmod(0o700); err != nil {
			return err
		}
		info, err = file.Stat()
		if err != nil {
			return err
		}
	}
	if info.Mode().Perm()&0o022 != 0 {
		return errors.New("cache directory is group/world writable")
	}
	return nil
}
