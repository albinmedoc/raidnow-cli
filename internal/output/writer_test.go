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

func TestRenderJSONPropagatesWriterFailure(t *testing.T) {
	err := RenderJSON(failingWriter{}, NewSuccess("version", "ok", "dev", nil, ""))
	if !errors.Is(err, errWriteFailed) {
		t.Fatalf("render JSON error = %v, want writer failure", err)
	}
}

func TestHumanRendererPropagatesWriterFailure(t *testing.T) {
	renderer := NewHumanRenderer(failingWriter{})
	testCases := []struct {
		name   string
		render func() error
	}{
		{name: "info", render: func() error { return renderer.Info("Söker") }},
		{name: "success", render: func() error { return renderer.Success("Klar") }},
		{name: "error", render: func() error { return renderer.Error(NewError(CodeNotFound, "Saknas")) }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.render(); !errors.Is(err, errWriteFailed) {
				t.Fatalf("human render error = %v, want writer failure", err)
			}
		})
	}
}
