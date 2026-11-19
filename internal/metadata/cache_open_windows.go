//go:build windows

package metadata

import (
	"errors"
	"os"
	"syscall"
)

const fileFlagOpenReparsePoint = 0x00200000

func openCacheFileNoFollow(path string) (*os.File, os.FileInfo, error) {
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, nil, err
	}
	handle, err := syscall.CreateFile(
		pointer,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		fileFlagOpenReparsePoint,
		0,
	)
	if err != nil {
		return nil, nil, err
	}
	var handleInfo syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &handleInfo); err != nil {
		_ = syscall.CloseHandle(handle)
		return nil, nil, err
	}
	if handleInfo.FileAttributes&fileAttributeReparsePoint != 0 || handleInfo.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		_ = syscall.CloseHandle(handle)
		return nil, nil, errors.New("cache file is not regular")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = syscall.CloseHandle(handle)
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
