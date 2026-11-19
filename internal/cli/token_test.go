package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type stubTokenBootstrapper struct {
	token string
	err   error
	calls int
}

func (stub *stubTokenBootstrapper) Token(context.Context) (string, error) {
	stub.calls++
	return stub.token, stub.err
}

func TestTokenFileFlagIsPersistentConfiguredAndIgnoredByTokenGet(t *testing.T) {
	bootstrap := &stubTokenBootstrapper{token: "fresh-token"}
	config := &RootConfig{}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: bootstrap, Config: config})
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"--token-file", "/definitely/missing/raidnow.token", "token", "get"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute token get with token file configured: %v", err)
	}
	if got, want := config.TokenFile, "/definitely/missing/raidnow.token"; got != want {
		t.Fatalf("configured token file = %q, want %q", got, want)
	}
	if got, want := stdout.String(), "fresh-token\n"; got != want {
		t.Fatalf("stdout = %q, want exact %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if bootstrap.calls != 1 {
		t.Fatalf("bootstrap calls = %d, want 1", bootstrap.calls)
	}

	var help bytes.Buffer
	helpCommand := NewRootCommandWithOptions(RootOptions{Bootstrap: bootstrap, Config: &RootConfig{}})
	helpCommand.SetOut(&help)
	helpCommand.SetErr(&help)
	helpCommand.SetArgs([]string{"--help"})
	if err := helpCommand.Execute(); err != nil {
		t.Fatalf("execute root help: %v", err)
	}
	if !strings.Contains(help.String(), "--token-file") || !strings.Contains(help.String(), "path or - for stdin") {
		t.Fatalf("help output %q does not document --token-file path or stdin", help.String())
	}
}

func TestTokenGetPrintsExactRawToken(t *testing.T) {
	bootstrap := &stubTokenBootstrapper{token: "opaque anonymous token"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: bootstrap})
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"token", "get"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute token get: %v", err)
	}
	if got, want := stdout.String(), "opaque anonymous token\n"; got != want {
		t.Fatalf("stdout = %q, want exact %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if bootstrap.calls != 1 {
		t.Fatalf("bootstrap calls = %d, want 1", bootstrap.calls)
	}
}

func TestTokenGetJSONPrintsCompactTokenObject(t *testing.T) {
	bootstrap := &stubTokenBootstrapper{token: "opaque\"token"}
	var stdout bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: bootstrap})
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--json", "token", "get"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute JSON token get: %v", err)
	}
	if got, want := stdout.String(), "{\"token\":\"opaque\\\"token\"}\n"; got != want {
		t.Fatalf("stdout = %q, want exact %q", got, want)
	}
}

func TestTokenGetFailureIsSanitizedOnStderrOnly(t *testing.T) {
	secret := "SECRET-TOKEN-CONTENT"
	bootstrap := &stubTokenBootstrapper{err: errors.New("upstream HTML <html>" + secret + "</html>")}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: bootstrap})
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"token", "get"})

	err := command.Execute()
	if err == nil {
		t.Fatal("execute failed token get: got nil error, want nonzero result")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "Fel [READ_NETWORK_ERROR]: Kunde inte hämta en anonym token från Raid NOW.\n"; got != want {
		t.Fatalf("stderr = %q, want exact %q", got, want)
	}
	if strings.Contains(stderr.String(), secret) || strings.Contains(stderr.String(), "<html>") || strings.Contains(err.Error(), secret) {
		t.Fatalf("failure leaked secret or HTML: err=%q stderr=%q", err, stderr.String())
	}
}

func TestJSONFlagDoesNotChangeVersionOutput(t *testing.T) {
	var stdout bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}})
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--json", "version"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute JSON version: %v", err)
	}
	if got, want := stdout.String(), "raidnow dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
