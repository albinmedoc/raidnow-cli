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

func TestBinaryInvalidCommandExitsNonzero(t *testing.T) {
	binary := buildBinary(t)
	command := exec.Command(binary, "definitely-not-a-command")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("execute invalid command: got nil error, want nonzero exit\noutput:\n%s", output)
	}

	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("execute invalid command error type = %T, want *exec.ExitError", err)
	}
	if exitError.ExitCode() == 0 {
		t.Fatalf("invalid command exit code = 0, want nonzero\noutput:\n%s", output)
	}
}

func TestHelpAndVersionLeaveWorkingDirectoryEmpty(t *testing.T) {
	binary := buildBinary(t)

	testCases := []struct {
		name      string
		arguments []string
	}{
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

func TestHelpAndVersionDoNotUseConfiguredNetworkProxy(t *testing.T) {
	binary := buildBinary(t)
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(response, "network access is forbidden in help and version", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)

	testCases := []struct {
		name      string
		arguments []string
	}{
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
		t.Fatalf("help/version made %d request(s) through the configured network proxy", got)
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
