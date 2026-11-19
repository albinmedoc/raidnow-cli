package metadata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// validateCachePathComponents walks from the volume root to dir with Lstat.
// Missing suffixes are allowed, but every existing component must be a real
// directory rather than a symlink, reparse point, or non-directory.
func validateCachePathComponents(dir string) (bool, error) {
	clean := filepath.Clean(dir)
	if !filepath.IsAbs(clean) {
		return false, errors.New("cache directory is not absolute")
	}
	volume := filepath.VolumeName(clean)
	root := volume + string(os.PathSeparator)
	relative := strings.TrimPrefix(clean, root)
	current := root
	if relative == "" {
		info, err := os.Lstat(current)
		return err == nil, validateDirectoryComponent(current, info, err)
	}
	parts := strings.Split(relative, string(os.PathSeparator))
	for index, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err := validateDirectoryComponent(current, info, err); err != nil {
			return false, err
		}
		if index == len(parts)-1 {
			return true, nil
		}
	}
	return false, nil
}

func validateDirectoryComponent(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || pathComponentIsReparsePoint(path) || !info.IsDir() {
		return errors.New("unsafe cache directory component")
	}
	return nil
}

func prepareCacheDirectory(dir string) error {
	existed, err := validateCachePathComponents(dir)
	if err != nil {
		return err
	}
	if existed {
		return validateCacheDirectorySecurity(dir, false)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if _, err := validateCachePathComponents(dir); err != nil {
		return err
	}
	if err := validateCacheDirectorySecurity(dir, true); err != nil {
		return err
	}
	_, err = validateCachePathComponents(dir)
	return err
}
