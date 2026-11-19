package output

import (
	"errors"
	"testing"
)

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

func TestRenderersPropagateWriterFailure(t *testing.T) {
	if err := RenderJSON(failingWriter{}, NewSuccess("version", "ok", "dev", nil)); !errors.Is(err, errWriteFailed) {
		t.Fatalf("render JSON error = %v, want writer failure", err)
	}
	if err := RenderError(failingWriter{}, NewError(CodeNotFound, "Saknas")); !errors.Is(err, errWriteFailed) {
		t.Fatalf("render error = %v, want writer failure", err)
	}
}
