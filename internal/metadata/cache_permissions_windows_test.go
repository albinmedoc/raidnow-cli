//go:build windows

package metadata

import (
	"os"
	"testing"
)

func assertCachePermissions(t *testing.T, dir, path string) {
	t.Helper()
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("stat cache directory: %v", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || pathComponentIsReparsePoint(dir) {
		t.Fatalf("cache directory is not a real directory: mode=%v", dirInfo.Mode())
	}
	fileInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 || pathComponentIsReparsePoint(path) {
		t.Fatalf("cache file is not a real regular file: mode=%v", fileInfo.Mode())
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("cache file is read-only: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close writable cache file: %v", err)
	}
}
