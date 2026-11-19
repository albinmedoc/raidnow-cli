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
		_, _ = response.Write([]byte(`<script>const app={gofriend_token:'` + observedToken + `'}</script>`))
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

func TestBootstrapTokenIgnoresDecoysOutsideExecutableScriptCode(t *testing.T) {
	server := bootstrapServer(t, http.StatusOK, `
		gofriend_token: 'non-script-decoy'
		<!-- <script>gofriend_token: 'html-comment-decoy'</script> -->
		<script>
			// gofriend_token: 'line-comment-decoy'
			/* gofriend_token: 'block-comment-decoy' */
			const quoted = "gofriend_token: 'double-string-decoy'";
			const single = 'gofriend_token: \'single-string-decoy\'';
			const template = `+"`gofriend_token: 'template-decoy'`"+`;
		</script>
		<script>const app = {gofriend_token: 'right-token'};</script>`)
	bootstrap := NewBootstrap(server.Client(), server.URL)

	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if got, want := token, "right-token"; got != want {
		t.Fatalf("token = %q, want executable script property %q", got, want)
	}
}

func TestBootstrapTokenMatchesTheExactPropertyName(t *testing.T) {
	server := bootstrapServer(t, http.StatusOK, `<script>data: {
		$gofriend_token: 'dollar-prefixed-secret',
		mygofriend_token: 'letter-prefixed-secret',
		_gofriend_token: 'underscore-prefixed-secret',
		gofriend_token_suffix: 'suffixed-secret',
		"gofriend_token": "right-token"
	}</script>`)
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
	server := bootstrapServer(t, http.StatusOK, `<script>data: {gofriend_token: '\x41\xE9'}</script>`)
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
	server := bootstrapServer(t, http.StatusOK, "<script>data: { gofriend_token : 'opaque\\npart' }</script>")
	bootstrap := NewBootstrap(server.Client(), server.URL)

	_, err := bootstrap.Token(context.Background())
	if err == nil {
		t.Fatal("bootstrap token with decoded newline: got nil error, want single-line rejection")
	}

	server.Config.Handler = http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte("<script>data: {gofriend_token: 'opaque\\x2Dvalue\\u0031\\/end'}</script>"))
	})
	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap escaped token: %v", err)
	}
	if got, want := token, "opaque-value1/end"; got != want {
		t.Fatalf("decoded token = %q, want %q", got, want)
	}
}

func TestBootstrapTokenRejectsInvalidUTF8AndUnicodeControls(t *testing.T) {
	testCases := []struct {
		name  string
		value []byte
	}{
		{name: "invalid_utf8", value: []byte{0xff}},
		{name: "raw_tab", value: []byte("part	part")},
		{name: "raw_bel", value: []byte("part\a part")},
		{name: "raw_escape", value: []byte("part\x1bpart")},
		{name: "raw_delete", value: []byte("part\x7fpart")},
		{name: "raw_c1_control", value: []byte("part\u0085part")},
		{name: "escaped_tab", value: []byte(`part	part`)},
		{name: "escaped_escape", value: []byte(`part\x1bpart`)},
		{name: "escaped_delete", value: []byte(`part\x7fpart`)},
		{name: "escaped_c1_control", value: []byte(`part\u0085part`)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := append([]byte("<script>const app={gofriend_token:'"), testCase.value...)
			body = append(body, []byte("'};</script>")...)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				_, _ = response.Write(body)
			}))
			t.Cleanup(server.Close)

			token, err := NewBootstrap(server.Client(), server.URL).Token(context.Background())
			if err == nil {
				t.Fatalf("bootstrap token = %q, want rejection", token)
			}
			if token != "" {
				t.Fatalf("rejected bootstrap emitted token %q", token)
			}
		})
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
		{name: "empty", status: http.StatusOK, body: `<script>gofriend_token: ''</script>`},
		{name: "malformed_escape", status: http.StatusOK, body: `<script>gofriend_token: 'SECRET\xZZ'</script>`},
		{name: "newline", status: http.StatusOK, body: `<script>gofriend_token: 'SECRET\nTOKEN'</script>`},
		{name: "nul", status: http.StatusOK, body: `<script>gofriend_token: 'SECRET\0TOKEN'</script>`},
		{name: "too_long_token", status: http.StatusOK, body: `<script>gofriend_token: '` + strings.Repeat("S", MaxTokenBytes+1) + `'</script>`},
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
