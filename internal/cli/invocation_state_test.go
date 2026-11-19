package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestReusableRootResetsJSONModeAfterSuccessfulLeaf(t *testing.T) {
	bootstrap := &stubTokenBootstrapper{token: "fresh-token"}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: bootstrap})

	var first bytes.Buffer
	command.SetOut(&first)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--json", "token", "get"})
	if err := command.Execute(); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if !strings.HasPrefix(first.String(), `{"ok":true`) {
		t.Fatalf("first output=%q", first.String())
	}

	var second bytes.Buffer
	command.SetOut(&second)
	command.SetArgs([]string{"token", "get"})
	if err := command.Execute(); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if got, want := second.String(), "fresh-token\n"; got != want {
		t.Fatalf("second output=%q want=%q", got, want)
	}
	assertAllFlagsAtDefaults(t, command)
}

func TestReusableRootDoesNotReuseTrainerOrTokenFlags(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	defaultToken := protectedTokenFile(t, "token-b\n")
	firstToken := protectedTokenFile(t, "token-a\n")
	command := NewRootCommandWithOptions(RootOptions{Publisher: publisher, Config: &RootConfig{TokenFile: defaultToken}})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(publishArguments(t, firstToken, "AshA", "123456789010", "--request-timeout=9s"))
	if err := command.Execute(); err != nil {
		t.Fatalf("first execute: %v", err)
	}

	command.SetArgs([]string{"trainer", "publish"})
	err := command.Execute()
	if got := output.ExitCodeFor(err); got != output.ExitInvalidInput {
		t.Fatalf("second exit=%d err=%v", got, err)
	}
	if calls := publisherCalls(publisher); len(calls) != 1 {
		t.Fatalf("publisher calls=%d want=1", len(calls))
	}
	assertAllFlagsAtDefaults(t, command)
}

func TestReusableRootUsesDefaultTimeoutAndTokenSourceOnLaterInvocation(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	defaultToken := protectedTokenFile(t, "token-b\n")
	firstToken := protectedTokenFile(t, "token-a\n")
	command := NewRootCommandWithOptions(RootOptions{Publisher: publisher, Config: &RootConfig{TokenFile: defaultToken, RequestTimeout: 2 * time.Second}})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	command.SetArgs(publishArguments(t, firstToken, "AshA", "123456789010", "--request-timeout=9s"))
	if err := command.Execute(); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	command.SetArgs(publishArguments(t, "", "AshB", "123456789011"))
	if err := command.Execute(); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	calls := publisherCalls(publisher)
	if len(calls) != 2 || calls[0].token != "token-a" || calls[0].options.Timeout != 9*time.Second || calls[1].token != "token-b" || calls[1].options.Timeout != 2*time.Second || calls[1].trainer.NameFormValue() != "AshB" {
		t.Fatalf("calls=%#v", calls)
	}
	assertAllFlagsAtDefaults(t, command)
}

func TestReusableRootRecoversAfterFlagParseAndArgsErrors(t *testing.T) {
	bootstrap := &stubTokenBootstrapper{token: "fresh-token"}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: bootstrap})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	for _, bad := range [][]string{{"--json", "--request-timeout", "bad", "token", "get"}, {"token", "get", "extra"}} {
		command.SetArgs(bad)
		if err := command.Execute(); output.ExitCodeFor(err) != output.ExitInvalidArguments {
			t.Fatalf("args=%v exit=%d err=%v", bad, output.ExitCodeFor(err), err)
		}
		assertAllFlagsAtDefaults(t, command)
		var stdout bytes.Buffer
		command.SetOut(&stdout)
		command.SetArgs([]string{"token", "get"})
		if err := command.Execute(); err != nil {
			t.Fatalf("valid execute after %v: %v", bad, err)
		}
		if stdout.String() != "fresh-token\n" {
			t.Fatalf("valid output=%q", stdout.String())
		}
	}
}

func TestReusableRootDoesNotReuseStdinSelectionsAfterError(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	defaultToken := protectedTokenFile(t, "default-token\n")
	command := NewRootCommandWithOptions(RootOptions{Publisher: publisher, Config: &RootConfig{TokenFile: defaultToken}})
	command.SetIn(&countingReader{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--token-file", "-", "trainer", "publish", "--trainer-name", "AshA", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", "-"})
	if err := command.Execute(); output.ExitCodeFor(err) != output.ExitInvalidArguments {
		t.Fatalf("dual stdin exit=%d err=%v", output.ExitCodeFor(err), err)
	}

	command.SetArgs(publishArguments(t, "", "AshB", "123456789011"))
	if err := command.Execute(); err != nil {
		t.Fatalf("valid execute: %v", err)
	}
	calls := publisherCalls(publisher)
	if len(calls) != 1 || calls[0].token != "default-token" || calls[0].trainer.NameFormValue() != "AshB" {
		t.Fatalf("calls=%#v", calls)
	}
	assertAllFlagsAtDefaults(t, command)
}

func TestRootConfigIsCopiedAndIndependentAcrossRoots(t *testing.T) {
	defaults := &RootConfig{TokenFile: protectedTokenFile(t, "default-token\n"), RequestTimeout: 4 * time.Second}
	want := *defaults
	publisher := &stubTrainerPublisher{}
	roots := []*cobra.Command{
		NewRootCommandWithOptions(RootOptions{Publisher: publisher, Config: defaults}),
		NewRootCommandWithOptions(RootOptions{Publisher: publisher, Config: defaults}),
	}
	arguments := [][]string{
		publishArguments(t, "", "Ash0", "123456789010", "--request-timeout=1s"),
		publishArguments(t, "", "Ash1", "123456789011", "--request-timeout=2s"),
	}
	var wait sync.WaitGroup
	for index, root := range roots {
		wait.Add(1)
		go func(index int, root *cobra.Command) {
			defer wait.Done()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(arguments[index])
			if err := root.Execute(); err != nil {
				t.Errorf("root %d: %v", index, err)
			}
		}(index, root)
	}
	wait.Wait()
	if !reflect.DeepEqual(*defaults, want) {
		t.Fatalf("config mutated: got=%#v want=%#v", *defaults, want)
	}
	calls := publisherCalls(publisher)
	if len(calls) != 2 {
		t.Fatalf("calls=%d", len(calls))
	}
	timeouts := map[time.Duration]bool{}
	for _, call := range calls {
		timeouts[call.options.Timeout] = true
	}
	if !timeouts[time.Second] || !timeouts[2*time.Second] {
		t.Fatalf("timeouts=%#v", timeouts)
	}

	roots[0].SetArgs(publishArguments(t, "", "Ash2", "123456789012", "--request-timeout=3s"))
	if err := roots[0].Execute(); err != nil {
		t.Fatalf("sequential first root: %v", err)
	}
	roots[1].SetArgs(publishArguments(t, "", "Ash3", "123456789013"))
	if err := roots[1].Execute(); err != nil {
		t.Fatalf("sequential second root: %v", err)
	}
	calls = publisherCalls(publisher)
	if len(calls) != 4 || calls[2].options.Timeout != 3*time.Second || calls[3].options.Timeout != 4*time.Second {
		t.Fatalf("sequential calls=%#v", calls)
	}
	if !reflect.DeepEqual(*defaults, want) {
		t.Fatalf("config mutated after sequential use: got=%#v want=%#v", *defaults, want)
	}
}

func TestUnknownNestedCommandsAreUsageErrorsAndResetFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "trainer", "bogus"},
		{"trainer", "login"},
		{"token", "bogus"},
		{"trainer", "bogus", "deeper"},
	} {
		command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}})
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs(args)
		err := command.Execute()
		if got := output.ExitCodeFor(err); got != output.ExitInvalidArguments {
			t.Fatalf("args=%v exit=%d err=%v", args, got, err)
		}
		var usage *UsageError
		if !errors.As(err, &usage) {
			t.Fatalf("args=%v error type=%T", args, err)
		}
		assertAllFlagsAtDefaults(t, command)
	}
}

func publishArguments(t *testing.T, tokenFile, name, code string, extras ...string) []string {
	t.Helper()
	args := append([]string{}, extras...)
	if tokenFile != "" {
		args = append(args, "--token-file", tokenFile)
	}
	return append(args, "trainer", "publish", "--trainer-name", name, "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, code))
}

func protectedTokenFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	return path
}

func publisherCalls(publisher *stubTrainerPublisher) []publishCall {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	return append([]publishCall(nil), publisher.calls...)
}

func assertAllFlagsAtDefaults(t *testing.T, root *cobra.Command) {
	t.Helper()
	visit := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(flag *pflag.Flag) {
			if flag.Changed || flag.Value.String() != flag.DefValue {
				t.Errorf("flag --%s value=%q default=%q changed=%v", flag.Name, flag.Value.String(), flag.DefValue, flag.Changed)
			}
		})
	}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		visit(command.LocalNonPersistentFlags())
		visit(command.PersistentFlags())
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
}
