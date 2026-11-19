//go:build windows

package metadata

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileExclusiveLock                 = 0x00000002
	lockfileFailImmediately               = 0x00000001
	errorLockViolation      syscall.Errno = 33
)

var (
	kernel32DLL      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32DLL.NewProc("LockFileEx")
	procUnlockFileEx = kernel32DLL.NewProc("UnlockFileEx")
)

func tryAdvisoryLock(path string) (func(), bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, false, err
	}
	overlapped := new(syscall.Overlapped)
	result, _, callErr := procLockFileEx.Call(
		file.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	if result == 0 {
		_ = file.Close()
		if callErr == errorLockViolation {
			return nil, false, nil
		}
		return nil, false, callErr
	}
	return func() {
		_, _, _ = procUnlockFileEx.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(overlapped)))
		_ = file.Close()
	}, true, nil
}
