//go:build unix

package secureinput

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadRejectsUnsafeUnixDescriptors(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("123456789012"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := Read(link, strings.NewReader("unused"), 64); err == nil {
		t.Fatal("symlink: got nil error, want rejection")
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatalf("chmod target: %v", err)
	}
	if _, err := Read(target, strings.NewReader("unused"), 64); err == nil {
		t.Fatal("group-readable file: got nil error, want rejection")
	}
	if _, err := Read(directory, strings.NewReader("unused"), 64); err == nil {
		t.Fatal("directory: got nil error, want rejection")
	}
}

func TestReadRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("create FIFO: %v", err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := Read(path, strings.NewReader("unused"), 64)
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("FIFO: got nil error, want rejection")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("FIFO read blocked")
	}
}
