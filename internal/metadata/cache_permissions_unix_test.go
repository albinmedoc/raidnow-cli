//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package metadata

import (
	"os"
	"testing"
)

func assertCachePermissions(t *testing.T, dir, path string) {
	t.Helper()
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat cache directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode = %o, want 700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %o, want 600", got)
	}
}
