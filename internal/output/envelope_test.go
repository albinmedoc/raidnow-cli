package output

import (
	"bytes"
	"errors"
	"testing"
)

func TestRenderJSONSuccessHasStableSchemaAndEmptyWarnings(t *testing.T) {
	var destination bytes.Buffer
	envelope := NewSuccess("version", "ok", struct {
		Version string `json:"version"`
	}{Version: "dev"}, nil)

	if err := RenderJSON(&destination, envelope); err != nil {
		t.Fatalf("render JSON success: %v", err)
	}
	const want = "{\"schemaVersion\":1,\"ok\":true,\"command\":\"version\",\"status\":\"ok\",\"data\":{\"version\":\"dev\"},\"warnings\":[]}\n"
	if got := destination.String(); got != want {
		t.Fatalf("JSON success = %q, want %q", got, want)
	}
}

func TestRenderJSONFailureHasStableSchema(t *testing.T) {
	var destination bytes.Buffer
	envelope := NewFailure("search", "failed", NewError(CodeNotFound, "Ingen raid hittades."))

	if err := RenderJSON(&destination, envelope); err != nil {
		t.Fatalf("render JSON failure: %v", err)
	}
	const want = "{\"schemaVersion\":1,\"ok\":false,\"command\":\"search\",\"status\":\"failed\",\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"Ingen raid hittades.\"}}\n"
	if got := destination.String(); got != want {
		t.Fatalf("JSON failure = %q, want %q", got, want)
	}
}

func TestRenderJSONReturnsEncodingErrors(t *testing.T) {
	if err := RenderJSON(&bytes.Buffer{}, NewSuccess("search", "ok", make(chan int), nil)); err == nil {
		t.Fatal("render unsupported JSON value: got nil error, want encoding error")
	}
}

func TestFailureHidesUnknownDetailsAndRedactsSecrets(t *testing.T) {
	var unknown bytes.Buffer
	if err := RenderJSON(&unknown, NewFailure("search", "failed", errors.New("token=do-not-print"))); err != nil {
		t.Fatal(err)
	}
	const wantUnknown = "{\"schemaVersion\":1,\"ok\":false,\"command\":\"search\",\"status\":\"failed\",\"error\":{\"code\":\"INTERNAL_ERROR\",\"message\":\"Ett oväntat internt fel inträffade.\"}}\n"
	if unknown.String() != wantUnknown {
		t.Fatalf("unknown failure = %q, want %q", unknown.String(), wantUnknown)
	}

	var redacted bytes.Buffer
	err := NewError(CodeInvalidInput, "Ogiltig token-value för 123456789012")
	if renderError := RenderJSON(&redacted, NewFailure("search", "failed", err, "token-value")); renderError != nil {
		t.Fatal(renderError)
	}
	const wantRedacted = "{\"schemaVersion\":1,\"ok\":false,\"command\":\"search\",\"status\":\"failed\",\"error\":{\"code\":\"INVALID_INPUT\",\"message\":\"Ogiltig [REDACTED] för [REDACTED]\"}}\n"
	if redacted.String() != wantRedacted {
		t.Fatalf("redacted failure = %q, want %q", redacted.String(), wantRedacted)
	}
}
