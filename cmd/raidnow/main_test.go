package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"

	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/cli"
	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
)

type failFirstWriter struct {
	writes int
	output strings.Builder
}

func (writer *failFirstWriter) Write(content []byte) (int, error) {
	writer.writes++
	if writer.writes == 1 {
		return 0, errors.New("SECRET-WRITER-FAILURE")
	}
	return writer.output.Write(content)
}

func TestBinaryWithoutArgumentsPrintsHelpAndSucceeds(t *testing.T) {
	binary := buildBinary(t)
	workingDirectory := t.TempDir()

	command := exec.Command(binary)
	command.Dir = workingDirectory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("execute binary without arguments: %v\noutput:\n%s", err, output)
	}

	for _, expected := range []string{"Usage:", "Use Raid NOW anonymously from your terminal"} {
		if !strings.Contains(string(output), expected) {
			t.Errorf("output %q does not contain %q", output, expected)
		}
	}
}

func TestBinaryMalformedCLIExitsTwoWithoutEchoingRawArguments(t *testing.T) {
	binary := buildBinary(t)
	const secret = "SECRET-RAW-ARGUMENT"
	testCases := []struct {
		name      string
		arguments []string
	}{
		{name: "unknown_command", arguments: []string{"unknown-" + secret}},
		{name: "invalid_flag", arguments: []string{"--unknown=" + secret}},
		{name: "root_positional", arguments: []string{"--", secret}},
		{name: "subcommand_positional", arguments: []string{"version", secret}},
		{name: "subcommand_invalid_flag", arguments: []string{"version", "--unknown=" + secret}},
		{name: "malformed_bool", arguments: []string{"--json=" + secret}},
		{name: "raw_token", arguments: []string{"--token", secret, "trainer", "publish"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command(binary, testCase.arguments...)
			var stdout strings.Builder
			var stderr strings.Builder
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if err == nil {
				t.Fatal("execute malformed CLI: got nil error, want exit 2")
			}
			exitError, ok := err.(*exec.ExitError)
			if !ok || exitError.ExitCode() != 2 {
				t.Fatalf("malformed CLI exit = %v, want code 2", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if got, want := stderr.String(), "Fel [INVALID_ARGUMENTS]: Ogiltiga kommandoradsargument.\n"; got != want {
				t.Fatalf("stderr = %q, want exact sanitized %q", got, want)
			}
			if strings.Contains(stderr.String(), secret) {
				t.Fatalf("stderr echoed raw argument: %q", stderr.String())
			}
		})
	}
}

func TestMalformedCLIWithJSONFlagUsesSanitizedInvalidArgumentsEnvelope(t *testing.T) {
	const secret = "SECRET-RAW-ARGUMENT"
	var stdout strings.Builder
	var stderr strings.Builder

	if exitCode := run([]string{"--json", "unknown-" + secret}, &stdout, &stderr); exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	want := "{\"ok\":false,\"command\":\"raidnow\",\"status\":\"failed\",\"error\":{\"code\":\"INVALID_ARGUMENTS\",\"message\":\"Ogiltiga kommandoradsargument.\"}}\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want exact %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Contains(stdout.String(), secret) {
		t.Fatalf("stdout echoed raw argument: %q", stdout.String())
	}
}

func TestVersionWriterFailureIsSanitizedInternalError(t *testing.T) {
	stdout := &failFirstWriter{}
	var stderr strings.Builder

	if exitCode := run([]string{"version"}, stdout, &stderr); exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if got, want := stderr.String(), "Fel [INTERNAL_ERROR]: Ett oväntat internt fel inträffade.\n"; got != want {
		t.Fatalf("stderr = %q, want exact %q", got, want)
	}
	if strings.Contains(stderr.String(), "SECRET-WRITER-FAILURE") {
		t.Fatalf("stderr leaked writer error: %q", stderr.String())
	}
}

func TestTokenWriterFailureIsSanitizedInternalJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte(`<script>new Vue({data:{gofriend_token:'safe-token'}})</script>`))
	}))
	t.Cleanup(server.Close)
	previousBootstrapURL := cli.BootstrapURL
	cli.BootstrapURL = server.URL
	t.Cleanup(func() { cli.BootstrapURL = previousBootstrapURL })
	stdout := &failFirstWriter{}
	var stderr strings.Builder

	if exitCode := run([]string{"--json", "token", "get"}, stdout, &stderr); exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	want := "{\"ok\":false,\"command\":\"token get\",\"status\":\"failed\",\"error\":{\"code\":\"INTERNAL_ERROR\",\"message\":\"Ett oväntat internt fel inträffade.\"}}\n"
	if got := stdout.output.String(); got != want {
		t.Fatalf("stdout recovery output = %q, want exact %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Contains(stdout.output.String(), "SECRET-WRITER-FAILURE") {
		t.Fatalf("stdout leaked writer error: %q", stdout.output.String())
	}
}

func TestNoArgumentsHelpAndVersionLeaveWorkingDirectoryEmpty(t *testing.T) {
	binary := buildBinary(t)

	testCases := []struct {
		name      string
		arguments []string
	}{
		{name: "no_arguments"},
		{name: "help", arguments: []string{"--help"}},
		{name: "version", arguments: []string{"version"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			workingDirectory := t.TempDir()
			command := exec.Command(binary, testCase.arguments...)
			command.Dir = workingDirectory
			command.Env = environmentWith(map[string]string{
				"HOME":            workingDirectory,
				"XDG_CACHE_HOME":  workingDirectory,
				"XDG_CONFIG_HOME": workingDirectory,
				"XDG_DATA_HOME":   workingDirectory,
			})
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("execute %s: %v\noutput:\n%s", testCase.name, err, output)
			}

			entries, err := os.ReadDir(workingDirectory)
			if err != nil {
				t.Fatalf("read working directory: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("working directory contains %d entries after %s: %v", len(entries), testCase.name, entries)
			}
		})
	}
}

func TestNoArgumentsHelpAndVersionDoNotUseConfiguredNetworkProxy(t *testing.T) {
	binary := buildBinary(t)
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(response, "network access is forbidden in no-argument, help, and version execution", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)

	testCases := []struct {
		name      string
		arguments []string
	}{
		{name: "no_arguments"},
		{name: "help", arguments: []string{"--help"}},
		{name: "version", arguments: []string{"version"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command(binary, testCase.arguments...)
			command.Dir = t.TempDir()
			command.Env = environmentWith(map[string]string{
				"ALL_PROXY":   proxy.URL,
				"HTTP_PROXY":  proxy.URL,
				"HTTPS_PROXY": proxy.URL,
				"NO_PROXY":    "",
				"all_proxy":   proxy.URL,
				"http_proxy":  proxy.URL,
				"https_proxy": proxy.URL,
				"no_proxy":    "",
			})
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("execute %s: %v\noutput:\n%s", testCase.name, err, output)
			}
		})
	}

	if got := requests.Load(); got != 0 {
		t.Fatalf("no-argument/help/version execution made %d request(s) through the configured network proxy", got)
	}
}

func TestBinaryTokenGetUsesOneStatelessRequestAndPrintsExactOutput(t *testing.T) {
	const token = "AbCdEf0123456789_-AbCdEf0123456789_-AbCdEfGH"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", request.Method)
		}
		if !strings.Contains(request.Header.Get("User-Agent"), "Mozilla/") {
			t.Errorf("User-Agent = %q, want browser-like value", request.Header.Get("User-Agent"))
		}
		if !strings.Contains(request.Header.Get("Accept"), "text/html") {
			t.Errorf("Accept = %q, want HTML", request.Header.Get("Accept"))
		}
		for _, forbidden := range []string{"Cookie", "X-CSRF-TOKEN"} {
			if value := request.Header.Get(forbidden); value != "" {
				t.Errorf("%s = %q, want empty", forbidden, value)
			}
		}
		response.Header().Set("Set-Cookie", "session=must-not-be-reused; Path=/")
		_, _ = response.Write([]byte("<script>new Vue({data:{gofriend_token:'" + token + "'}})</script>"))
	}))
	t.Cleanup(server.Close)

	binary := buildBinaryWithBootstrapURL(t, server.URL)
	for _, testCase := range []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "human", arguments: []string{"token", "get"}, want: token + "\n"},
		{name: "json", arguments: []string{"--json", "token", "get"}, want: `{"ok":true,"command":"token get","status":"secret","data":{"token":"` + token + `","containsSecret":true},"warnings":["Tokenen är hemlig; lagra och dela den säkert."]}` + "\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			workingDirectory := t.TempDir()
			command := exec.Command(binary, testCase.arguments...)
			command.Dir = workingDirectory
			command.Env = environmentWith(map[string]string{
				"HOME":            workingDirectory,
				"XDG_CACHE_HOME":  workingDirectory,
				"XDG_CONFIG_HOME": workingDirectory,
				"XDG_DATA_HOME":   workingDirectory,
			})
			var stdout strings.Builder
			var stderr strings.Builder
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("execute token get: %v; stderr=%q", err, stderr.String())
			}
			if stdout.String() != testCase.want {
				t.Fatalf("stdout = %q, want exact %q", stdout.String(), testCase.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			entries, err := os.ReadDir(workingDirectory)
			if err != nil {
				t.Fatalf("read working directory: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("token get created files: %v", entries)
			}
		})
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("request count = %d, want exactly one per invocation", got)
	}
}

func TestBinaryTokenGetJSONFailureUsesStableEnvelopeOnStdout(t *testing.T) {
	const secret = "SECRET-UPSTREAM-CONTENT"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Error(response, secret, http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	binary := buildBinaryWithBootstrapURL(t, server.URL)
	command := exec.Command(binary, "--json", "token", "get")
	var stdout strings.Builder
	var stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	if err == nil {
		t.Fatal("execute failed JSON token get: got nil error, want exit 8")
	}
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 8 {
		t.Fatalf("JSON token failure exit = %v, want code 8", err)
	}
	want := "{\"ok\":false,\"command\":\"token get\",\"status\":\"failed\",\"error\":{\"code\":\"READ_NETWORK_ERROR\",\"message\":\"Kunde inte hämta en anonym token från Raid NOW.\"}}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want exact %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Contains(stdout.String(), secret) {
		t.Fatalf("JSON failure leaked upstream content: %q", stdout.String())
	}
}

func TestBinaryTokenGetSanitizesUpstreamFailures(t *testing.T) {
	const secret = "SECRET-UPSTREAM-CONTENT"
	for _, testCase := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "non_2xx", status: http.StatusBadGateway, body: "<html>" + secret + "</html>"},
		{name: "missing_token", status: http.StatusOK, body: "<html>" + secret + "</html>"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				response.WriteHeader(testCase.status)
				_, _ = response.Write([]byte(testCase.body))
			}))
			t.Cleanup(server.Close)
			binary := buildBinaryWithBootstrapURL(t, server.URL)
			command := exec.Command(binary, "token", "get")
			var stdout strings.Builder
			var stderr strings.Builder
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if err == nil {
				t.Fatal("execute failed token get: got nil error, want nonzero exit")
			}
			exitError, ok := err.(*exec.ExitError)
			if !ok || exitError.ExitCode() != 8 {
				t.Fatalf("exit = %v, want code 8", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if got, want := stderr.String(), "Fel [READ_NETWORK_ERROR]: Kunde inte hämta en anonym token från Raid NOW.\n"; got != want {
				t.Fatalf("stderr = %q, want exact %q", got, want)
			}
			if strings.Contains(stderr.String(), secret) || strings.Contains(stderr.String(), "<html>") {
				t.Fatalf("stderr leaked upstream content: %q", stderr.String())
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("request count = %d, want 1 with no follow-up", got)
			}
		})
	}
}

type mainStubBootstrap struct{}

func (mainStubBootstrap) Token(context.Context) (string, error) { return "unused", nil }

type mainStubPublisher struct{ err error }

func (publisher *mainStubPublisher) PublishTrainer(context.Context, string, domain.Trainer, upstream.CallOptions) error {
	return publisher.err
}

func TestRunTrainerPublishExitOutputAndNoSideEffects(t *testing.T) {
	secrets := t.TempDir()
	tokenPath := filepath.Join(secrets, "token")
	codePath := filepath.Join(secrets, "code")
	if err := os.WriteFile(tokenPath, []byte("opaque-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codePath, []byte("123456789012\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseArgs := []string{"--token-file", tokenPath, "trainer", "publish", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", codePath}
	for _, testCase := range []struct {
		name      string
		json      bool
		publisher *mainStubPublisher
		wantExit  int
		wantOut   string
		wantErr   string
	}{
		{name: "human success", publisher: &mainStubPublisher{}, wantOut: "Tränaruppgifterna publicerades.\n"},
		{name: "json success", json: true, publisher: &mainStubPublisher{}, wantOut: "{\"ok\":true,\"command\":\"trainer publish\",\"status\":\"published\",\"data\":{\"published\":true},\"warnings\":[]}\n"},
		{name: "human uncertain", publisher: &mainStubPublisher{err: output.NewError(output.CodeWriteOutcomeUnknown, "secret-marker")}, wantExit: 9, wantErr: "Fel [WRITE_OUTCOME_UNKNOWN]: Upstream-anslutningen bröts och skrivningens utfall är okänt.\n"},
		{name: "json uncertain", json: true, publisher: &mainStubPublisher{err: output.NewError(output.CodeWriteOutcomeUnknown, "secret-marker")}, wantExit: 9, wantOut: "{\"ok\":false,\"command\":\"trainer publish\",\"status\":\"failed\",\"error\":{\"code\":\"WRITE_OUTCOME_UNKNOWN\",\"message\":\"Upstream-anslutningen bröts och skrivningens utfall är okänt.\"}}\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			working := t.TempDir()
			previous, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(working); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Chdir(previous) }()
			args := append([]string(nil), baseArgs...)
			if testCase.json {
				args = append([]string{"--json"}, args...)
			}
			var stdout, stderr strings.Builder
			exit := runWithOptions(args, &stdout, &stderr, cli.RootOptions{Bootstrap: mainStubBootstrap{}, Publisher: testCase.publisher, RequestTimeout: time.Second})
			if exit != testCase.wantExit || stdout.String() != testCase.wantOut || stderr.String() != testCase.wantErr {
				t.Fatalf("exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
			}
			entries, err := os.ReadDir(working)
			if err != nil || len(entries) != 0 {
				t.Fatalf("side effects=%v err=%v", entries, err)
			}
		})
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "raidnow")
	command := exec.Command("go", "build", "-o", binary, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build raidnow binary: %v\noutput:\n%s", err, output)
	}
	return binary
}

func buildBinaryWithBootstrapURL(t *testing.T, bootstrapURL string) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "raidnow")
	linkerFlags := "-X github.com/albinmedoc/raidnow-cli/internal/cli.BootstrapURL=" + bootstrapURL
	command := exec.Command("go", "build", "-ldflags", linkerFlags, "-o", binary, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build raidnow binary with bootstrap URL: %v\noutput:\n%s", err, output)
	}
	return binary
}

func environmentWith(overrides map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := overrides[key]; !overridden {
			environment = append(environment, entry)
		}
	}
	for key, value := range overrides {
		environment = append(environment, fmt.Sprintf("%s=%s", key, value))
	}
	return environment
}
