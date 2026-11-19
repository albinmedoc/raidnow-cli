package secureinput

import (
	"errors"
	"io"
	"os"
)

var ErrInvalidInput = errors.New("invalid protected input")

// Read obtains a bounded secret from a regular file or from stdin when path is
// "-". It removes exactly one terminal LF or CRLF and never modifies the file.
func Read(path string, stdin io.Reader, maxBytes int) ([]byte, error) {
	if path == "" || maxBytes < 1 {
		return nil, ErrInvalidInput
	}

	reader := stdin
	if path != "-" {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, ErrInvalidInput
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, ErrInvalidInput
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, ErrInvalidInput
		}
		reader = file
	} else if reader == nil {
		return nil, ErrInvalidInput
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
