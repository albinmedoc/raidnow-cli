package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestTrainerInputFlagsReadNormalizedTrainerFromStdinWithoutCodeArgument(t *testing.T) {
	command := &cobra.Command{Use: "future"}
	input := BindTrainerInputFlags(command)
	command.SetIn(strings.NewReader("1234\u30005678 9012\n"))
	arguments := []string{
		"--trainer-name", " Ash123 ",
		"--trainer-level", "80",
		"--trainer-country", " WORLD ",
		"--trainer-code-file", "-",
	}
	if err := command.ParseFlags(arguments); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	trainer, err := input.Read(command)
	if err != nil {
		t.Fatalf("read trainer input: %v", err)
	}
	if trainer.NameFormValue() != "Ash123" || trainer.LevelFormValue() != "80" || trainer.CountryFormValue() != "world" || trainer.CodeFormValue() != "123456789012" {
		t.Fatalf("unexpected normalized trainer: %+v", trainer)
	}
	if command.Flags().Lookup("trainer-code") != nil {
		t.Fatal("raw --trainer-code flag must not exist")
	}
	for _, argument := range arguments {
		if strings.Contains(argument, "123456789012") {
			t.Fatalf("trainer code appeared in argv: %q", argument)
		}
	}
}

func TestTrainerInputFlagsReadProtectedFileWithoutWritingTrainerValues(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "trainer-code")
	const content = "1234 5678 9012\r\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write code fixture: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read code fixture: %v", err)
	}

	command := &cobra.Command{Use: "future"}
	input := BindTrainerInputFlags(command)
	if err := command.ParseFlags([]string{"--trainer-name", "Ash123", "--trainer-level", "1", "--trainer-country", "se", "--trainer-code-file", path}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	trainer, err := input.Read(command)
	if err != nil {
		t.Fatalf("read trainer input: %v", err)
	}
	if trainer.CodeFormValue() != "123456789012" {
		t.Fatal("protected file code was not normalized")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read code fixture after: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("trainer code file was modified")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "trainer-code" {
		t.Fatalf("trainer input created files: %v", entries)
	}
}

func TestTrainerInputFlagsReturnSanitizedInvalidInputForMissingCodeFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "SECRET-123456789012-missing")
	for _, testCase := range []struct {
		name      string
		arguments []string
	}{
		{name: "flag_omitted", arguments: []string{"--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "jp"}},
		{name: "path_missing", arguments: []string{"--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "jp", "--trainer-code-file", missing}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			command := &cobra.Command{Use: "future"}
			input := BindTrainerInputFlags(command)
			if err := command.ParseFlags(testCase.arguments); err != nil {
				t.Fatalf("parse flags: %v", err)
			}
			_, err := input.Read(command)
			if err == nil {
				t.Fatal("read missing code file: got nil error")
			}
			if got := output.ExitCodeFor(err); got != output.ExitInvalidInput {
				t.Fatalf("exit mapping = %d, want %d", got, output.ExitInvalidInput)
			}
			for _, rendered := range []string{err.Error(), strings.TrimSpace(string(mustJSON(t, output.NewFailure("future", "failed", err, ""))))} {
				if strings.Contains(rendered, "123456789012") || strings.Contains(rendered, missing) {
					t.Fatalf("error exposed code or path: %q", rendered)
				}
			}
		})
	}
}

func TestRootCommandTreeContainsOnlyStatelessTokenAndTrainerCommands(t *testing.T) {
	root := NewRootCommand()
	seen := map[string]bool{}
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		for _, child := range command.Commands() {
			seen[child.CommandPath()] = true
			visit(child)
		}
	}
	visit(root)
	for _, required := range []string{"raidnow token", "raidnow token get", "raidnow trainer", "raidnow trainer publish"} {
		if !seen[required] {
			t.Fatalf("missing command %q; tree=%#v", required, seen)
		}
	}
	for _, forbidden := range []string{"auth", "login", "profile", "import", "export"} {
		for path := range seen {
			if strings.Contains(path, " "+forbidden) {
				t.Fatalf("forbidden command %q in tree", path)
			}
		}
	}
	if root.PersistentFlags().Lookup("token") != nil {
		t.Fatal("raw --token flag must not exist")
	}
	if root.PersistentFlags().Lookup("request-timeout") == nil {
		t.Fatal("global --request-timeout flag is missing")
	}
}

func TestRootDoesNotExposeTrainerFlagsGlobally(t *testing.T) {
	command := NewRootCommand()
	for _, name := range []string{"trainer-name", "trainer-level", "trainer-country", "trainer-code-file"} {
		if command.PersistentFlags().Lookup(name) != nil || command.Flags().Lookup(name) != nil {
			t.Fatalf("root exposes misleading global --%s", name)
		}
	}
}

func mustJSON(t *testing.T, value output.Envelope) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := output.RenderJSON(&buffer, value); err != nil {
		t.Fatalf("render JSON: %v", err)
	}
	return buffer.Bytes()
}
