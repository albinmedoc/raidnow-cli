//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package metadata

import (
	"errors"
	"os"
	"syscall"
)

func openCacheFileNoFollow(path string) (*os.File, os.FileInfo, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, nil, errors.New("cache file open failure")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New("cache file is not regular")
	}
	return file, info, nil
}
