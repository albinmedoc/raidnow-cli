package metadata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestCachePathUsesAbsoluteXDGAndOverride(t *testing.T) {
	xdg := t.TempDir()
	override := t.TempDir()

	got, err := CachePath(Options{CacheDir: override})
	if err != nil {
		t.Fatalf("CachePath override: %v", err)
	}
	if want := filepath.Join(override, "ary-pokemon.json"); got != want {
		t.Fatalf("CachePath override = %q, want %q", got, want)
	}

	t.Setenv("XDG_CACHE_HOME", xdg)
	got, err = CachePath(Options{})
	if err != nil {
		t.Fatalf("CachePath XDG: %v", err)
	}
	if want := filepath.Join(xdg, "raidnow", "ary-pokemon.json"); got != want {
		t.Fatalf("CachePath XDG = %q, want %q", got, want)
	}

	t.Setenv("XDG_CACHE_HOME", "")
	userCache, userErr := os.UserCacheDir()
	if userErr != nil {
		t.Fatalf("UserCacheDir: %v", userErr)
	}
	got, err = CachePath(Options{})
	if err != nil {
		t.Fatalf("CachePath default: %v", err)
	}
	if want := filepath.Join(userCache, "raidnow", cacheFilename); got != want {
		t.Fatalf("CachePath default = %q, want %q", got, want)
	}

	t.Setenv("XDG_CACHE_HOME", "relative-cache")
	got, err = CachePath(Options{})
	if err != nil {
		t.Fatalf("CachePath relative XDG fallback: %v", err)
	}
	if want := filepath.Join(userCache, "raidnow", cacheFilename); got != want {
		t.Fatalf("CachePath relative XDG = %q, want fallback %q", got, want)
	}
}

func TestResolvePersistsRefreshThenFreshHitSkipsFetcher(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	catalog := testCatalog(t, `{"25":{"en":"Pikachu","image":"https://example.invalid/25.png"}}`)
	calls := 0
	fetch := func(context.Context) (Catalog, error) {
		calls++
		return catalog, nil
	}
	options := Options{CacheDir: t.TempDir(), Now: func() time.Time { return now }}

	first, err := Resolve(context.Background(), fetch, options)
	if err != nil {
		t.Fatalf("Resolve refresh: %v", err)
	}
	if first.Catalog.Len() != 1 || first.Status.State != StateRefreshed || calls != 1 {
		t.Fatalf("first result = %#v calls=%d", first, calls)
	}

	fetch = func(context.Context) (Catalog, error) {
		t.Fatal("fresh cache invoked fetcher")
		return Catalog{}, nil
	}
	second, err := Resolve(context.Background(), fetch, options)
	if err != nil {
		t.Fatalf("Resolve fresh: %v", err)
	}
	if second.Catalog.Len() != 1 || second.Status.State != StateFresh {
		t.Fatalf("second result = %#v", second)
	}
}

func testCatalog(t *testing.T, object string) Catalog {
	t.Helper()
	catalog, err := Parse([]byte(`<script>var aryPokemon=` + object + `;</script>`))
	if err != nil {
		t.Fatalf("Parse test catalog: %v", err)
	}
	return catalog
}

func TestResolveRefreshPoliciesAndCutoff(t *testing.T) {
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	oldCatalog := testCatalog(t, `{"old":{"en":"Old"}}`)
	newCatalog := testCatalog(t, `{"new":{"en":"New"}}`)

	t.Run("stale refresh", func(t *testing.T) {
		dir := t.TempDir()
		now := base
		if _, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return oldCatalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }}); err != nil {
			t.Fatal(err)
		}
		now = base.Add(DefaultFreshTTL + time.Second)
		result, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return newCatalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
		if err != nil || result.Status.State != StateRefreshed {
			t.Fatalf("stale refresh = %#v, %v", result, err)
		}
		if _, ok := result.Catalog.Lookup("new"); !ok {
			t.Fatal("refresh did not return new catalog")
		}
	})

	t.Run("forced refresh", func(t *testing.T) {
		dir := t.TempDir()
		now := base
		_, _ = Resolve(context.Background(), func(context.Context) (Catalog, error) { return oldCatalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
		result, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return newCatalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }, Force: true})
		if err != nil || result.Status.State != StateRefreshed {
			t.Fatalf("forced refresh = %#v, %v", result, err)
		}
	})

	for _, tc := range []struct {
		name      string
		age       time.Duration
		freshOnly bool
		wantOld   bool
		wantExit  int
	}{
		{name: "stale if error", age: 2 * 24 * time.Hour, wantOld: true},
		{name: "exact cutoff", age: DefaultStaleIfError, wantOld: true},
		{name: "just over cutoff", age: DefaultStaleIfError + time.Nanosecond, wantExit: 11},
		{name: "fresh only", age: 2 * 24 * time.Hour, freshOnly: true, wantExit: 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			now := base
			_, _ = Resolve(context.Background(), func(context.Context) (Catalog, error) { return oldCatalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
			now = base.Add(tc.age)
			result, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return Catalog{}, errors.New("SECRET upstream failure") }, Options{CacheDir: dir, Now: func() time.Time { return now }, FreshOnly: tc.freshOnly})
			if tc.wantOld {
				if err != nil || result.Status.State != StateDegraded {
					t.Fatalf("fallback = %#v, %v", result, err)
				}
				if _, ok := result.Catalog.Lookup("old"); !ok {
					t.Fatal("fallback did not retain old catalog")
				}
				return
			}
			if got := output.ExitCodeFor(err); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d; err=%v", got, tc.wantExit, err)
			}
		})
	}
}

func TestCacheStrictDecodeAndRecovery(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	catalog := testCatalog(t, `{"150":{"en":"Mewtwo","image":"url"}}`)
	valid, _, err := encodeCache(catalog, now)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func([]byte) []byte{
		"empty":    func([]byte) []byte { return nil },
		"trailing": func(b []byte) []byte { return append(append([]byte{}, b...), []byte(` {}`)...) },
		"bad digest": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"contentDigest":"`, `"contentDigest":"0`, 1))
		},
		"bad schema": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"schemaVersion":1`, `"schemaVersion":2`, 1))
		},
		"bad source": func(b []byte) []byte { return []byte(strings.Replace(string(b), cacheSource, "other", 1)) },
		"bad time": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), now.Format(time.RFC3339), "not-time", 1))
		},
		"duplicate": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `{"schemaVersion":1`, `{"schemaVersion":1,"schemaVersion":1`, 1))
		},
		"entry unknown": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"imageUrl":"url"`, `"imageUrl":"url","extra":1`, 1))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCache(mutate(valid)); !errors.Is(err, ErrInvalidMetadata) {
				t.Fatalf("decode error = %v, want ErrInvalidMetadata", err)
			}
		})
	}
	if _, err := decodeCache(append(valid, make([]byte, MaxCacheBytes)...)); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("oversize error = %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, cacheFilename)
	if err := os.WriteFile(path, []byte(`corrupt SECRET-ID SECRET-NAME https://secret.invalid`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
	if err != nil || result.Status.State != StateRefreshed {
		t.Fatalf("corrupt recovery = %#v, %v", result, err)
	}
	if _, state := loadCache(path); state != loadValid {
		t.Fatalf("replaced cache state = %v, want valid", state)
	}
}

func TestSuspiciousEmptyRefreshDoesNotReplaceEligibleCache(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, `{"id":{"en":"Name"}}`)
	_, _ = Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
	path := filepath.Join(dir, cacheFilename)
	before, _ := os.ReadFile(path)
	now = now.Add(48 * time.Hour)
	result, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return Catalog{}, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
	if err != nil || result.Status.State != StateDegraded {
		t.Fatalf("empty refresh fallback = %#v, %v", result, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("empty refresh changed last good cache")
	}
}

func TestDeterministicEncodingPreservesExactForms(t *testing.T) {
	one := Catalog{entries: map[string]Entry{
		"150":         {id: "150", name: "Mewtwo"},
		"150_armored": {id: "150_armored", name: "Armored Mewtwo", imageURL: "armored"},
	}}
	two := Catalog{entries: map[string]Entry{
		"150_armored": {id: "150_armored", name: "Armored Mewtwo", imageURL: "armored"},
		"150":         {id: "150", name: "Mewtwo"},
	}}
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	bytesOne, digestOne, err := encodeCache(one, now)
	if err != nil {
		t.Fatal(err)
	}
	bytesTwo, digestTwo, _ := encodeCache(two, now)
	if string(bytesOne) != string(bytesTwo) || digestOne != digestTwo {
		t.Fatal("canonical output depends on map order")
	}
	loaded, err := decodeCache(bytesOne)
	if err != nil || loaded.catalog.Len() != 2 {
		t.Fatalf("decode = %#v, %v", loaded, err)
	}
	if entry, ok := loaded.catalog.Lookup("150_armored"); !ok || entry.Name() != "Armored Mewtwo" {
		t.Fatal("exact form ID was merged or lost")
	}
}

func TestAtomicFailuresKeepOldBytesAndCleanTemps(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	oldCatalog := testCatalog(t, `{"old":{"en":"Old"}}`)
	newCatalog := testCatalog(t, `{"new":{"en":"New"}}`)
	_, _ = Resolve(context.Background(), func(context.Context) (Catalog, error) { return oldCatalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
	path := filepath.Join(dir, cacheFilename)
	before, _ := os.ReadFile(path)

	for _, tc := range []struct {
		name string
		edit func(*Options)
	}{
		{name: "write", edit: func(o *Options) { o.writeFile = func(*os.File, []byte) error { return errors.New("SECRET write") } }},
		{name: "rename", edit: func(o *Options) { o.rename = func(string, string) error { return errors.New("SECRET rename") } }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := Options{CacheDir: dir, Now: func() time.Time { return now }, Force: true}
			tc.edit(&options)
			_, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return newCatalog, nil }, options)
			if got := output.ExitCodeFor(err); got != 10 {
				t.Fatalf("exit = %d, want 10; err=%v", got, err)
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(before) {
				t.Fatal("failed atomic replacement changed old bytes")
			}
			matches, _ := filepath.Glob(filepath.Join(dir, ".ary-pokemon-*.tmp"))
			if len(matches) != 0 {
				t.Fatalf("temporary files remain: %v", matches)
			}
		})
	}
}

func TestUnsafeFilesAndLockContentionMapToLocalPersistence(t *testing.T) {
	catalog := testCatalog(t, `{"id":{"en":"Name"}}`)
	for _, tc := range []struct {
		name string
		make func(string) error
	}{
		{name: "symlink", make: func(path string) error { return os.Symlink(filepath.Join(filepath.Dir(path), "target"), path) }},
		{name: "nonregular", make: func(path string) error { return os.Mkdir(path, 0o700) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, cacheFilename)
			if err := tc.make(path); err != nil {
				t.Skipf("not portable: %v", err)
			}
			_, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalog, nil }, Options{CacheDir: dir})
			if got := output.ExitCodeFor(err); got != 10 {
				t.Fatalf("exit = %d, want 10; err=%v", got, err)
			}
		})
	}

	dir := t.TempDir()
	lockPath := filepath.Join(dir, cacheFilename+".lock")
	unlock, lockErr := acquireLock(lockPath, withDefaults(Options{LockTimeout: time.Second}))
	if lockErr != nil {
		t.Fatalf("acquire first lock: %v", lockErr)
	}
	defer unlock()
	_, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalog, nil }, Options{CacheDir: dir, LockTimeout: 20 * time.Millisecond})
	if got := output.ExitCodeFor(err); got != 10 {
		t.Fatalf("lock contention exit = %d, want 10; err=%v", got, err)
	}
}

func TestProcessLockContention(t *testing.T) {
	if os.Getenv("RAIDNOW_CACHE_LOCK_HELPER") == "1" {
		path := os.Getenv("RAIDNOW_CACHE_LOCK_PATH")
		unlock, err := acquireLock(path, withDefaults(Options{LockTimeout: time.Second}))
		if err != nil {
			os.Exit(2)
		}
		defer unlock()
		if err := os.WriteFile(os.Getenv("RAIDNOW_CACHE_LOCK_READY"), []byte("ready"), 0o600); err != nil {
			os.Exit(3)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("RAIDNOW_CACHE_LOCK_RELEASE")); err == nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		os.Exit(4)
	}

	dir := t.TempDir()
	lockPath := filepath.Join(dir, cacheFilename+".lock")
	ready := filepath.Join(dir, "ready")
	release := filepath.Join(dir, "release")
	command := exec.Command(os.Args[0], "-test.run=^TestProcessLockContention$", "-test.count=1")
	command.Env = append(os.Environ(),
		"RAIDNOW_CACHE_LOCK_HELPER=1",
		"RAIDNOW_CACHE_LOCK_PATH="+lockPath,
		"RAIDNOW_CACHE_LOCK_READY="+ready,
		"RAIDNOW_CACHE_LOCK_RELEASE="+release,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			t.Fatal("helper did not acquire process lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	catalog := testCatalog(t, `{"id":{"en":"Name"}}`)
	_, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalog, nil }, Options{CacheDir: dir, LockTimeout: 20 * time.Millisecond})
	if got := output.ExitCodeFor(err); got != 10 {
		t.Fatalf("process contention exit = %d, want 10; err=%v", got, err)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("lock helper: %v", err)
	}
}

func TestConcurrentReadersAndWritersSeeOnlyValidCatalogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	catalogs := []Catalog{
		testCatalog(t, `{"a":{"en":"A"}}`),
		testCatalog(t, `{"b":{"en":"B"}}`),
	}
	_, _ = Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalogs[0], nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalogs[i%2], nil }, Options{CacheDir: dir, Now: func() time.Time { return now }, Force: true})
			errs <- err
		}(i)
		go func() {
			defer wg.Done()
			loaded, state := loadCache(filepath.Join(dir, cacheFilename))
			if state != loadValid || loaded.catalog.Len() != 1 {
				errs <- fmt.Errorf("invalid concurrent read state=%v len=%d", state, loaded.catalog.Len())
				return
			}
			errs <- nil
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPermissionsExactIDFallbackAndSafeFormatting(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	const secretID = "SECRET-ID_form"
	const secretName = "SECRET-NAME"
	const secretURL = "https://secret.invalid/SECRET-URL"
	catalog := testCatalog(t, `{"SECRET-ID_form":{"en":"SECRET-NAME","image":"https://secret.invalid/SECRET-URL"},"blank":{}}`)
	result, err := Resolve(context.Background(), func(context.Context) (Catalog, error) { return catalog, nil }, Options{CacheDir: dir, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, %v", info.Mode().Perm(), err)
	}
	path := filepath.Join(dir, cacheFilename)
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, %v", info.Mode().Perm(), err)
	}
	if name, image := DisplayForID(catalog, secretID); name != secretName || image != secretURL {
		t.Fatalf("exact display = %q, %q", name, image)
	}
	if name, image := DisplayForID(catalog, "SECRET-ID"); name != "SECRET-ID" || image != "" {
		t.Fatalf("missing exact fallback = %q, %q", name, image)
	}
	if name, image := DisplayForID(catalog, "blank"); name != "blank" || image != "" {
		t.Fatalf("blank fallback = %q, %q", name, image)
	}
	for _, formatted := range []string{fmt.Sprint(result), fmt.Sprintf("%#v", result), fmt.Sprint(result.Status), fmt.Sprintf("%#v", result.Status)} {
		for _, secret := range []string{secretID, secretName, secretURL} {
			if strings.Contains(formatted, secret) {
				t.Fatalf("safe formatting %q leaked %q", formatted, secret)
			}
		}
	}
	missingDir := t.TempDir()
	_, err = Resolve(context.Background(), nil, Options{CacheDir: missingDir})
	if output.ExitCodeFor(err) != 11 || strings.Contains(fmt.Sprintf("%#v", err), "SECRET") {
		t.Fatalf("unavailable mapping/format = %d, %#v", output.ExitCodeFor(err), err)
	}
}
