package session

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadTokenInputReadsFileAndRemovesOneTerminalLineEnding(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "newline", content: " opaque token \n", want: " opaque token "},
		{name: "crlf", content: "opaque-token\r\n", want: "opaque-token"},
		{name: "no_line_ending", content: " opaque-token ", want: " opaque-token "},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("write token file: %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read token before: %v", err)
			}

			got, err := ReadTokenInput(path, strings.NewReader("unused"))
			if err != nil {
				t.Fatalf("read token input: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("token = %q, want exact %q", got, testCase.want)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read token after: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("token file changed: got %q, want %q", after, before)
			}
		})
	}
}

func TestReadTokenInputReadsDashFromProvidedStdin(t *testing.T) {
	got, err := ReadTokenInput("-", strings.NewReader("stdin-token\n"))
	if err != nil {
		t.Fatalf("read stdin token: %v", err)
	}
	if got != "stdin-token" {
		t.Fatalf("token = %q, want stdin token", got)
	}
}

func TestReadTokenInputRejectsInvalidContent(t *testing.T) {
	testCases := []struct {
		name    string
		content []byte
	}{
		{name: "empty"},
		{name: "whitespace_only", content: []byte(" 	 \n")},
		{name: "embedded_newline", content: []byte("part1\npart2")},
		{name: "second_terminal_newline", content: []byte("token\n\n")},
		{name: "embedded_cr", content: []byte("part1\rpart2")},
		{name: "nul", content: []byte("part1\x00part2")},
		{name: "invalid_utf8", content: []byte{0xff}},
		{name: "unicode_control", content: []byte("part\u0085part")},
		{name: "line_separator", content: []byte("part\u2028part")},
		{name: "paragraph_separator", content: []byte("part\u2029part")},
		{name: "oversized", content: []byte(strings.Repeat("x", MaxTokenBytes+1))},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ReadTokenInput("-", bytes.NewReader(testCase.content))
			if err == nil {
				t.Fatal("read invalid token: got nil error, want rejection")
			}
			if strings.Contains(err.Error(), "part1") || strings.Contains(err.Error(), strings.Repeat("x", 32)) {
				t.Fatalf("error %q leaks token content", err)
			}
		})
	}
}

func TestReadTokenInputRejectsDirectoryAndReadErrors(t *testing.T) {
	if _, err := ReadTokenInput(t.TempDir(), strings.NewReader("unused")); err == nil {
		t.Fatal("read directory: got nil error, want rejection")
	}
	if _, err := ReadTokenInput(filepath.Join(t.TempDir(), "missing"), strings.NewReader("unused")); err == nil {
		t.Fatal("read missing file: got nil error, want rejection")
	}
}

func TestReadTokenInputRejectsUnixSymlinkWithoutReturningTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix no-follow semantics are not portable to Windows")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	const secret = "symlink-target-secret"
	if err := os.WriteFile(target, []byte(secret), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(directory, "token")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	token, err := ReadTokenInput(link, strings.NewReader("unused"))
	if err == nil {
		t.Fatalf("read symlink token = %q, want rejection", token)
	}
	if token != "" || strings.Contains(err.Error(), secret) {
		t.Fatalf("symlink rejection exposed target: token=%q err=%q", token, err)
	}
}
