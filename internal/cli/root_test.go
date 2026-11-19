package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelpContainsCommandNameAndDescription(t *testing.T) {
	var output bytes.Buffer
	command := NewRootCommand()
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute root help: %v", err)
	}

	help := output.String()
	for _, expected := range []string{"raidnow", "Search for Raid NOW raids from your terminal"} {
		if !strings.Contains(help, expected) {
			t.Errorf("help output %q does not contain %q", help, expected)
		}
	}
}

func TestVersionEmitsStableDevelopmentVersion(t *testing.T) {
	var output bytes.Buffer
	command := NewRootCommand()
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"version"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	if got, want := output.String(), "raidnow dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestVersionUsesInjectedPackageVersion(t *testing.T) {
	previousVersion := Version
	Version = "v9.8.7-test"
	t.Cleanup(func() {
		Version = previousVersion
	})

	var output bytes.Buffer
	command := NewRootCommand()
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"version"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute version command: %v", err)
	}

	if got, want := output.String(), "raidnow v9.8.7-test\n"; got != want {
		t.Fatalf("version output = %q, want injected %q", got, want)
	}
}

func TestRootRejectsUnexpectedPositionalArguments(t *testing.T) {
	command := NewRootCommand()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--", "unexpected"})

	if err := command.Execute(); err == nil {
		t.Fatal("execute root command with positional argument: got nil error, want rejection")
	}
}
