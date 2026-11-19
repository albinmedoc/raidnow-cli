package output

import (
	"bytes"
	"errors"
	"testing"
)

func TestRedactSecretsAndStandaloneTrainerCodes(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		secrets []string
		want    string
	}{
		{name: "secret and compact code", input: "token abc123, tränarkod 123456789012, raid 12345, lång 9123456789012", secrets: []string{"abc123", ""}, want: "token [REDACTED], tränarkod [REDACTED], raid 12345, lång 9123456789012"},
		{name: "overlapping secrets", input: "prefix-secret prefix", secrets: []string{"prefix", "prefix-secret"}, want: "[REDACTED] [REDACTED]"},
		{name: "multiple codes", input: "123456789012 234567890123", want: "[REDACTED] [REDACTED]"},
		{name: "formatted codes", input: "compact (123456789012), spaced: 1234 5678 9012; hyphenated=1234-5678-9012!", want: "compact ([REDACTED]), spaced: [REDACTED]; hyphenated=[REDACTED]!"},
		{name: "codes inside identifiers", input: "raid123456789012prod code1234 5678 9012prod code1234-5678-9012prod", want: "raid123456789012prod code1234 5678 9012prod code1234-5678-9012prod"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Redact(testCase.input, testCase.secrets...); got != testCase.want {
				t.Fatalf("redacted text = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestRenderErrorUsesPublicSanitizedError(t *testing.T) {
	var destination bytes.Buffer
	if err := RenderError(&destination, NewError(CodeNotFound, "Ingen raid för token-value och 123456789012"), "token-value"); err != nil {
		t.Fatal(err)
	}
	const want = "Fel [NOT_FOUND]: Ingen raid för [REDACTED] och [REDACTED]\n"
	if destination.String() != want {
		t.Fatalf("output = %q, want %q", destination.String(), want)
	}

	destination.Reset()
	if err := RenderError(&destination, errors.New("token=raw-secret")); err != nil {
		t.Fatal(err)
	}
	const wantUnknown = "Fel [INTERNAL_ERROR]: Ett oväntat internt fel inträffade.\n"
	if destination.String() != wantUnknown {
		t.Fatalf("unknown output = %q, want %q", destination.String(), wantUnknown)
	}
}
