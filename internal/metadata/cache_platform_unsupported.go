//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package metadata

import (
	"errors"
	"os"
)

var errUnsupportedCachePlatform = errors.New("metadata cache is unsupported on this platform")

func openCacheFileNoFollow(string) (*os.File, os.FileInfo, error) {
	return nil, nil, errUnsupportedCachePlatform
}

func tryAdvisoryLock(string) (func(), bool, error) {
	return nil, false, errUnsupportedCachePlatform
}

func replaceCacheFile(string, string) error {
	return errUnsupportedCachePlatform
}

func finalizeTempReplacement(*os.File, string, string, func(string, string) error) error {
	return errUnsupportedCachePlatform
}

func pathComponentIsReparsePoint(string) bool { return true }

func validateCacheDirectorySecurity(string, bool) error {
	return errUnsupportedCachePlatform
}
