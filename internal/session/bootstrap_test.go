package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

const observedToken = "AbCdEf0123456789_-AbCdEf0123456789_-AbCdEfGH"

func TestBootstrapTokenExtractsServerRenderedToken(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "bootstrap_single_quote.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = response.Write(fixture)
	}))
	t.Cleanup(server.Close)

	bootstrap := NewBootstrap(server.Client(), server.URL)
	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if got, want := token, observedToken; got != want {
		t.Fatalf("token = %q, want %q", got, want)
	}
}

func TestBootstrapTokenExtractsDoubleQuotedEscapedFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "bootstrap_double_quote.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write(fixture)
	}))
	t.Cleanup(server.Close)

	bootstrap := NewBootstrap(server.Client(), server.URL)
	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if got, want := token, "DoubleQuoteToken-With1Escapes"; got != want {
		t.Fatalf("token = %q, want %q", got, want)
	}
}

func TestBootstrapTokenSendsBrowserLikeReadOnlyRequestWithoutSessionState(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", request.Method)
		}
		if userAgent := request.Header.Get("User-Agent"); !strings.Contains(userAgent, "Mozilla/") {
			t.Errorf("User-Agent = %q, want browser-like value", userAgent)
		}
		if accept := request.Header.Get("Accept"); !strings.Contains(accept, "text/html") {
			t.Errorf("Accept = %q, want HTML", accept)
		}
		if cookie := request.Header.Get("Cookie"); cookie != "" {
			t.Errorf("Cookie = %q, want none", cookie)
		}
		if csrf := request.Header.Get("X-CSRF-TOKEN"); csrf != "" {
			t.Errorf("X-CSRF-TOKEN = %q, want none", csrf)
		}
		response.Header().Set("Set-Cookie", "session=must-not-be-retained; Path=/")
		_, _ = response.Write([]byte(`{gofriend_token:'` + observedToken + `'}`))
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{Name: "existing", Value: "must-not-be-sent"}})
	client := server.Client()
	client.Jar = jar

	bootstrap := NewBootstrap(client, server.URL)
	if _, err := bootstrap.Token(context.Background()); err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
	if cookies := jar.Cookies(serverURL); len(cookies) != 1 || cookies[0].Name != "existing" {
		t.Fatalf("cookie jar changed: %#v", cookies)
	}
}

func TestBootstrapTokenMatchesTheExactPropertyName(t *testing.T) {
	server := bootstrapServer(t, http.StatusOK, `data: {
		$gofriend_token: 'dollar-prefixed-secret',
		mygofriend_token: 'letter-prefixed-secret',
		_gofriend_token: 'underscore-prefixed-secret',
		gofriend_token_suffix: 'suffixed-secret',
		"gofriend_token": "right-token"
	}`)
	bootstrap := NewBootstrap(server.Client(), server.URL)

	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if got, want := token, "right-token"; got != want {
		t.Fatalf("token = %q, want exact property value %q", got, want)
	}
}

func TestBootstrapTokenDecodesHexEscapesAsJavaScriptCodePoints(t *testing.T) {
	server := bootstrapServer(t, http.StatusOK, `data: {gofriend_token: '\x41\xE9'}`)
	bootstrap := NewBootstrap(server.Client(), server.URL)

	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap hex-escaped token: %v", err)
	}
	if got, want := token, "Aé"; got != want {
		t.Fatalf("decoded token = %q, want %q", got, want)
	}
	if !utf8.ValidString(token) {
		t.Fatalf("decoded token %q is not valid UTF-8", token)
	}
}

func TestBootstrapTokenDecodesOrdinaryJavaScriptEscapes(t *testing.T) {
	server := bootstrapServer(t, http.StatusOK, "data: { gofriend_token : 'opaque\\npart' }")
	bootstrap := NewBootstrap(server.Client(), server.URL)

	_, err := bootstrap.Token(context.Background())
	if err == nil {
		t.Fatal("bootstrap token with decoded newline: got nil error, want single-line rejection")
	}

	server.Config.Handler = http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte("data: {gofriend_token: 'opaque\\x2Dvalue\\u0031\\/end'}"))
	})
	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap escaped token: %v", err)
	}
	if got, want := token, "opaque-value1/end"; got != want {
		t.Fatalf("decoded token = %q, want %q", got, want)
	}
}

func TestBootstrapTokenRejectsUnsafeOrInvalidResponsesWithoutLeakingContent(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "non_2xx", status: http.StatusBadGateway, body: `<html>SECRET-UPSTREAM-BODY</html>`},
		{name: "missing", status: http.StatusOK, body: `<html>SECRET-MISSING-BODY</html>`},
		{name: "empty", status: http.StatusOK, body: `gofriend_token: ''`},
		{name: "malformed_escape", status: http.StatusOK, body: `gofriend_token: 'SECRET\xZZ'`},
		{name: "newline", status: http.StatusOK, body: `gofriend_token: 'SECRET\nTOKEN'`},
		{name: "nul", status: http.StatusOK, body: `gofriend_token: 'SECRET\0TOKEN'`},
		{name: "too_long_token", status: http.StatusOK, body: `gofriend_token: '` + strings.Repeat("S", MaxTokenBytes+1) + `'`},
		{name: "oversized_body", status: http.StatusOK, body: strings.Repeat("H", MaxBootstrapBodyBytes+1)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := bootstrapServer(t, testCase.status, testCase.body)
			bootstrap := NewBootstrap(server.Client(), server.URL)
			_, err := bootstrap.Token(context.Background())
			if err == nil {
				t.Fatal("bootstrap token: got nil error, want rejection")
			}
			for _, forbidden := range []string{"SECRET", "<html>", strings.Repeat("H", 32), strings.Repeat("S", 32)} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("error %q leaks response content %q", err, forbidden)
				}
			}
		})
	}
}

func TestBootstrapTokenUsesBoundedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	bootstrap := NewBootstrap(server.Client(), server.URL)
	bootstrap.timeout = 20 * time.Millisecond

	started := time.Now()
	_, err := bootstrap.Token(context.Background())
	if err == nil {
		t.Fatal("bootstrap token: got nil error, want timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bootstrap timeout took %s, want bounded request", elapsed)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("error exposes internal timeout cause")
	}
}

func bootstrapServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(status)
		_, _ = response.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}
