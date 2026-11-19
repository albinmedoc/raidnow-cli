//go:build !windows

package metadata

import "os"

func replaceCacheFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func finalizeTempReplacement(file *os.File, tempName, destination string, rename func(string, string) error) error {
	if _, err := validateTempPathIdentity(file, tempName); err != nil {
		return err
	}
	// Unix rename permits the validated descriptor to remain open through the
	// atomic namespace replacement.
	return rename(tempName, destination)
}
