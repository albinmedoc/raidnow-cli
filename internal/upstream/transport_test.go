package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestPostFormDoesNotFollowRedirectOrRetry(t *testing.T) {
	var sourceRequests atomic.Int32
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetRequests.Add(1)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		sourceRequests.Add(1)
		http.Redirect(response, request, target.URL+"/secret-marker", http.StatusFound)
	}))
	defer source.Close()

	client := mustClient(t, Config{Endpoint: source.URL, HTTPClient: source.Client(), AllowInsecureHTTP: true})
	_, err := client.PostForm(context.Background(), ParList, nil, CallOptions{})
	assertExitCode(t, err, output.ExitUpstreamRejection)
	assertNoSecret(t, err)
	if got := sourceRequests.Load(); got != 1 {
		t.Fatalf("source requests = %d, want 1", got)
	}
	if got := targetRequests.Load(); got != 0 {
		t.Fatalf("redirect target requests = %d, want 0", got)
	}
}

func TestPostFormDisablesInjectedCookieJar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if cookie := request.Header.Get("Cookie"); cookie != "" {
			t.Errorf("Cookie = %q, want none", cookie)
		}
		_, _ = response.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("new cookie jar: %v", err)
	}
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{Name: "session", Value: "secret-marker"}})
	httpClient := server.Client()
	httpClient.Jar = jar
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: httpClient, AllowInsecureHTTP: true})

	if _, err := client.PostForm(context.Background(), ParList, nil, CallOptions{}); err != nil {
		t.Fatalf("post form: %v", err)
	}
}

func TestCanceledWriteBeforeDispatchIsSafeReadNetworkFailure(t *testing.T) {
	var requests atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("must not dispatch")
	})}})

	_, err := client.PostForm(ctx, LineMatchingCreate, nil, CallOptions{})
	assertExitCode(t, err, output.ExitReadNetwork)
	assertNoSecret(t, err)
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestCancellationAfterRoundTripEntryBeforeRequestWriteIsSafe(t *testing.T) {
	for _, operation := range []Operation{ParList, LineMatchingCreate} {
		t.Run(operationName(t, operation), func(t *testing.T) {
			entered := make(chan struct{})
			client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				close(entered)
				<-request.Context().Done()
				return nil, errors.New("disconnect secret-marker")
			})}})
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				_, err := client.PostForm(ctx, operation, nil, CallOptions{Timeout: time.Second})
				done <- err
			}()
			<-entered
			cancel()
			assertExitCode(t, <-done, output.ExitReadNetwork)
		})
	}
}

func TestRealTransportDisconnectAfterDispatchUsesRegisteredClassification(t *testing.T) {
	testCases := []struct {
		operation Operation
		want      int
	}{
		{operation: ParList, want: output.ExitReadNetwork},
		{operation: LineMatchingCreate, want: output.ExitWriteOutcomeUnknown},
	}
	for _, testCase := range testCases {
		t.Run(operationName(t, testCase.operation), func(t *testing.T) {
			dispatched := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				hijacker, ok := response.(http.Hijacker)
				if !ok {
					t.Error("response writer does not support hijacking")
					return
				}
				connection, _, err := hijacker.Hijack()
				if err != nil {
					t.Errorf("hijack: %v", err)
					return
				}
				close(dispatched)
				_ = connection.Close()
			}))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

			done := make(chan error, 1)
			go func() {
				_, err := client.PostForm(context.Background(), testCase.operation, url.Values{"token": {"secret-marker"}}, CallOptions{})
				done <- err
			}()
			<-dispatched
			assertExitCode(t, <-done, testCase.want)
		})
	}
}

func TestUnexpectedEOFAfterDispatchUsesRegisteredClassification(t *testing.T) {
	testCases := []struct {
		operation Operation
		want      int
	}{
		{operation: ParList, want: output.ExitReadNetwork},
		{operation: LineMatchingCreate, want: output.ExitWriteOutcomeUnknown},
	}
	for _, testCase := range testCases {
		t.Run(operationName(t, testCase.operation), func(t *testing.T) {
			body := &errorBody{err: io.ErrUnexpectedEOF}
			client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				signalWroteRequest(request, nil)
				return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
			})}})
			_, err := client.PostForm(context.Background(), testCase.operation, nil, CallOptions{Timeout: time.Second})
			assertExitCode(t, err, testCase.want)
			assertNoSecret(t, err)
			if !body.closed.Load() {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestResponseMarksWriteDispatchedWithoutTraceCallback(t *testing.T) {
	body := &errorBody{err: io.ErrUnexpectedEOF}
	client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	})}})

	_, err := client.PostForm(context.Background(), LineMatchingCreate, nil, CallOptions{Timeout: time.Second})
	assertExitCode(t, err, output.ExitWriteOutcomeUnknown)
	assertNoSecret(t, err)
	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

func TestOversizedResponseTakesPrecedenceOverFinalReadError(t *testing.T) {
	for _, operation := range []Operation{ParList, LineMatchingCreate} {
		t.Run(operationName(t, operation), func(t *testing.T) {
			body := &bytesThenErrorBody{content: []byte(strings.Repeat("x", maxResponseBytes+1)), err: io.ErrUnexpectedEOF}
			client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				signalWroteRequest(request, nil)
				return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
			})}})

			_, err := client.PostForm(context.Background(), operation, url.Values{"token": {"secret-marker"}}, CallOptions{Timeout: time.Second})
			assertExitCode(t, err, output.ExitUpstreamRejection)
			assertNoSecret(t, err)
			if !body.closed.Load() {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestSlowResponseBodyHonorsPerCallTimeout(t *testing.T) {
	body := &blockingBody{}
	client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		signalWroteRequest(request, nil)
		body.ctx = request.Context()
		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	})}})

	_, err := client.PostForm(context.Background(), ParList, nil, CallOptions{Timeout: 10 * time.Millisecond})
	assertExitCode(t, err, output.ExitReadNetwork)
	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

func operationName(t *testing.T, operation Operation) string {
	t.Helper()
	descriptor, err := operation.Descriptor()
	if err != nil {
		t.Fatalf("operation descriptor: %v", err)
	}
	return descriptor.Name()
}

type traceAwareRoundTripFunc func(*http.Request) (*http.Response, error)

func (function traceAwareRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
func (traceAwareRoundTripFunc) upstreamTraceAware() {}

type errorBody struct {
	err    error
	closed atomic.Bool
}

func (body *errorBody) Read([]byte) (int, error) { return 0, body.err }
func (body *errorBody) Close() error {
	body.closed.Store(true)
	return nil
}

type blockingBody struct {
	ctx    context.Context
	closed atomic.Bool
}

func (body *blockingBody) Read([]byte) (int, error) {
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}
func (body *blockingBody) Close() error {
	body.closed.Store(true)
	return nil
}

type bytesThenErrorBody struct {
	content []byte
	err     error
	closed  atomic.Bool
}

func (body *bytesThenErrorBody) Read(buffer []byte) (int, error) {
	if len(body.content) == 0 {
		return 0, body.err
	}
	n := copy(buffer, body.content)
	body.content = body.content[n:]
	if len(body.content) == 0 {
		return n, body.err
	}
	return n, nil
}

func (body *bytesThenErrorBody) Close() error {
	body.closed.Store(true)
	return nil
}

func signalWroteRequest(request *http.Request, err error) {
	trace := httptrace.ContextClientTrace(request.Context())
	if trace != nil && trace.WroteRequest != nil {
		trace.WroteRequest(httptrace.WroteRequestInfo{Err: err})
	}
}

func mustClient(t *testing.T, config Config) *Client {
	t.Helper()
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if got := output.ExitCodeFor(err); got != want {
		t.Fatalf("exit code = %d, want %d; error=%v", got, want, err)
	}
}

func assertNoSecret(t *testing.T, err error) {
	t.Helper()
	formatted := fmt.Sprintf("%v %#v %+v", err, err, err)
	if strings.Contains(formatted, "secret-marker") || strings.Contains(formatted, "123456789012") || strings.Contains(formatted, "token=") {
		t.Fatalf("error formatting leaked secret: %s", formatted)
	}
}
