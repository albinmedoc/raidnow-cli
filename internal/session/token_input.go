package session

import (
	"errors"
	"io"

	"github.com/albinmedoc/raidnow-cli/internal/secureinput"
)

var errInvalidTokenInput = errors.New("invalid token input")

func ReadTokenInput(path string, stdin io.Reader) (string, error) {
	content, err := secureinput.Read(path, stdin, MaxTokenBytes)
	if err != nil {
		return "", errInvalidTokenInput
	}
	value := string(content)
	if !tokenIsValid(value) {
		return "", errInvalidTokenInput
	}
	return value, nil
}
