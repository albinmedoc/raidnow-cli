package output

import (
	"bytes"
	"errors"
	"testing"
)

func TestRedactSecretsAndStandaloneTrainerCodes(t *testing.T) {
	const input = "token abc123, tränarkod 123456789012, raid 12345, lång 9123456789012"

	got := Redact(input, "abc123", "")
	const want = "token [REDACTED], tränarkod [REDACTED], raid 12345, lång 9123456789012"
	if got != want {
		t.Fatalf("redacted text = %q, want %q", got, want)
	}
}

func TestRedactIsDeterministicForOverlappingSecrets(t *testing.T) {
	const input = "prefix-secret prefix"
	first := Redact(input, "prefix", "prefix-secret")
	second := Redact(input, "prefix-secret", "prefix")

	if first != second {
		t.Fatalf("redaction depends on secret order: first %q, second %q", first, second)
	}
	if first != "[REDACTED] [REDACTED]" {
		t.Fatalf("redacted text = %q, want both secrets redacted", first)
	}
}

func TestRedactMasksEveryStandaloneTrainerCode(t *testing.T) {
	const input = "123456789012 234567890123"
	const want = "[REDACTED] [REDACTED]"
	if got := Redact(input); got != want {
		t.Fatalf("redacted trainer codes = %q, want %q", got, want)
	}
}

func TestHumanRendererWritesSwedishTextToSelectedWriter(t *testing.T) {
	var destination bytes.Buffer
	renderer := NewHumanRenderer(&destination, "token-value")

	if err := renderer.Info("Söker med token-value"); err != nil {
		t.Fatalf("render info: %v", err)
	}
	if err := renderer.Success("Raid hittad för 123456789012"); err != nil {
		t.Fatalf("render success: %v", err)
	}
	if err := renderer.Error(NewError(CodeNotFound, "Ingen raid för token-value")); err != nil {
		t.Fatalf("render error: %v", err)
	}

	const want = "Info: Söker med [REDACTED]\nKlart: Raid hittad för [REDACTED]\nFel [NOT_FOUND]: Ingen raid för [REDACTED]\n"
	if got := destination.String(); got != want {
		t.Fatalf("human output = %q, want %q", got, want)
	}
}

func TestHumanRendererHidesUnknownErrorDetails(t *testing.T) {
	var destination bytes.Buffer
	renderer := NewHumanRenderer(&destination)

	if err := renderer.Error(errors.New("token=raw-secret")); err != nil {
		t.Fatalf("render unknown error: %v", err)
	}

	const want = "Fel [INTERNAL_ERROR]: Ett oväntat internt fel inträffade.\n"
	if got := destination.String(); got != want {
		t.Fatalf("unknown human error = %q, want %q", got, want)
	}
}
