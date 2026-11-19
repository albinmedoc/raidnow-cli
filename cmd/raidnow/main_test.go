package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReadmeDocumentsSingleOpenNoclobberTokenSave(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	contents := string(readme)
	for _, required := range []string{
		"token_file=./raidnow.token",
		`umask 077; set -C; exec raidnow token get > "$token_file"`,
		"user-owned, non-world-writable directory",
		"existing regular file",
		"symbolic link to a regular file",
		"partial file",
		"explicitly remove",
		"special files",
		"O_NOFOLLOW",
	} {
		if !strings.Contains(contents, required) {
			t.Fatalf("README token-file example does not contain %q", required)
		}
	}
	for _, forbidden := range []string{"mktemp", "chmod 600", "stat -c"} {
		if strings.Contains(contents, forbidden) {
			t.Fatalf("README still uses check/reopen flow %q", forbidden)
		}
	}
}

func TestReadmeTokenFileExampleHasDeterministicNoclobberBehavior(t *testing.T) {
	script := readmeTokenSaveScript(t)
	directory := t.TempDir()
	fakeRaidnow := filepath.Join(directory, "raidnow")
	invocations := filepath.Join(directory, "invocations")
	fake := fmt.Sprintf("#!/bin/sh\nprintf invoked >> %q\nprintf 'example-secret-token\\n'\n", invocations)
	if err := os.WriteFile(fakeRaidnow, []byte(fake), 0o700); err != nil {
		t.Fatalf("write fake raidnow: %v", err)
	}
	run := func() error {
		command := exec.Command("bash", "-c", script)
		command.Dir = directory
		command.Env = environmentWith(map[string]string{"PATH": directory + string(os.PathListSeparator) + os.Getenv("PATH")})
		return command.Run()
	}

	if err := run(); err != nil {
		t.Fatalf("create token file: %v", err)
	}
	tokenPath := filepath.Join(directory, "raidnow.token")
	info, err := os.Lstat(tokenPath)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %v, want regular 0600", info.Mode())
	}
	content, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if got, want := string(content), "example-secret-token\n"; got != want {
		t.Fatalf("token file content = %q, want %q", got, want)
	}

	if err := os.WriteFile(tokenPath, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("replace token fixture: %v", err)
	}
	if err := os.Remove(invocations); err != nil {
		t.Fatalf("reset invocation marker: %v", err)
	}
	if err := run(); err == nil {
		t.Fatal("existing destination: got success, want noclobber failure")
	}
	if got, _ := os.ReadFile(tokenPath); string(got) != "existing\n" {
		t.Fatalf("existing destination changed to %q", got)
	}
	if _, err := os.Stat(invocations); !os.IsNotExist(err) {
		t.Fatalf("producer invoked for existing destination: %v", err)
	}

	if err := os.Remove(tokenPath); err != nil {
		t.Fatalf("remove existing destination: %v", err)
	}
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, tokenPath); err != nil {
		t.Fatalf("create destination symlink: %v", err)
	}
	if err := run(); err == nil {
		t.Fatal("symlink destination: got success, want noclobber failure")
	}
	if got, _ := os.ReadFile(target); string(got) != "target\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
	if _, err := os.Stat(invocations); !os.IsNotExist(err) {
		t.Fatalf("producer invoked for symlink destination: %v", err)
	}
}

func readmeTokenSaveScript(t *testing.T) string {
	t.Helper()
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	contents := string(readme)
	start := strings.Index(contents, "token_file=./raidnow.token")
	if start < 0 {
		t.Fatal("README token-file example start is missing")
	}
	end := strings.Index(contents[start:], "\n```")
	if end < 0 {
		t.Fatal("README token-file example end is missing")
	}
	return contents[start : start+end]
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

	for _, expected := range []string{"Usage:", "Search for Raid NOW raids from your terminal"} {
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
		{name: "malformed_bool", arguments: []string{"--json=" + secret}},
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
		{name: "json", arguments: []string{"--json", "token", "get"}, want: `{"token":"` + token + `"}` + "\n"},
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
