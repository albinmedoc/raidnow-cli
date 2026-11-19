package upstream

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestKnownOperationsBindImmutableMetadata(t *testing.T) {
	testCases := []struct {
		operation  Operation
		name       string
		kind       OperationKind
		timeout    time.Duration
		allowEmpty bool
	}{
		{LineMatchingGetList, "line_matching_get_list", ReadOperation, 10 * time.Second, false},
		{ParList, "par_list", ReadOperation, 10 * time.Second, false},
		{LineMatchingList, "line_matching_list", ReadOperation, 10 * time.Second, false},
		{LineMatchingCheck, "line_matching_check", ReadOperation, 10 * time.Second, false},
		{ParFindFirst, "par_find_first", ReadOperation, 10 * time.Second, false},
		{ChildList, "child_list", ReadOperation, 10 * time.Second, false},
		{UserInfoPost, "user_info_post", WriteOperation, 15 * time.Second, true},
		{LineMatchingCreate, "line_matching_create", WriteOperation, 15 * time.Second, false},
		{LineMatchingUpdate, "line_matching_update", WriteOperation, 15 * time.Second, false},
		{LineMatchingCancel, "line_matching_cancel", WriteOperation, 15 * time.Second, false},
		{ChildPostFast, "child_post_fast", WriteOperation, 15 * time.Second, false},
		{GuestRaidDelPost, "guest_raid_del_post", WriteOperation, 15 * time.Second, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			descriptor, err := testCase.operation.Descriptor()
			if err != nil {
				t.Fatalf("descriptor: %v", err)
			}
			if descriptor.Name() != testCase.name {
				t.Fatalf("name = %q, want %q", descriptor.Name(), testCase.name)
			}
			if descriptor.Kind() != testCase.kind {
				t.Fatalf("kind = %v, want %v", descriptor.Kind(), testCase.kind)
			}
			if descriptor.DefaultTimeout() != testCase.timeout {
				t.Fatalf("timeout = %v, want %v", descriptor.DefaultTimeout(), testCase.timeout)
			}
			if descriptor.AllowsEmptyResponse() != testCase.allowEmpty {
				t.Fatalf("allows empty = %v, want %v", descriptor.AllowsEmptyResponse(), testCase.allowEmpty)
			}
		})
	}
}

func TestUnknownOperationsAreInvalid(t *testing.T) {
	for _, operation := range []Operation{0, Operation(255)} {
		if _, err := operation.Descriptor(); output.ExitCodeFor(err) != output.ExitInvalidArguments {
			t.Fatalf("operation %d error = %v, want invalid arguments", operation, err)
		}
	}
}

func TestPostFormRejectsUnknownOperationBeforeDispatch(t *testing.T) {
	var requests atomic.Int32
	client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, fmt.Errorf("must not dispatch")
	})}})
	for _, operation := range []Operation{0, Operation(255)} {
		_, err := client.PostForm(context.Background(), operation, nil, CallOptions{})
		if got := output.ExitCodeFor(err); got != output.ExitInvalidArguments {
			t.Fatalf("operation %d exit code = %d, want %d; error=%v", operation, got, output.ExitInvalidArguments, err)
		}
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestPublicCallOptionsCannotOverrideOperationSafety(t *testing.T) {
	optionsType := reflect.TypeOf(CallOptions{})
	for _, forbidden := range []string{"Kind", "Name", "AllowEmptyResponse"} {
		if _, found := optionsType.FieldByName(forbidden); found {
			t.Fatalf("CallOptions exposes forbidden %s override", forbidden)
		}
	}
	method, found := reflect.TypeOf((*Client)(nil)).MethodByName("PostForm")
	if !found {
		t.Fatal("PostForm method not found")
	}
	if got, want := method.Type.In(2), reflect.TypeOf(Operation(0)); got != want {
		t.Fatalf("PostForm operation parameter = %v, want %v", got, want)
	}
}
