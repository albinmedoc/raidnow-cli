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
		Endpoint:          server.URL + "/form?existing=1",
		HTTPClient:        server.Client(),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	values := url.Values{
		"field[]": {"first", "second"},
		"token":   {"secret-marker"},
	}
	result, err := client.PostForm(context.Background(), LineMatchingGetList, values, CallOptions{Timeout: time.Second})
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
	if gotBody != "field%5B%5D=first&field%5B%5D=second&token=secret-marker&type=line_matching_get_list" {
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

func TestDefaultEndpointIsPinnedHTTPS(t *testing.T) {
	const want = "https://pokemongo-get.com/wp-content/themes/simplicity2-child/include/gofriend_enraid_masta.php?v=2025121601"
	if DefaultEndpoint != want {
		t.Fatalf("default endpoint = %q, want %q", DefaultEndpoint, want)
	}
}

func TestPostFormRejectsInvalidLocalArgumentsBeforeDispatch(t *testing.T) {
	var requests atomic.Int32
	client := mustClient(t, Config{HTTPClient: &http.Client{Transport: traceAwareRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, fmt.Errorf("must not dispatch")
	})}})
	testCases := []struct {
		name    string
		ctx     context.Context
		values  url.Values
		options CallOptions
	}{
		{name: "nil context"},
		{name: "caller type", ctx: context.Background(), values: url.Values{"type": {"override"}}},
		{name: "negative timeout", ctx: context.Background(), options: CallOptions{Timeout: -time.Second}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, callErr := client.PostForm(testCase.ctx, LineMatchingGetList, testCase.values, testCase.options)
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

func TestNewClientValidatesEndpointSafety(t *testing.T) {
	testCases := []struct {
		name      string
		endpoint  string
		allowHTTP bool
	}{
		{name: "empty host", endpoint: "https://"},
		{name: "malformed", endpoint: "https://example.com/%zz"},
		{name: "unsupported scheme", endpoint: "ftp://example.com/form"},
		{name: "userinfo", endpoint: "https://user:secret@example.com/form"},
		{name: "fragment", endpoint: "https://example.com/form#fragment"},
		{name: "relative", endpoint: "/form"},
		{name: "http without opt in", endpoint: "http://example.com/form?token=secret-marker"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewClient(Config{Endpoint: testCase.endpoint, AllowInsecureHTTP: testCase.allowHTTP})
			if got := output.ExitCodeFor(err); got != output.ExitInvalidArguments {
				t.Fatalf("exit code = %d, want %d; error=%v", got, output.ExitInvalidArguments, err)
			}
			formatted := fmt.Sprintf("%v %#v", err, err)
			if strings.Contains(formatted, testCase.endpoint) || strings.Contains(formatted, "secret") {
				t.Fatalf("error formatting leaked endpoint: %s", formatted)
			}
		})
	}
}

func TestNewClientRejectsInvalidEndpointPorts(t *testing.T) {
	testCases := []struct {
		name     string
		endpoint string
	}{
		{name: "zero", endpoint: "https://example.com:0/form"},
		{name: "above maximum", endpoint: "https://example.com:65536/form"},
		{name: "far above maximum", endpoint: "https://example.com:99999/form"},
		{name: "non numeric", endpoint: "https://example.com:not-a-port/form"},
		{name: "empty explicit port", endpoint: "https://example.com:/form"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewClient(Config{Endpoint: testCase.endpoint})
			if got := output.ExitCodeFor(err); got != output.ExitInvalidArguments {
				t.Fatalf("exit code = %d, want %d; error=%v", got, output.ExitInvalidArguments, err)
			}
			formatted := fmt.Sprintf("%v %#v", err, err)
			if strings.Contains(formatted, testCase.endpoint) || strings.Contains(formatted, "example.com") {
				t.Fatalf("error formatting leaked endpoint: %s", formatted)
			}
		})
	}
}

func TestNewClientAcceptsValidEndpointPorts(t *testing.T) {
	testCases := []struct {
		name     string
		endpoint string
	}{
		{name: "default endpoint", endpoint: ""},
		{name: "absent port", endpoint: "https://example.com/form"},
		{name: "minimum port", endpoint: "https://example.com:1/form"},
		{name: "HTTPS port", endpoint: "https://example.com:443/form"},
		{name: "maximum port", endpoint: "https://example.com:65535/form"},
		{name: "IPv4", endpoint: "https://192.0.2.1:443/form"},
		{name: "bracketed IPv6", endpoint: "https://[2001:db8::1]:443/form"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewClient(Config{Endpoint: testCase.endpoint}); err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
		})
	}
}

func TestNewClientAllowsHTTPOnlyWithExplicitOptIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.PostForm(context.Background(), ParList, nil, CallOptions{}); err != nil {
		t.Fatalf("post form: %v", err)
	}
}

func TestNewClientRejectsArbitraryCustomRoundTripper(t *testing.T) {
	_, err := NewClient(Config{HTTPClient: &http.Client{Transport: arbitraryRoundTripper{}}})
	if got := output.ExitCodeFor(err); got != output.ExitInvalidArguments {
		t.Fatalf("exit code = %d, want %d; error=%v", got, output.ExitInvalidArguments, err)
	}
	assertNoSecret(t, err)
}

func TestNewClientRejectsTypedNilStandardTransport(t *testing.T) {
	var transport *http.Transport
	_, err := NewClient(Config{HTTPClient: &http.Client{Transport: transport}})
	if got := output.ExitCodeFor(err); got != output.ExitInvalidArguments {
		t.Fatalf("exit code = %d, want %d; error=%v", got, output.ExitInvalidArguments, err)
	}
}

type arbitraryRoundTripper struct{}

func (arbitraryRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("secret-marker")
}
