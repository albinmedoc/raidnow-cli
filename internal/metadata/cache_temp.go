package metadata

import (
	"errors"
	"os"
)

func validateTempPathIdentity(file *os.File, path string) (os.FileInfo, error) {
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return nil, errors.New("unsafe temporary cache file")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || pathComponentIsReparsePoint(path) || !pathInfo.Mode().IsRegular() {
		return nil, errors.New("unsafe temporary cache path")
	}
	if !os.SameFile(opened, pathInfo) {
		return nil, errors.New("temporary cache identity changed")
	}
	return opened, nil
}
