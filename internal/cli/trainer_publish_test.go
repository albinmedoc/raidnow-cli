package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
)

type publishCall struct {
	token   string
	trainer domain.Trainer
	options upstream.CallOptions
}

type stubTrainerPublisher struct {
	mutex sync.Mutex
	calls []publishCall
	err   error
}

func (stub *stubTrainerPublisher) PublishTrainer(_ context.Context, token string, trainer domain.Trainer, options upstream.CallOptions) error {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	stub.calls = append(stub.calls, publishCall{token: token, trainer: trainer, options: options})
	return stub.err
}

func TestTrainerPublishReadsInvocationSecretsAndPrintsExactHumanSuccess(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	var stdout, stderr bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher, TokenFile: "-", RequestTimeout: 3 * time.Second})
	command.SetIn(strings.NewReader("opaque-token\n"))
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"trainer", "publish", "--trainer-name", " Ash123 ", "--trainer-level", "80", "--trainer-country", " WORLD ", "--trainer-code-file", protectedTrainerCodeFile(t, "1234 5678 9012\n")})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := stdout.String(), "Tränaruppgifterna publicerades.\n"; got != want {
		t.Fatalf("stdout=%q want=%q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q", stderr.String())
	}
	if len(publisher.calls) != 1 {
		t.Fatalf("calls=%d", len(publisher.calls))
	}
	call := publisher.calls[0]
	if call.token != "opaque-token" || call.trainer.NameFormValue() != "Ash123" || call.trainer.CodeFormValue() != "123456789012" || call.trainer.LevelFormValue() != "80" || call.trainer.CountryFormValue() != "world" || call.options.Timeout != 3*time.Second {
		t.Fatalf("unexpected call: token=%q trainer=%+v options=%#v", call.token, call.trainer, call.options)
	}
}

func TestTrainerPublishJSONSuccessHasNoIdentityFields(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	var stdout bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher, TokenFile: "-"})
	command.SetIn(strings.NewReader("opaque-token\n"))
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--json", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012\n")})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"trainer publish\",\"status\":\"published\",\"data\":{\"published\":true},\"warnings\":[]}\n"
	if stdout.String() != want {
		t.Fatalf("stdout=%q want=%q", stdout.String(), want)
	}
	for _, secret := range []string{"opaque-token", "Ash123", "123456789012", "se"} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("JSON leaked %q: %q", secret, stdout.String())
		}
	}
}

func TestTrainerPublishRejectsMissingInvalidInputsBeforePublisher(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		in   string
	}{
		{name: "missing token file", args: []string{"trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "invalid token", in: "\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "missing trainer", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "missing name", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "missing level", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "missing country", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "missing code file", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se"}},
		{name: "invalid name", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "A!", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "invalid level", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "81", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "invalid country", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")}},
		{name: "bad code", in: "token\n", args: []string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "SECRET-123")}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			publisher := &stubTrainerPublisher{}
			command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher})
			command.SetIn(strings.NewReader(testCase.in))
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(testCase.args)
			err := command.Execute()
			if output.ExitCodeFor(err) != output.ExitInvalidInput {
				t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
			}
			if len(publisher.calls) != 0 {
				t.Fatal("publisher called")
			}
			for _, rendered := range []string{fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
				if strings.Contains(rendered, "SECRET") || strings.Contains(rendered, "123456789012") || strings.Contains(rendered, "opaque-token") {
					t.Fatalf("error leaked input: %q", rendered)
				}
			}
		})
	}
}

func TestTrainerPublishRejectsDualStdinBeforeReadingOrPublishing(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	reader := &countingReader{}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher})
	command.SetIn(reader)
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", "-"})
	err := command.Execute()
	if output.ExitCodeFor(err) != output.ExitInvalidArguments {
		t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
	}
	if reader.calls != 0 || len(publisher.calls) != 0 {
		t.Fatalf("reader calls=%d publisher calls=%d", reader.calls, len(publisher.calls))
	}
}

func TestTrainerPublishRejectsNegativeTimeoutBeforeReadingOrPublishing(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	reader := &countingReader{}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher})
	command.SetIn(reader)
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--request-timeout=-1s", "--token-file", "-", "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")})
	err := command.Execute()
	if output.ExitCodeFor(err) != output.ExitInvalidArguments || reader.calls != 0 || len(publisher.calls) != 0 {
		t.Fatalf("exit=%d reader=%d publisher=%d err=%v", output.ExitCodeFor(err), reader.calls, len(publisher.calls), err)
	}
}

func TestTrainerPublishPropagatesSanitizedPublisherErrors(t *testing.T) {
	publisher := &stubTrainerPublisher{err: output.NewError(output.CodeWriteOutcomeUnknown, "Upstream-utfallet är okänt.")}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher, TokenFile: "-"})
	command.SetIn(strings.NewReader("opaque-token\n"))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")})
	err := command.Execute()
	if output.ExitCodeFor(err) != output.ExitWriteOutcomeUnknown {
		t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
	}
}

func TestTrainerPublishRedactsAllInvocationValuesFromPublisherErrors(t *testing.T) {
	publisher := &stubTrainerPublisher{err: output.NewError(output.CodeUpstreamRejection, "opaque-token Ash123 123456789012 se")}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher, TokenFile: "-"})
	command.SetIn(strings.NewReader("opaque-token\n"))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012")})
	err := command.Execute()
	if output.ExitCodeFor(err) != output.ExitUpstreamRejection {
		t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
	}
	for _, rendered := range []string{fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err), string(mustJSON(t, output.NewFailure("trainer publish", "failed", err)))} {
		for _, secret := range []string{"opaque-token", "Ash123", "123456789012", " se"} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("error leaked %q: %q", secret, rendered)
			}
		}
	}
}

func TestTrainerPublishInvalidArgsAndUnsupportedRawTokenAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"trainer", "publish", "extra"}, {"trainer", "publish", "--token", "secret"}, {"--request-timeout", "bad", "trainer", "publish"}} {
		command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}})
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs(args)
		if err := command.Execute(); output.ExitCodeFor(err) != output.ExitInvalidArguments {
			t.Fatalf("args=%v exit=%d err=%v", args, output.ExitCodeFor(err), err)
		}
	}
}

func TestTrainerPublishConcurrentCommandsDoNotShareInputs(t *testing.T) {
	publisher := &stubTrainerPublisher{}
	codePaths := []string{
		protectedTrainerCodeFile(t, "123456789010"),
		protectedTrainerCodeFile(t, "123456789011"),
	}
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: publisher, TokenFile: "-"})
			command.SetIn(strings.NewReader(fmt.Sprintf("token-%d\n", index)))
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs([]string{"trainer", "publish", "--trainer-name", fmt.Sprintf("Ash%d", index), "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", codePaths[index]})
			if err := command.Execute(); err != nil {
				t.Errorf("execute: %v", err)
			}
		}(index)
	}
	wait.Wait()
	if len(publisher.calls) != 2 {
		t.Fatalf("calls=%d", len(publisher.calls))
	}
	seen := map[string]string{}
	for _, call := range publisher.calls {
		seen[call.token] = call.trainer.NameFormValue() + ":" + call.trainer.CodeFormValue()
	}
	if seen["token-0"] != "Ash0:123456789010" || seen["token-1"] != "Ash1:123456789011" {
		t.Fatalf("seen=%#v", seen)
	}
}

type countingReader struct{ calls int }

func (reader *countingReader) Read([]byte) (int, error) {
	reader.calls++
	return 0, errors.New("must not read")
}

func protectedTrainerCodeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trainer-code")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write trainer code: %v", err)
	}
	return path
}
