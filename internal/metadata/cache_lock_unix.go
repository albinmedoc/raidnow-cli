//go:build !windows

package metadata

import (
	"errors"
	"os"
	"syscall"
)

func tryAdvisoryLock(path string) (func(), bool, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, false, errors.New("lock file failure")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, false, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = file.Close()
	}, true, nil
}
