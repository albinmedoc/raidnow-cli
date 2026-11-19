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
)

const observedToken = "AbCdEf0123456789_-AbCdEf0123456789_-AbCdEfGH"

func TestBootstrapTokenExtractsServerRenderedToken(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "bootstrap_single_quote.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	server := bootstrapServer(t, http.StatusOK, string(fixture))

	token, err := NewBootstrap(server.Client(), server.URL).Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if token != observedToken {
		t.Fatalf("token = %q, want %q", token, observedToken)
	}
}

func TestBootstrapTokenExtractsQuotedPropertyAndMinimalEscapes(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "bootstrap_double_quote.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	server := bootstrapServer(t, http.StatusOK, string(fixture))

	token, err := NewBootstrap(server.Client(), server.URL).Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if token != "DoubleQuoteToken-With1Escapes" {
		t.Fatalf("token = %q, want minimally unescaped token", token)
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
		if request.Header.Get("Cookie") != "" || request.Header.Get("X-CSRF-TOKEN") != "" {
			t.Error("request included session state")
		}
		response.Header().Set("Set-Cookie", "session=must-not-be-retained; Path=/")
		_, _ = response.Write([]byte(`<script>new Vue({data:{gofriend_token:'` + observedToken + `'}})</script>`))
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{Name: "existing", Value: "must-not-be-sent"}})
	client := server.Client()
	client.Jar = jar

	if _, err := NewBootstrap(client, server.URL).Token(context.Background()); err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requests.Load())
	}
	if cookies := jar.Cookies(serverURL); len(cookies) != 1 || cookies[0].Name != "existing" {
		t.Fatalf("cookie jar changed: %#v", cookies)
	}
}

func TestExtractTokenUsesExecutableInlineScriptsAndIgnoresCheapDecoys(t *testing.T) {
	body := []byte(`<html><body>
		gofriend_token: 'outside'
		<script src="external.js">gofriend_token:'external'</script>
		<script type="application/json">gofriend_token:'json'</script>
		<script>
			// gofriend_token: 'line-comment'
			/* gofriend_token: 'block-comment' */
			const quoted = "gofriend_token: 'string'";
			const locale = "雪";
			const longer = {mygofriend_token: 'longer'};
		</script>
		<script type="text/javascript; charset=utf-8">new Vue({data:{gofriend_token:'right-token'}})</script>
	</body></html>`)

	token, err := extractToken(body)
	if err != nil || token != "right-token" {
		t.Fatalf("extractToken() = %q, %v; want right-token", token, err)
	}
}

func TestExtractTokenRequiresExactlyOneValidCandidate(t *testing.T) {
	for _, body := range []string{
		`<script>const other = true</script>`,
		`<script>gofriend_token:'one'; gofriend_token:'two'</script>`,
		`<script>gofriend_token:123</script>`,
		`<script>gofriend_token:'bad\q'</script>`,
		`<script>gofriend_token:'bad
value'</script>`,
		`<script>gofriend_token:'` + strings.Repeat("x", MaxTokenBytes+1) + `'</script>`,
	} {
		assertBootstrapRejected(t, []byte(body))
	}
}

func TestBootstrapTokenRejectsUnsafeResponsesWithoutLeakingContent(t *testing.T) {
	for _, testCase := range []struct {
		status int
		body   string
	}{
		{status: http.StatusBadGateway, body: `<html>SECRET-UPSTREAM-BODY</html>`},
		{status: http.StatusOK, body: `<script>gofriend_token:'SECRET\nTOKEN'</script>`},
		{status: http.StatusOK, body: strings.Repeat("SECRET", MaxBootstrapBodyBytes)},
	} {
		server := bootstrapServer(t, testCase.status, testCase.body)
		_, err := NewBootstrap(server.Client(), server.URL).Token(context.Background())
		if err == nil {
			t.Fatal("bootstrap token: got nil error, want rejection")
		}
		if strings.Contains(err.Error(), "SECRET") || err.Error() != errBootstrap.Error() {
			t.Fatalf("error %q is not sanitized", err)
		}
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
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("bounded timeout error = %v, elapsed = %s", err, time.Since(started))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("error exposes internal timeout cause")
	}
}

func assertBootstrapRejected(t *testing.T, body []byte) {
	t.Helper()
	token, err := extractToken(body)
	if err == nil || token != "" {
		t.Fatalf("extract token=%q err=%v, want empty sanitized failure", token, err)
	}
	if err.Error() != errBootstrap.Error() {
		t.Fatalf("error=%q, want exact sanitized %q", err, errBootstrap)
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
