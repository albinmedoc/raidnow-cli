package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestPostFormSendsStatelessBrowserLikeRequest(t *testing.T) {
	var gotRequest *http.Request
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotRequest = request.Clone(request.Context())
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		gotBody = string(body)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint:   server.URL + "/form?existing=1",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	values := url.Values{
		"field[]": {"first", "second"},
		"token":   {"secret-marker"},
	}
	result, err := client.PostForm(context.Background(), "lookup_operation", values, OperationPolicy{
		Kind:    ReadOperation,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("post form: %v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Fatalf("result = %q", result)
	}
	if gotRequest == nil {
		t.Fatal("server received no request")
	}
	if gotRequest.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotRequest.Method)
	}
	if gotRequest.URL.Path != "/form" || gotRequest.URL.RawQuery != "existing=1" {
		t.Fatalf("request URL = %q, want path and endpoint query preserved", gotRequest.URL.String())
	}
	if got := gotRequest.Header.Get("Origin"); got != "https://raidnow.leekduck.com" {
		t.Fatalf("Origin = %q", got)
	}
	if got := gotRequest.Header.Get("Referer"); got != "https://raidnow.leekduck.com/" {
		t.Fatalf("Referer = %q", got)
	}
	if got := gotRequest.Header.Get("Accept"); got != "application/json, text/javascript, */*; q=0.01" {
		t.Fatalf("Accept = %q", got)
	}
	if got := gotRequest.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded; charset=UTF-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := gotRequest.Header.Get("User-Agent"); !strings.HasPrefix(got, "Mozilla/") {
		t.Fatalf("User-Agent = %q, want browser-like value", got)
	}
	if gotBody != "field%5B%5D=first&field%5B%5D=second&token=secret-marker&type=lookup_operation" {
		t.Fatalf("body = %q", gotBody)
	}
	if gotRequest.Header.Get("Cookie") != "" {
		t.Fatalf("Cookie = %q, want none", gotRequest.Header.Get("Cookie"))
	}
	if gotRequest.Header.Get("X-CSRF-Token") != "" {
		t.Fatalf("CSRF header = %q, want none", gotRequest.Header.Get("X-CSRF-Token"))
	}
	if values.Get("type") != "" || len(values["field[]"]) != 2 {
		t.Fatalf("caller values mutated: %#v", values)
	}
}

func TestDefaultEndpointIsPinned(t *testing.T) {
	const want = "https://pokemongo-get.com/wp-content/themes/simplicity2-child/include/gofriend_enraid_masta.php?v=2025121601"
	if DefaultEndpoint != want {
		t.Fatalf("default endpoint = %q, want %q", DefaultEndpoint, want)
	}
}

func TestPostFormRejectsInvalidLocalArgumentsBeforeDispatch(t *testing.T) {
	var requests atomic.Int32
	client, err := NewClient(Config{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, fmt.Errorf("must not dispatch")
	})}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	testCases := []struct {
		name      string
		operation string
		values    url.Values
		policy    OperationPolicy
	}{
		{name: "blank operation", operation: " ", policy: validReadPolicy()},
		{name: "invalid operation", operation: "bad operation", policy: validReadPolicy()},
		{name: "caller type", operation: "lookup", values: url.Values{"type": {"override"}}, policy: validReadPolicy()},
		{name: "missing kind", operation: "lookup", policy: OperationPolicy{Timeout: time.Second}},
		{name: "invalid kind", operation: "lookup", policy: OperationPolicy{Kind: OperationKind(99), Timeout: time.Second}},
		{name: "missing timeout", operation: "lookup", policy: OperationPolicy{Kind: ReadOperation}},
		{name: "negative timeout", operation: "lookup", policy: OperationPolicy{Kind: ReadOperation, Timeout: -time.Second}},
		{name: "empty option wrong operation", operation: "lookup", policy: OperationPolicy{Kind: ReadOperation, Timeout: time.Second, AllowEmptyResponse: true}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, callErr := client.PostForm(context.Background(), testCase.operation, testCase.values, testCase.policy)
			if got := output.ExitCodeFor(callErr); got != output.ExitInvalidArguments {
				t.Fatalf("exit code = %d, want %d; error=%v", got, output.ExitInvalidArguments, callErr)
			}
			if strings.Contains(fmt.Sprintf("%v %#v", callErr, callErr), "override") {
				t.Fatalf("error formatting leaked form value: %v %#v", callErr, callErr)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestNewClientRejectsInvalidEndpoint(t *testing.T) {
	testCases := []string{"http://", "ftp://example.com/form", "https://user:secret@example.com/form", "https://example.com/form#fragment"}
	for _, endpoint := range testCases {
		t.Run(endpoint, func(t *testing.T) {
			_, err := NewClient(Config{Endpoint: endpoint})
			if got := output.ExitCodeFor(err); got != output.ExitInvalidArguments {
				t.Fatalf("exit code = %d, want %d; error=%v", got, output.ExitInvalidArguments, err)
			}
			if strings.Contains(fmt.Sprintf("%v %#v", err, err), endpoint) || strings.Contains(fmt.Sprintf("%v %#v", err, err), "secret") {
				t.Fatalf("error formatting leaked endpoint: %v %#v", err, err)
			}
		})
	}
}

func validReadPolicy() OperationPolicy {
	return OperationPolicy{Kind: ReadOperation, Timeout: time.Second}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
