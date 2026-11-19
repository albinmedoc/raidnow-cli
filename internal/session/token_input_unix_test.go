//go:build unix

package session

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadTokenInputRejectsFIFOImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("create FIFO: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := ReadTokenInput(path, strings.NewReader("unused"))
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("read FIFO: got nil error, want rejection")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("read FIFO blocked; want immediate descriptor-first rejection")
	}
}

func TestReadTokenInputUnixDescriptorChecksSymlinkAndMode(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if token, err := ReadTokenInput(link, strings.NewReader("unused")); err == nil || token != "" {
		t.Fatalf("read symlink: token=%q err=%v, want rejection", token, err)
	}

	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatalf("chmod target: %v", err)
	}
	if token, err := ReadTokenInput(target, strings.NewReader("unused")); err == nil || token != "" {
		t.Fatalf("read insecure mode: token=%q err=%v, want rejection", token, err)
	}
}

func TestReadTokenInputUnixRejectsDifferentOwnerWhenPermitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	if err := os.Chown(path, os.Geteuid()+1, -1); err != nil {
		t.Skipf("cannot create different-owner fixture: %v", err)
	}
	defer os.Chown(path, os.Geteuid(), -1)

	if token, err := ReadTokenInput(path, strings.NewReader("unused")); err == nil || token != "" {
		t.Fatalf("read different-owner file: token=%q err=%v, want rejection", token, err)
	}
}
