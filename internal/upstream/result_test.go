package upstream

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestPostFormEmptyResponsePolicyComesFromRegistry(t *testing.T) {
	testCases := []struct {
		name      string
		operation Operation
		body      string
		wantExit  int
	}{
		{name: "empty rejected", operation: ParList, body: "", wantExit: output.ExitUpstreamRejection},
		{name: "whitespace rejected", operation: ParList, body: " \n\t", wantExit: output.ExitUpstreamRejection},
		{name: "malformed rejected", operation: ParList, body: `{"secret-marker":`, wantExit: output.ExitUpstreamRejection},
		{name: "empty allowed only for user info", operation: UserInfoPost, body: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := &trackingReadCloser{Reader: strings.NewReader(testCase.body)}
			client := responseClient(t, http.StatusOK, body)
			result, err := client.PostForm(context.Background(), testCase.operation, nil, CallOptions{})
			if testCase.wantExit != 0 {
				assertExitCode(t, err, testCase.wantExit)
				assertNoSecret(t, err)
			} else {
				if err != nil {
					t.Fatalf("post form: %v", err)
				}
				if len(result) != 0 {
					t.Fatalf("result = %q, want empty", result)
				}
			}
			if !body.closed.Load() {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestPostFormReturnsExactValidJSONInDefensiveCopy(t *testing.T) {
	payload := []byte("  {\"value\":[1,2]} \n")
	body := &mutableBody{payload: payload}
	client := responseClient(t, http.StatusOK, body)

	result, err := client.PostForm(context.Background(), ParList, nil, CallOptions{})
	if err != nil {
		t.Fatalf("post form: %v", err)
	}
	if !bytes.Equal(result, payload) {
		t.Fatalf("result = %q, want %q", result, payload)
	}
	for index := range payload {
		payload[index] = 'x'
	}
	if got := string(result); got != "  {\"value\":[1,2]} \n" {
		t.Fatalf("result changed with response storage: %q", got)
	}
	result[0] = '!'
	if body.payload[0] == '!' {
		t.Fatal("result aliases response body storage")
	}
	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

func TestPostFormRejectsResponseOverOneMiB(t *testing.T) {
	payload := append([]byte(`{"data":"`), bytes.Repeat([]byte{'a'}, maxResponseBytes)...)
	payload = append(payload, []byte(`"}`)...)
	body := &trackingReadCloser{Reader: bytes.NewReader(payload)}
	client := responseClient(t, http.StatusOK, body)

	_, err := client.PostForm(context.Background(), ParList, nil, CallOptions{})
	assertExitCode(t, err, output.ExitUpstreamRejection)
	assertNoSecret(t, err)
	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

func TestPostFormClosesBodyForEveryHTTPStatus(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError, http.StatusFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := &trackingReadCloser{Reader: strings.NewReader(`{"ok":true,"secret":"secret-marker"}`)}
			client := responseClient(t, status, body)
			_, err := client.PostForm(context.Background(), ParList, nil, CallOptions{Timeout: time.Second})
			if status == http.StatusOK {
				if err != nil {
					t.Fatalf("post form: %v", err)
				}
			} else {
				assertExitCode(t, err, output.ExitUpstreamRejection)
				assertNoSecret(t, err)
			}
			if !body.closed.Load() {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func responseClient(t *testing.T, status int, body io.ReadCloser) *Client {
	t.Helper()
	return mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		signalWroteRequest(request, nil)
		return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}, nil
	})}})
}

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (body *trackingReadCloser) Close() error {
	body.closed.Store(true)
	return nil
}

type mutableBody struct {
	payload []byte
	offset  int
	closed  atomic.Bool
}

func (body *mutableBody) Read(destination []byte) (int, error) {
	if body.offset >= len(body.payload) {
		return 0, io.EOF
	}
	count := copy(destination, body.payload[body.offset:])
	body.offset += count
	return count, nil
}

func (body *mutableBody) Close() error {
	body.closed.Store(true)
	return nil
}
