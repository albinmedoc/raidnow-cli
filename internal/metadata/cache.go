package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

const (
	cacheFilename       = "ary-pokemon.json"
	cacheSchemaVersion  = 1
	cacheSource         = "raidnow-aryPokemon"
	MaxCacheBytes       = 4 << 20
	DefaultFreshTTL     = 24 * time.Hour
	DefaultStaleIfError = 30 * 24 * time.Hour
	defaultLockTimeout  = 2 * time.Second
)

// FetchFunc obtains a validated immutable catalog. Resolve timestamps it with Options.Now.
type FetchFunc func(context.Context) (Catalog, error)

// Options controls cache location, freshness policy, and deterministic time.
type Options struct {
	CacheDir     string
	FreshTTL     time.Duration
	StaleIfError time.Duration
	Now          func() time.Time
	Force        bool
	FreshOnly    bool
	LockTimeout  time.Duration

	writeFile         func(*os.File, []byte) error
	rename            func(string, string) error
	beforeCacheOpen   func(string)
	beforeReplaceTemp func(string)
	beforeLockWait    func()
}

type State string

const (
	StateFresh     State = "fresh"
	StateRefreshed State = "refreshed"
	StateDegraded  State = "degraded"
)

// Status is safe to format: it contains no entry identifiers, names, or URLs.
type Status struct {
	State  State
	Path   string
	Count  int
	Age    time.Duration
	Digest string
}

func (status Status) String() string {
	return fmt.Sprintf("metadata.Status{state:%s entries:%d age:%s digest:%s path:%q}", status.State, status.Count, status.Age, status.Digest, status.Path)
}
func (status Status) GoString() string { return status.String() }

// Result contains metadata and a sanitized cache status.
type Result struct {
	Catalog Catalog
	Status  Status
}

func (result Result) String() string {
	return fmt.Sprintf("metadata.Result{entries:%d status:%s}", result.Catalog.Len(), result.Status)
}
func (result Result) GoString() string { return result.String() }

type ErrorKind string

const (
	ErrorLocalPersistence ErrorKind = "local-persistence"
	ErrorUnavailable      ErrorKind = "metadata-unavailable"
)

// CacheError is sanitized and wraps the public error used for exit-code mapping.
type CacheError struct {
	Kind ErrorKind
	err  error
}

func (err *CacheError) Error() string {
	if err == nil {
		return "metadata cache error"
	}
	if err.Kind == ErrorLocalPersistence {
		return "metadata cache local persistence error"
	}
	return "metadata unavailable"
}
func (err *CacheError) Unwrap() error    { return err.err }
func (err *CacheError) GoString() string { return err.Error() }

func localError() error {
	return &CacheError{Kind: ErrorLocalPersistence, err: output.NewError(output.CodeLocalPersistence, "Lokalt metadata-cachefel.")}
}
func unavailableError() error {
	return &CacheError{Kind: ErrorUnavailable, err: output.NewError(output.CodeMetadataUnavailable, "Pokémon-metadata är inte tillgänglig.")}
}

// CachePath returns the metadata cache file path without creating it.
func CachePath(options Options) (string, error) {
	if options.CacheDir != "" {
		if !filepath.IsAbs(options.CacheDir) {
			return "", localError()
		}
		return filepath.Join(options.CacheDir, cacheFilename), nil
	}
	xdg := os.Getenv("XDG_CACHE_HOME")
	if xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "raidnow", cacheFilename), nil
	}
	dir, err := userCacheDirIgnoringRelativeXDG(xdg)
	if err != nil {
		return "", localError()
	}
	return filepath.Join(dir, "raidnow", cacheFilename), nil
}

func userCacheDirIgnoringRelativeXDG(xdg string) (string, error) {
	if xdg == "" || filepath.IsAbs(xdg) || runtime.GOOS == "windows" || runtime.GOOS == "darwin" || runtime.GOOS == "ios" || runtime.GOOS == "plan9" {
		return os.UserCacheDir()
	}
	home := os.Getenv("HOME")
	if home == "" {
		return "", errors.New("user cache directory unavailable")
	}
	return filepath.Join(home, ".cache"), nil
}

type cacheEntry struct {
	NameEn   string `json:"nameEn"`
	ImageURL string `json:"imageUrl"`
}

type cacheDocument struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Source        string                `json:"source"`
	FetchedAt     string                `json:"fetchedAt"`
	ContentDigest string                `json:"contentDigest"`
	Entries       map[string]cacheEntry `json:"entries"`
}

type loadedCache struct {
	catalog Catalog
	fetched time.Time
	digest  string
}

type loadState int

const (
	loadMissing loadState = iota
	loadValid
	loadCorrupt
	loadLocalFailure
)

// Resolve applies freshness policy, refreshes when required, and atomically persists success.
func Resolve(ctx context.Context, fetch FetchFunc, options Options) (Result, error) {
	options = withDefaults(options)
	path, err := CachePath(options)
	if err != nil {
		return Result{}, err
	}
	exists, err := validateCachePathComponents(filepath.Dir(path))
	if err != nil {
		return Result{}, localError()
	}
	if exists {
		if err := validateCacheDirectorySecurity(filepath.Dir(path), false); err != nil {
			return Result{}, localError()
		}
	}
	now := options.Now().UTC()
	loaded, state := loadCacheWithOptions(path, options)
	if state == loadLocalFailure {
		return Result{}, localError()
	}
	eligible := false
	fresh := false
	age := time.Duration(0)
	if state == loadValid {
		age = now.Sub(loaded.fetched)
		if age < 0 {
			state = loadCorrupt
		} else {
			fresh = age <= options.FreshTTL
			eligible = age <= options.StaleIfError
		}
	}
	if state == loadValid && fresh && !options.Force {
		return resultFor(path, loaded, StateFresh, age), nil
	}

	if fetch != nil {
		catalog, fetchErr := fetch(ctx)
		if fetchErr == nil && catalog.Len() > 0 && validateCatalog(catalog) == nil {
			fetchedAt := options.Now().UTC()
			data, digest, encodeErr := encodeCache(catalog, fetchedAt)
			if encodeErr == nil {
				if persistErr := persistCache(ctx, path, data, options); persistErr != nil {
					return Result{}, localError()
				}
				return resultFor(path, loadedCache{catalog: catalog, fetched: fetchedAt, digest: digest}, StateRefreshed, 0), nil
			}
		}
	}
	if state == loadValid && eligible && !options.FreshOnly {
		return resultFor(path, loaded, StateDegraded, age), nil
	}
	return Result{}, unavailableError()
}

func withDefaults(options Options) Options {
	if options.FreshTTL <= 0 {
		options.FreshTTL = DefaultFreshTTL
	}
	if options.StaleIfError <= 0 {
		options.StaleIfError = DefaultStaleIfError
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.LockTimeout <= 0 {
		options.LockTimeout = defaultLockTimeout
	}
	return options
}

func resultFor(path string, loaded loadedCache, state State, age time.Duration) Result {
	return Result{Catalog: loaded.catalog, Status: Status{State: state, Path: path, Count: loaded.catalog.Len(), Age: age, Digest: loaded.digest}}
}

func validateCatalog(catalog Catalog) error {
	if catalog.Len() == 0 || catalog.Len() > MaxEntries {
		return ErrInvalidMetadata
	}
	for _, entry := range catalog.List() {
		if len(entry.id) == 0 || len(entry.id) > MaxIDBytes || strings.TrimSpace(entry.id) == "" || !safeText(entry.id) ||
			len(entry.name) > MaxNameBytes || entry.name != strings.TrimSpace(entry.name) || !safeText(entry.name) ||
			len(entry.imageURL) > MaxImageURLBytes || entry.imageURL != strings.TrimSpace(entry.imageURL) || !safeText(entry.imageURL) {
			return ErrInvalidMetadata
		}
	}
	return nil
}

func canonicalEntries(catalog Catalog) ([]byte, error) {
	if err := validateCatalog(catalog); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, entry := range catalog.List() {
		if index > 0 {
			buffer.WriteByte(',')
		}
		id, _ := json.Marshal(entry.id)
		name, _ := json.Marshal(entry.name)
		image, _ := json.Marshal(entry.imageURL)
		buffer.Write(id)
		buffer.WriteString(`:{"nameEn":`)
		buffer.Write(name)
		buffer.WriteString(`,"imageUrl":`)
		buffer.Write(image)
		buffer.WriteByte('}')
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func encodeCache(catalog Catalog, fetchedAt time.Time) ([]byte, string, error) {
	entries, err := canonicalEntries(catalog)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(entries)
	digest := hex.EncodeToString(sum[:])
	var buffer bytes.Buffer
	buffer.WriteString(`{"schemaVersion":1,"source":"raidnow-aryPokemon","fetchedAt":`)
	timestamp, _ := json.Marshal(fetchedAt.UTC().Format(time.RFC3339))
	buffer.Write(timestamp)
	buffer.WriteString(`,"contentDigest":`)
	digestJSON, _ := json.Marshal(digest)
	buffer.Write(digestJSON)
	buffer.WriteString(`,"entries":`)
	buffer.Write(entries)
	buffer.WriteString("}\n")
	if buffer.Len() > MaxCacheBytes {
		return nil, "", ErrInvalidMetadata
	}
	return buffer.Bytes(), digest, nil
}

func loadCache(path string) (loadedCache, loadState) {
	return loadCacheWithOptions(path, Options{})
}

func loadCacheWithOptions(path string, options Options) (loadedCache, loadState) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return loadedCache{}, loadMissing
	}
	if err != nil {
		return loadedCache{}, loadLocalFailure
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return loadedCache{}, loadLocalFailure
	}
	if info.Size() == 0 || info.Size() > MaxCacheBytes {
		return loadedCache{}, loadCorrupt
	}
	if options.beforeCacheOpen != nil {
		options.beforeCacheOpen(path)
	}
	file, info, err := openCacheFileNoFollow(path)
	if err != nil {
		return loadedCache{}, loadLocalFailure
	}
	if info.Size() == 0 || info.Size() > MaxCacheBytes {
		_ = file.Close()
		return loadedCache{}, loadCorrupt
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxCacheBytes+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return loadedCache{}, loadLocalFailure
	}
	loaded, err := decodeCache(data)
	if err != nil {
		return loadedCache{}, loadCorrupt
	}
	return loaded, loadValid
}

func decodeCache(data []byte) (loadedCache, error) {
	if len(data) == 0 || len(data) > MaxCacheBytes || validateJSON(data) != nil {
		return loadedCache{}, ErrInvalidMetadata
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || len(top) != 5 {
		return loadedCache{}, ErrInvalidMetadata
	}
	for _, key := range []string{"schemaVersion", "source", "fetchedAt", "contentDigest", "entries"} {
		if _, ok := top[key]; !ok {
			return loadedCache{}, ErrInvalidMetadata
		}
	}
	var document cacheDocument
	if err := json.Unmarshal(data, &document); err != nil || document.SchemaVersion != cacheSchemaVersion || document.Source != cacheSource {
		return loadedCache{}, ErrInvalidMetadata
	}
	fetched, err := time.Parse(time.RFC3339, document.FetchedAt)
	if err != nil || len(document.ContentDigest) != sha256.Size*2 {
		return loadedCache{}, ErrInvalidMetadata
	}
	decodedDigest, err := hex.DecodeString(document.ContentDigest)
	if err != nil || hex.EncodeToString(decodedDigest) != document.ContentDigest || len(document.Entries) == 0 || len(document.Entries) > MaxEntries {
		return loadedCache{}, ErrInvalidMetadata
	}
	var rawEntries map[string]json.RawMessage
	if err := json.Unmarshal(top["entries"], &rawEntries); err != nil {
		return loadedCache{}, ErrInvalidMetadata
	}
	entries := make(map[string]Entry, len(document.Entries))
	for id, value := range document.Entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawEntries[id], &fields); err != nil || len(fields) != 2 {
			return loadedCache{}, ErrInvalidMetadata
		}
		nameRaw, nameOK := fields["nameEn"]
		imageRaw, imageOK := fields["imageUrl"]
		var rawName, rawImage string
		if !nameOK || !imageOK || len(bytes.TrimSpace(nameRaw)) == 0 || bytes.TrimSpace(nameRaw)[0] != '"' ||
			len(bytes.TrimSpace(imageRaw)) == 0 || bytes.TrimSpace(imageRaw)[0] != '"' ||
			json.Unmarshal(nameRaw, &rawName) != nil || json.Unmarshal(imageRaw, &rawImage) != nil ||
			rawName != value.NameEn || rawImage != value.ImageURL {
			return loadedCache{}, ErrInvalidMetadata
		}
		entries[id] = Entry{id: id, name: value.NameEn, imageURL: value.ImageURL}
	}
	catalog := Catalog{entries: entries}
	canonical, err := canonicalEntries(catalog)
	if err != nil {
		return loadedCache{}, ErrInvalidMetadata
	}
	sum := sha256.Sum256(canonical)
	if !bytes.Equal(decodedDigest, sum[:]) {
		return loadedCache{}, ErrInvalidMetadata
	}
	return loadedCache{catalog: catalog, fetched: fetched.UTC(), digest: document.ContentDigest}, nil
}

func persistCache(ctx context.Context, path string, data []byte, options Options) error {
	dir := filepath.Dir(path)
	if err := prepareCacheDirectory(dir); err != nil {
		return err
	}
	unlock, err := acquireLock(ctx, path+".lock", options)
	if err != nil {
		return err
	}
	defer unlock()
	if info, statErr := os.Lstat(path); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return errors.New("unsafe cache destination")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	temp, err := os.CreateTemp(dir, ".ary-pokemon-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() { _ = temp.Close(); _ = os.Remove(tempName) }
	defer cleanup()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	write := options.writeFile
	if write == nil {
		write = func(file *os.File, contents []byte) error {
			_, err := file.Write(contents)
			return err
		}
	}
	if err := write(temp, data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if options.beforeReplaceTemp != nil {
		options.beforeReplaceTemp(tempName)
	}
	rename := options.rename
	if rename == nil {
		rename = replaceCacheFile
	}
	if err := finalizeTempReplacement(temp, tempName, path, rename); err != nil {
		return err
	}
	if directory, openErr := os.Open(dir); openErr == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

// Advisory locks serialize writers across goroutines and processes. The OS
// releases the lock when the process exits, so crashes do not leave stale locks.
func acquireLock(ctx context.Context, path string, options Options) (func(), error) {
	deadline := time.NewTimer(options.LockTimeout)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if info, err := os.Lstat(path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, errors.New("unsafe lock")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		unlock, acquired, err := tryAdvisoryLock(path)
		if err != nil {
			return nil, err
		}
		if acquired {
			return unlock, nil
		}
		if options.beforeLockWait != nil {
			options.beforeLockWait()
		}
		retry := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !retry.Stop() {
				select {
				case <-retry.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-deadline.C:
			if !retry.Stop() {
				select {
				case <-retry.C:
				default:
				}
			}
			return nil, errors.New("lock timeout")
		case <-retry.C:
		}
	}
}

// DisplayForID performs exact-ID lookup only and falls back to the original ID.
func DisplayForID(catalog Catalog, id string) (name, imageURL string) {
	entry, ok := catalog.Lookup(id)
	if !ok || strings.TrimSpace(entry.Name()) == "" {
		return id, ""
	}
	return entry.Name(), entry.ImageURL()
}
