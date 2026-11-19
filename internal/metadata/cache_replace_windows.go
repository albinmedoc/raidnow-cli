//go:build windows

package metadata

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	movefileReplaceExisting = 0x1
	movefileWriteThrough    = 0x8
)

var procMoveFileEx = kernel32DLL.NewProc("MoveFileExW")

func replaceCacheFile(oldPath, newPath string) error {
	oldPointer, err := syscall.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPointer, err := syscall.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	result, _, callErr := procMoveFileEx.Call(
		uintptr(unsafe.Pointer(oldPointer)),
		uintptr(unsafe.Pointer(newPointer)),
		uintptr(movefileReplaceExisting|movefileWriteThrough),
	)
	if result == 0 {
		return callErr
	}
	return nil
}

func finalizeTempReplacement(file *os.File, tempName, destination string, rename func(string, string) error) error {
	opened, err := validateTempPathIdentity(file, tempName)
	if err != nil {
		return err
	}
	// Windows replacement can require closing the source handle. Preserve its
	// file identity, close at the last possible point, then revalidate the path
	// immediately before MoveFileEx.
	if err := file.Close(); err != nil {
		return err
	}
	pathInfo, err := os.Lstat(tempName)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || pathComponentIsReparsePoint(tempName) || !pathInfo.Mode().IsRegular() || !os.SameFile(opened, pathInfo) {
		return errors.New("temporary cache identity changed")
	}
	return rename(tempName, destination)
}
