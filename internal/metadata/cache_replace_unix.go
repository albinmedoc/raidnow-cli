//go:build !windows

package metadata

import "os"

func replaceCacheFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
