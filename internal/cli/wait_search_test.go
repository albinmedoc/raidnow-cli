package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) {
	panic("stdin must not be read")
}

func runPrimitiveCommandContext(t *testing.T, ctx context.Context, operations *stubRaidOperations, input string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}, Operations: operations, TokenFile: "-", RequestTimeout: 3 * time.Second})
	command.SetContext(ctx)
	command.SetIn(strings.NewReader(input))
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	err := command.Execute()
	return stdout.String(), err
}

func TestQueueWaitPollsImmediatelyAndRendersMatch(t *testing.T) {
	for _, testCase := range []struct {
		name string
		json bool
		want string
	}{
		{name: "human", want: "matched raid-7\n"},
		{name: "json", json: true, want: "{\"ok\":true,\"command\":\"queue wait\",\"status\":\"matched\",\"data\":{\"state\":\"matched\",\"raidId\":\"raid-7\"},\"warnings\":[]}\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueMatched, RaidID: "raid-7"}}
			args := []string{"queue", "wait", "--rank", "5", "--pokemon", "150"}
			if testCase.json {
				args = append([]string{"--json"}, args...)
			}
			stdout, err := runPrimitiveCommand(t, operations, "opaque-token\n", args...)
			if err != nil || stdout != testCase.want {
				t.Fatalf("stdout=%q err=%v", stdout, err)
			}
			if names := primitiveCallNames(operations.calls); strings.Join(names, ",") != "poll" {
				t.Fatalf("calls=%v", names)
			}
		})
	}
}

func TestQueueWaitWaitsThenMatchesWithoutCleanup(t *testing.T) {
	operations := &stubRaidOperations{polls: []domain.QueuePoll{{State: domain.QueueWaiting}, {State: domain.QueueMatched, RaidID: "raid-8"}}}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "queue", "wait", "--rank", "5", "--pokemon", "150", "--poll-interval", "250ms", "--max-wait", "2s")
	if err != nil || stdout != "matched raid-8\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if names := strings.Join(primitiveCallNames(operations.calls), ","); names != "poll,poll" {
		t.Fatalf("calls=%s", names)
	}
}

func TestQueueWaitTimeoutAndCancellationNeverCancelUpstream(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueWaiting}}
		_, err := runPrimitiveCommand(t, operations, "token\n", "queue", "wait", "--rank", "5", "--pokemon", "150", "--max-wait", "1ms")
		if output.ExitCodeFor(err) != output.ExitNotFound || strings.Join(primitiveCallNames(operations.calls), ",") != "poll" {
			t.Fatalf("exit=%d calls=%v err=%v", output.ExitCodeFor(err), primitiveCallNames(operations.calls), err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueWaiting}}
		operations.afterPoll = func(int) { cancel() }
		_, err := runPrimitiveCommandContext(t, ctx, operations, "token\n", "queue", "wait", "--rank", "5", "--pokemon", "150")
		if output.ExitCodeFor(err) != output.ExitReadNetwork || strings.Join(primitiveCallNames(operations.calls), ",") != "poll" {
			t.Fatalf("exit=%d calls=%v err=%v", output.ExitCodeFor(err), primitiveCallNames(operations.calls), err)
		}
	})
}

func TestSearchCreatesOnceThenReturnsMatchedQueue(t *testing.T) {
	for _, testCase := range []struct {
		name string
		json bool
		want string
	}{
		{name: "human", want: "matched raid-9\n"},
		{name: "json", json: true, want: "{\"ok\":true,\"command\":\"search\",\"status\":\"matched\",\"data\":{\"state\":\"matched\",\"raidId\":\"raid-9\"},\"warnings\":[]}\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueMatched, RaidID: "raid-9"}}
			args := searchArguments(t)
			if testCase.json {
				args = append([]string{"--json"}, args...)
			}
			stdout, err := runPrimitiveCommand(t, operations, "opaque-token\n", args...)
			if err != nil || stdout != testCase.want {
				t.Fatalf("stdout=%q err=%v", stdout, err)
			}
			if names := strings.Join(primitiveCallNames(operations.calls), ","); names != "create,poll" {
				t.Fatalf("calls=%s", names)
			}
		})
	}
}

func TestSearchWaitsThenMatchesWithOneCreateAndTwoPolls(t *testing.T) {
	operations := &stubRaidOperations{polls: []domain.QueuePoll{{State: domain.QueueWaiting}, {State: domain.QueueMatched, RaidID: "raid-later"}}}
	args := append(searchArguments(t), "--poll-interval", "250ms", "--max-wait", "2s")
	stdout, err := runPrimitiveCommand(t, operations, "token\n", args...)
	if err != nil || stdout != "matched raid-later\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if names := strings.Join(primitiveCallNames(operations.calls), ","); names != "create,poll,poll" {
		t.Fatalf("calls=%s", names)
	}
}

func TestSearchMatchWinsConcurrentCancellationAndSkipsCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueMatched, RaidID: "raid-race"}}
	operations.afterPoll = func(int) { cancel() }
	stdout, err := runPrimitiveCommandContext(t, ctx, operations, "token\n", searchArguments(t)...)
	if err != nil || stdout != "matched raid-race\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if names := strings.Join(primitiveCallNames(operations.calls), ","); names != "create,poll" {
		t.Fatalf("calls=%s", names)
	}
}

func TestSearchTimeoutCleansUpOnceAndReturnsNotFoundWhenAbsent(t *testing.T) {
	operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueWaiting}}
	args := append(searchArguments(t), "--max-wait", "1ms")
	_, err := runPrimitiveCommand(t, operations, "token\n", args...)
	if output.ExitCodeFor(err) != output.ExitNotFound {
		t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
	}
	if names := strings.Join(primitiveCallNames(operations.calls), ","); names != "create,poll,cancel,check" {
		t.Fatalf("calls=%s", names)
	}
}

func TestSearchCancellationCleansUpOnceAndReturnsReadErrorWhenAbsent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueWaiting}}
	operations.afterPoll = func(int) { cancel() }
	_, err := runPrimitiveCommandContext(t, ctx, operations, "token\n", searchArguments(t)...)
	if output.ExitCodeFor(err) != output.ExitReadNetwork {
		t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
	}
	if names := strings.Join(primitiveCallNames(operations.calls), ","); names != "create,poll,cancel,check" {
		t.Fatalf("calls=%s", names)
	}
	if operations.cancelCtxErr != nil || operations.checkCtxErr != nil {
		t.Fatalf("cleanup inherited cancelled context: cancel=%v check=%v", operations.cancelCtxErr, operations.checkCtxErr)
	}
}

func TestSearchCleanupPreservesUnknownWriteAndDetectsRemainingQueue(t *testing.T) {
	t.Run("unknown cancel", func(t *testing.T) {
		operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueWaiting}, cancelErr: output.NewError(output.CodeWriteOutcomeUnknown, "SECRET-TOKEN")}
		args := append(searchArguments(t), "--max-wait", "1ms")
		_, err := runPrimitiveCommand(t, operations, "token\n", args...)
		if output.ExitCodeFor(err) != output.ExitWriteOutcomeUnknown || strings.Contains(err.Error(), "SECRET-TOKEN") {
			t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
		}
		if names := strings.Join(primitiveCallNames(operations.calls), ","); names != "create,poll,cancel,check" {
			t.Fatalf("calls=%s", names)
		}
	})

	t.Run("still present", func(t *testing.T) {
		operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueWaiting}, check: domain.QueueCheck{Present: true}}
		args := append(searchArguments(t), "--max-wait", "1ms")
		_, err := runPrimitiveCommand(t, operations, "token\n", args...)
		if output.ExitCodeFor(err) != output.ExitLifecycleConflict {
			t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
		}
	})

	t.Run("check failed", func(t *testing.T) {
		operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueWaiting}, checkErr: errors.New("SECRET-CHECK")}
		args := append(searchArguments(t), "--max-wait", "1ms")
		_, err := runPrimitiveCommand(t, operations, "token\n", args...)
		if output.ExitCodeFor(err) != output.ExitPartialSuccess || strings.Contains(err.Error(), "SECRET-CHECK") {
			t.Fatalf("exit=%d err=%v", output.ExitCodeFor(err), err)
		}
	})
}

func TestWaitAndSearchValidateBeforeNetwork(t *testing.T) {
	for _, args := range [][]string{
		{"queue", "wait", "--rank", "5", "--pokemon", "150", "--poll-interval", "249ms"},
		{"queue", "wait", "--rank", "5", "--pokemon", "150", "--max-wait", "0"},
		append(searchArguments(t), "--poll-interval", "249ms"),
		append(searchArguments(t), "--max-wait", "0"),
	} {
		operations := &stubRaidOperations{}
		_, err := runPrimitiveCommand(t, operations, "token\n", args...)
		if output.ExitCodeFor(err) == output.ExitSuccess || len(operations.calls) != 0 {
			t.Fatalf("args=%v exit=%d calls=%v", args, output.ExitCodeFor(err), operations.calls)
		}
	}
}

func TestSearchRejectsDualStdinBeforeReading(t *testing.T) {
	operations := &stubRaidOperations{}
	args := []string{"search", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", "-", "--rank", "5", "--pokemon", "150"}
	var stdout bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}, Operations: operations, TokenFile: "-", RequestTimeout: time.Second})
	command.SetIn(io.Reader(panicReader{}))
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	err := command.Execute()
	if output.ExitCodeFor(err) != output.ExitInvalidArguments || len(operations.calls) != 0 {
		t.Fatalf("exit=%d calls=%v err=%v", output.ExitCodeFor(err), operations.calls, err)
	}
}

func searchArguments(t *testing.T) []string {
	t.Helper()
	return []string{"search", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "se", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012"), "--rank", "5", "--pokemon", "150", "--guest-score", "4"}
}

func primitiveCallNames(calls []primitiveCall) []string {
	names := make([]string, len(calls))
	for index := range calls {
		names[index] = calls[index].name
	}
	return names
}
