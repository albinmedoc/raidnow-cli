package secureinput

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRemovesExactlyOneTerminalLineEndingWithoutMutatingFile(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "lf", content: "1234 5678 9012\n", want: "1234 5678 9012"},
		{name: "crlf", content: "1234 5678 9012\r\n", want: "1234 5678 9012"},
		{name: "two_line_endings", content: "1234 5678 9012\n\n", want: "1234 5678 9012\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read before: %v", err)
			}
			got, err := Read(path, strings.NewReader("unused"), 64)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if string(got) != testCase.want {
				t.Fatalf("value = %q, want %q", got, testCase.want)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read after: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("file changed: got %q, want %q", after, before)
			}
		})
	}
}

func TestReadUsesProvidedStdinAndRejectsMissingOversizeOrReadFailure(t *testing.T) {
	got, err := Read("-", strings.NewReader("123456789012\n"), 12)
	if err != nil {
		t.Fatalf("Read stdin: %v", err)
	}
	if string(got) != "123456789012" {
		t.Fatalf("stdin value = %q", got)
	}
	for _, testCase := range []struct {
		name  string
		path  string
		stdin io.Reader
		max   int
	}{
		{name: "nil_stdin", path: "-", max: 12},
		{name: "missing_path", path: filepath.Join(t.TempDir(), "missing"), stdin: strings.NewReader("unused"), max: 12},
		{name: "empty_path", path: "", stdin: strings.NewReader("unused"), max: 12},
		{name: "oversize", path: "-", stdin: strings.NewReader("1234567890123"), max: 12},
		{name: "read_failure", path: "-", stdin: failingReader{}, max: 12},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Read(testCase.path, testCase.stdin, testCase.max); err == nil {
				t.Fatal("Read: got nil error, want rejection")
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("SECRET-READ-FAILURE")
}
