package output

import (
	"bytes"
	"errors"
	"testing"
)

func TestRenderJSONSuccessEnvelopeHasStableOrderAndEmptyWarnings(t *testing.T) {
	var destination bytes.Buffer
	envelope := NewSuccess("version", "ok", struct {
		Version string `json:"version"`
	}{Version: "dev"}, nil, "")

	if err := RenderJSON(&destination, envelope); err != nil {
		t.Fatalf("render JSON success: %v", err)
	}

	const want = "{\"ok\":true,\"command\":\"version\",\"status\":\"ok\",\"data\":{\"version\":\"dev\"},\"warnings\":[]}\n"
	if got := destination.String(); got != want {
		t.Fatalf("JSON success = %q, want %q", got, want)
	}
}

func TestTypedResultBuildsSuccessEnvelope(t *testing.T) {
	result := Result[string]{
		Command:     "token get",
		Status:      "ok",
		Data:        "issued",
		Warnings:    []string{"Spara token säkert."},
		OperationID: "operation-1",
	}

	envelope := result.Envelope()
	if !envelope.OK || envelope.Command != result.Command || envelope.Data != result.Data {
		t.Fatalf("result envelope = %#v, want command and typed data preserved", envelope)
	}
	if len(envelope.Warnings) != 1 || envelope.Warnings[0] != result.Warnings[0] {
		t.Fatalf("result warnings = %#v, want %#v", envelope.Warnings, result.Warnings)
	}
	if envelope.OperationID != result.OperationID {
		t.Fatalf("operation ID = %q, want %q", envelope.OperationID, result.OperationID)
	}
}

func TestRenderJSONFailureEnvelopeHasStableSchema(t *testing.T) {
	var destination bytes.Buffer
	envelope := NewFailure("search", "failed", NewError(CodeNotFound, "Ingen raid hittades."), "operation-7")

	if err := RenderJSON(&destination, envelope); err != nil {
		t.Fatalf("render JSON failure: %v", err)
	}

	const want = "{\"ok\":false,\"command\":\"search\",\"status\":\"failed\",\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"Ingen raid hittades.\"},\"operationId\":\"operation-7\"}\n"
	if got := destination.String(); got != want {
		t.Fatalf("JSON failure = %q, want %q", got, want)
	}
}

func TestRenderJSONReturnsEncodingErrors(t *testing.T) {
	envelope := NewSuccess("search", "ok", make(chan int), nil, "")

	if err := RenderJSON(&bytes.Buffer{}, envelope); err == nil {
		t.Fatal("render unsupported JSON value: got nil error, want encoding error")
	}
}

func TestFailureEnvelopeHidesUnknownErrorDetails(t *testing.T) {
	var destination bytes.Buffer
	envelope := NewFailure("search", "failed", errors.New("token=do-not-print"), "")

	if err := RenderJSON(&destination, envelope); err != nil {
		t.Fatalf("render unknown failure: %v", err)
	}

	const want = "{\"ok\":false,\"command\":\"search\",\"status\":\"failed\",\"error\":{\"code\":\"INTERNAL_ERROR\",\"message\":\"Ett oväntat internt fel inträffade.\"}}\n"
	if got := destination.String(); got != want {
		t.Fatalf("JSON unknown failure = %q, want %q", got, want)
	}
}

func TestFailureEnvelopeRedactsKnownErrorMessage(t *testing.T) {
	var destination bytes.Buffer
	err := NewError(CodeInvalidInput, "Ogiltig token-value för 123456789012")
	envelope := NewFailure("search", "failed", err, "", "token-value")

	if renderError := RenderJSON(&destination, envelope); renderError != nil {
		t.Fatalf("render redacted failure: %v", renderError)
	}

	const want = "{\"ok\":false,\"command\":\"search\",\"status\":\"failed\",\"error\":{\"code\":\"INVALID_INPUT\",\"message\":\"Ogiltig [REDACTED] för [REDACTED]\"}}\n"
	if got := destination.String(); got != want {
		t.Fatalf("JSON redacted failure = %q, want %q", got, want)
	}
}
