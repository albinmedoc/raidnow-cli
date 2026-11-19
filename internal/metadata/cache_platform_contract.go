package metadata

import "os"

var (
	_ func(string) (*os.File, os.FileInfo, error)                      = openCacheFileNoFollow
	_ func(string) (func(), bool, error)                               = tryAdvisoryLock
	_ func(string, string) error                                       = replaceCacheFile
	_ func(*os.File, string, string, func(string, string) error) error = finalizeTempReplacement
	_ func(string) bool                                                = pathComponentIsReparsePoint
	_ func(string, bool) error                                         = validateCacheDirectorySecurity
)
