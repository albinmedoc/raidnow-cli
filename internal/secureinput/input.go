package secureinput

import (
	"errors"
	"io"
	"os"
)

var ErrInvalidInput = errors.New("invalid protected input")

// Read obtains a bounded secret from an explicitly selected protected file or
// from the supplied stdin when path is "-". It removes exactly one terminal LF
// or CRLF and never creates or modifies files.
func Read(path string, stdin io.Reader, maxBytes int) ([]byte, error) {
	if path == "" || maxBytes < 1 {
		return nil, ErrInvalidInput
	}
	var reader io.Reader
	var file *os.File
	if path == "-" {
		if stdin == nil {
			return nil, ErrInvalidInput
		}
		reader = stdin
	} else {
		opened, err := openProtectedFile(path)
		if err != nil {
			return nil, ErrInvalidInput
		}
		file = opened
		defer file.Close()
		reader = file
	}

	content, err := io.ReadAll(io.LimitReader(reader, int64(maxBytes)+3))
	if err != nil {
		return nil, ErrInvalidInput
	}
	if len(content) > 0 && content[len(content)-1] == '\n' {
		content = content[:len(content)-1]
		if len(content) > 0 && content[len(content)-1] == '\r' {
			content = content[:len(content)-1]
		}
	}
	if len(content) > maxBytes {
		return nil, ErrInvalidInput
	}
	return content, nil
}
