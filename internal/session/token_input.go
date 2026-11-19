package session

import (
	"errors"
	"io"
	"os"
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
		opened, err := openTokenFile(path)
		if err != nil {
			return "", errInvalidTokenInput
		}
		file = opened
		defer file.Close()
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
	value := string(content)
	if !tokenIsValid(value) {
		return "", errInvalidTokenInput
	}
	return value, nil
}
