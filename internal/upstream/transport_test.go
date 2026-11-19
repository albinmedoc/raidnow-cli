package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
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

	client := mustClient(t, Config{Endpoint: source.URL, HTTPClient: source.Client()})
	_, err := client.PostForm(context.Background(), "lookup", nil, validReadPolicy())
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
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: httpClient})

	if _, err := client.PostForm(context.Background(), "lookup", nil, validReadPolicy()); err != nil {
		t.Fatalf("post form: %v", err)
	}
}

func TestCanceledWriteBeforeDispatchIsSafeReadNetworkFailure(t *testing.T) {
	var requests atomic.Int32
	client := mustClient(t, Config{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("secret-marker")
	})}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.PostForm(ctx, "write_operation", nil, OperationPolicy{Kind: WriteOperation, Timeout: time.Second})
	assertExitCode(t, err, output.ExitReadNetwork)
	assertNoSecret(t, err)
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestTransportFailureClassificationAfterDispatch(t *testing.T) {
	testCases := []struct {
		name string
		kind OperationKind
		want int
	}{
		{name: "read", kind: ReadOperation, want: output.ExitReadNetwork},
		{name: "write", kind: WriteOperation, want: output.ExitWriteOutcomeUnknown},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var requests atomic.Int32
			client := mustClient(t, Config{HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests.Add(1)
				<-request.Context().Done()
				return nil, errors.New("disconnect secret-marker")
			})}})

			_, err := client.PostForm(context.Background(), "operation", nil, OperationPolicy{Kind: testCase.kind, Timeout: 10 * time.Millisecond})
			assertExitCode(t, err, testCase.want)
			assertNoSecret(t, err)
			if got := requests.Load(); got != 1 {
				t.Fatalf("requests = %d, want 1", got)
			}
		})
	}
}

func TestCallerCancellationDuringResponseUsesOperationClassification(t *testing.T) {
	testCases := []struct {
		name string
		kind OperationKind
		want int
	}{
		{name: "read", kind: ReadOperation, want: output.ExitReadNetwork},
		{name: "write", kind: WriteOperation, want: output.ExitWriteOutcomeUnknown},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			started := make(chan struct{})
			client := mustClient(t, Config{HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				close(started)
				<-request.Context().Done()
				return nil, request.Context().Err()
			})}})
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				_, err := client.PostForm(ctx, "operation", nil, OperationPolicy{Kind: testCase.kind, Timeout: time.Second})
				done <- err
			}()
			<-started
			cancel()
			assertExitCode(t, <-done, testCase.want)
		})
	}
}

func TestUnexpectedEOFAfterDispatchUsesOperationClassification(t *testing.T) {
	testCases := []struct {
		name string
		kind OperationKind
		want int
	}{
		{name: "read", kind: ReadOperation, want: output.ExitReadNetwork},
		{name: "write", kind: WriteOperation, want: output.ExitWriteOutcomeUnknown},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := &errorBody{err: io.ErrUnexpectedEOF}
			client := mustClient(t, Config{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
			})}})
			_, err := client.PostForm(context.Background(), "operation", nil, OperationPolicy{Kind: testCase.kind, Timeout: time.Second})
			assertExitCode(t, err, testCase.want)
			assertNoSecret(t, err)
			if !body.closed.Load() {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestSocketEOFFromRoundTripperUsesOperationClassification(t *testing.T) {
	testCases := []struct {
		name string
		kind OperationKind
		want int
	}{
		{name: "read", kind: ReadOperation, want: output.ExitReadNetwork},
		{name: "write", kind: WriteOperation, want: output.ExitWriteOutcomeUnknown},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := mustClient(t, Config{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, io.EOF
			})}})
			_, err := client.PostForm(context.Background(), "operation", url.Values{"token": {"secret-marker"}}, OperationPolicy{Kind: testCase.kind, Timeout: time.Second})
			assertExitCode(t, err, testCase.want)
			assertNoSecret(t, err)
		})
	}
}

func TestSlowResponseBodyHonorsPerCallTimeout(t *testing.T) {
	body := &blockingBody{}
	client := mustClient(t, Config{HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body.ctx = request.Context()
		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	})}})

	_, err := client.PostForm(context.Background(), "lookup", nil, OperationPolicy{Kind: ReadOperation, Timeout: 10 * time.Millisecond})
	assertExitCode(t, err, output.ExitReadNetwork)
	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

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
