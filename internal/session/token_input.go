package session

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
)

var errInvalidTokenInput = errors.New("invalid token input")

func ReadTokenInput(path string, stdin io.Reader) (string, error) {
	var reader io.Reader
	var file *os.File
	if path == "-" {
		if stdin == nil {
			return "", errInvalidTokenInput
		}
		reader = stdin
	} else {
		var err error
		file, err = os.Open(path)
		if err != nil {
			return "", errInvalidTokenInput
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() || !info.Mode().IsRegular() {
			return "", errInvalidTokenInput
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return "", errInvalidTokenInput
		}
		reader = file
	}

	content, err := io.ReadAll(io.LimitReader(reader, MaxTokenBytes+3))
	if err != nil || len(content) > MaxTokenBytes+2 {
		return "", errInvalidTokenInput
	}
	if len(content) > 0 && content[len(content)-1] == '\n' {
		content = content[:len(content)-1]
		if len(content) > 0 && content[len(content)-1] == '\r' {
			content = content[:len(content)-1]
		}
	}
	if len(content) == 0 || len(content) > MaxTokenBytes || strings.TrimSpace(string(content)) == "" {
		return "", errInvalidTokenInput
	}
	if strings.ContainsAny(string(content), "\r\n\x00") {
		return "", errInvalidTokenInput
	}
	return string(content), nil
}
