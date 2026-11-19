//go:build !windows

package metadata

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestResolveRejectsInsecureExistingCacheDirectoryWithoutRepairingMode(t *testing.T) {
	dir := cacheTestDir(t)
	catalog := testCatalog(t, `{"id":{"en":"Name"}}`)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data, _, err := encodeCache(catalog, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}

	_, err = Resolve(context.Background(), nil, Options{CacheDir: dir, Now: func() time.Time { return now }})
	if got := output.ExitCodeFor(err); got != 10 {
		t.Fatalf("exit = %d, want 10; err=%v", got, err)
	}
	info, statErr := os.Stat(dir)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if got := info.Mode().Perm(); got != 0o777 {
		t.Fatalf("existing directory mode was repaired to %o", got)
	}
}
