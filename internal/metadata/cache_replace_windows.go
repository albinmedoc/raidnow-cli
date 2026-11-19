//go:build windows

package metadata

import (
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
