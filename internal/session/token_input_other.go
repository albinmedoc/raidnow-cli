//go:build !unix && !windows

package session

import "os"

func openTokenFile(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errInvalidTokenInput
	}
	return file, nil
}
