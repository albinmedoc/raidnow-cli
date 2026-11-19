//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package metadata

import (
	"testing"
)

func assertCachePermissions(t *testing.T, _, _ string) {
	t.Helper()
	t.Fatal("cache persistence is unsupported on this platform")
}
