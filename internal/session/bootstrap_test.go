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
		<script>new Vue({data:{gofriend_token: 'right-token'}});</script>`)
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
	server := bootstrapServer(t, http.StatusOK, `<script>new Vue({data: {
		$gofriend_token: 'dollar-prefixed-secret',
		mygofriend_token: 'letter-prefixed-secret',
		_gofriend_token: 'underscore-prefixed-secret',
		gofriend_token_suffix: 'suffixed-secret',
		"gofriend_token": "right-token"
	}})</script>`)
	bootstrap := NewBootstrap(server.Client(), server.URL)

	token, err := bootstrap.Token(context.Background())
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	if got, want := token, "right-token"; got != want {
		t.Fatalf("token = %q, want exact property value %q", got, want)
	}
}

func TestBootstrapTokenRequiresExactVueDataShape(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "bare_property", body: `<script>gofriend_token: 'wrong-token'</script>`},
		{name: "plain_object", body: `<script>({data:{gofriend_token:'wrong-token'}})</script>`},
		{name: "wrong_constructor", body: `<script>new NotVue({data:{gofriend_token:'wrong-token'}})</script>`},
		{name: "nested_data", body: `<script>new Vue({options:{data:{gofriend_token:'wrong-token'}}})</script>`},
		{name: "data_function", body: `<script>new Vue({data(){return {gofriend_token:'wrong-token'}}})</script>`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := bootstrapServer(t, http.StatusOK, testCase.body)
			token, err := NewBootstrap(server.Client(), server.URL).Token(context.Background())
			if err == nil || token != "" {
				t.Fatalf("bootstrap token = %q, err=%v; want sanitized rejection", token, err)
			}
		})
	}
}

func TestBootstrapTokenDecodesHexEscapesAsJavaScriptCodePoints(t *testing.T) {
	server := bootstrapServer(t, http.StatusOK, `<script>new Vue({data: {gofriend_token: '\x41\xE9'}})</script>`)
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
	server := bootstrapServer(t, http.StatusOK, "<script>new Vue({data: { gofriend_token : 'opaque\\npart' }})</script>")
	bootstrap := NewBootstrap(server.Client(), server.URL)

	_, err := bootstrap.Token(context.Background())
	if err == nil {
		t.Fatal("bootstrap token with decoded newline: got nil error, want single-line rejection")
	}

	server.Config.Handler = http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte("<script>new Vue({data: {gofriend_token: 'opaque\\x2Dvalue\\u0031\\/end'}})</script>"))
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
		{name: "escaped_line_separator", value: []byte(`part\u2028part`)},
		{name: "escaped_paragraph_separator", value: []byte(`part\u2029part`)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := append([]byte("<script>new Vue({data:{gofriend_token:'"), testCase.value...)
			body = append(body, []byte("'}});</script>")...)
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
		{name: "empty", status: http.StatusOK, body: `<script>new Vue({data:{gofriend_token:''}})</script>`},
		{name: "malformed_escape", status: http.StatusOK, body: `<script>new Vue({data:{gofriend_token:'SECRET\xZZ'}})</script>`},
		{name: "newline", status: http.StatusOK, body: `<script>new Vue({data:{gofriend_token:'SECRET\nTOKEN'}})</script>`},
		{name: "nul", status: http.StatusOK, body: `<script>new Vue({data:{gofriend_token:'SECRET\0TOKEN'}})</script>`},
		{name: "too_long_token", status: http.StatusOK, body: `<script>new Vue({data:{gofriend_token:'` + strings.Repeat("S", MaxTokenBytes+1) + `'}})</script>`},
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

func TestExtractTokenVisitsOnlyExecutableInlineScriptsOutsideTemplates(t *testing.T) {
	valid := `new Vue({data:{gofriend_token:'right-token'}})`
	testCases := []struct {
		name string
		body string
	}{
		{name: "non_script_text", body: valid},
		{name: "html_comment", body: `<!-- <script>` + valid + `</script> -->`},
		{name: "space_after_open_angle", body: `< script>` + valid + `</script>`},
		{name: "script_prefix", body: `<scriptx>` + valid + `</scriptx>`},
		{name: "template_ancestor", body: `<template><div><script>` + valid + `</script></div></template>`},
		{name: "mathml_namespace", body: `<math><script>` + valid + `</script></math>`},
		{name: "svg_namespace", body: `<svg><script>` + valid + `</script></svg>`},
		{name: "src_attribute", body: `<script src="app.js">` + valid + `</script>`},
		{name: "href_attribute", body: `<script href="app.js">` + valid + `</script>`},
		{name: "svg_namespaced_href_attribute", body: `<svg><script xlink:href="app.js">` + valid + `</script></svg>`},
		{name: "non_javascript_type", body: `<script type="application/json">` + valid + `</script>`},
		{name: "malformed_javascript_type", body: `<script type="text/javascript; charset">` + valid + `</script>`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertBootstrapRejected(t, []byte(testCase.body))
		})
	}

	executableTypes := []struct {
		name      string
		attribute string
	}{
		{name: "absent", attribute: ""},
		{name: "empty", attribute: ` type=""`},
		{name: "module", attribute: ` type="module"`},
		{name: "text_javascript", attribute: ` type="text/javascript"`},
		{name: "parameterized_text_javascript", attribute: ` type="text/javascript; charset=utf-8"`},
		{name: "application_javascript", attribute: ` type="application/javascript"`},
		{name: "text_ecmascript", attribute: ` type="text/ecmascript"`},
		{name: "application_ecmascript", attribute: ` type="application/ecmascript"`},
		{name: "case_insensitive", attribute: ` type="Text/JavaScript; Charset=UTF-8"`},
	}
	for _, testCase := range executableTypes {
		t.Run("executable_"+testCase.name, func(t *testing.T) {
			token, err := extractToken([]byte(`<script` + testCase.attribute + `>` + valid + `</script>`))
			if err != nil || token != "right-token" {
				t.Fatalf("token=%q err=%v, want executable inline script", token, err)
			}
		})
	}
}

func TestExtractTokenIgnoresJavaScriptSyntaxDecoys(t *testing.T) {
	body := `<script>
		// new Vue({data:{gofriend_token:'line-comment'}})
		/* new Vue({data:{gofriend_token:'block-comment'}}) */
		<!-- new Vue({data:{gofriend_token:'legacy-html-open-comment'}})
		--> new Vue({data:{gofriend_token:'legacy-html-close-comment'}})
		const a = "new Vue({data:{gofriend_token:'string'}})";
		const b = ` + "`new Vue({data:{gofriend_token:'template'}})`" + `;
		const c = /new Vue\(\{data:\{gofriend_token:'regex'\}\}\)/;
		gofriend_token: 'label';
		new Vue({data:{gofriend_token:'right-token'}});
	</script>`
	token, err := extractToken([]byte(body))
	if err != nil || token != "right-token" {
		t.Fatalf("token=%q err=%v, want only structural Vue candidate", token, err)
	}
}

func TestExtractTokenRejectsAmbiguousOrDynamicVueObjects(t *testing.T) {
	testCases := []struct {
		name   string
		script string
	}{
		{name: "computed_data", script: `new Vue({['data']:{gofriend_token:'x'}})`},
		{name: "computed_token", script: `new Vue({data:{['gofriend_token']:'x'}})`},
		{name: "config_shorthand", script: `new Vue({data})`},
		{name: "data_shorthand", script: `new Vue({data:{gofriend_token}})`},
		{name: "config_getter", script: `new Vue({get data(){return {gofriend_token:'x'}}})`},
		{name: "unrelated_getter", script: `new Vue({data:{gofriend_token:'x'},get mounted(){return true}})`},
		{name: "unrelated_setter", script: `new Vue({data:{gofriend_token:'x'},set mounted(value){}})`},
		{name: "config_method", script: `new Vue({data(){return {gofriend_token:'x'}}})`},
		{name: "computed_lifecycle_method", script: `new Vue({data:{gofriend_token:'x'},['mounted'](){}})`},
		{name: "escaped_lifecycle_method", script: `new Vue({data:{gofriend_token:'x'},m\u006funted(){}})`},
		{name: "escaped_quoted_lifecycle_method", script: `new Vue({data:{gofriend_token:'x'},'m\u006funted'(){}})`},
		{name: "async_lifecycle_method", script: `new Vue({data:{gofriend_token:'x'},async mounted(){}})`},
		{name: "generator_lifecycle_method", script: `new Vue({data:{gofriend_token:'x'},*mounted(){}})`},
		{name: "data_unrelated_method", script: `new Vue({data:{gofriend_token:'x',mounted(){}}})`},
		{name: "token_getter", script: `new Vue({data:{get gofriend_token(){return 'x'}}})`},
		{name: "token_method", script: `new Vue({data:{gofriend_token(){return 'x'}}})`},
		{name: "config_spread", script: `new Vue({...config,data:{gofriend_token:'x'}})`},
		{name: "data_spread", script: `new Vue({data:{...values,gofriend_token:'x'}})`},
		{name: "duplicate_data", script: `new Vue({data:{gofriend_token:'x'},data:{gofriend_token:'y'}})`},
		{name: "duplicate_token", script: `new Vue({data:{gofriend_token:'x',gofriend_token:'y'}})`},
		{name: "extra_argument", script: `new Vue({data:{gofriend_token:'x'}}, other)`},
		{name: "spread_argument", script: `new Vue(...[{data:{gofriend_token:'x'}}])`},
		{name: "non_string", script: `new Vue({data:{gofriend_token:123}})`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertBootstrapRejected(t, []byte(`<script>`+testCase.script+`</script>`))
		})
	}
}

func TestExtractTokenRejectsEscapedIdentifierAmbiguity(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		script string
	}{
		{name: "mixed_data", script: `new Vue({data:{gofriend_token:'first'},d\u0061ta:{gofriend_token:'second'}})`},
		{name: "escaped_data_only", script: `new Vue({d\u0061ta:{gofriend_token:'secret'}})`},
		{name: "mixed_token", script: `new Vue({data:{gofriend_token:'first',gofriend_\u0074oken:'second'}})`},
		{name: "escaped_token_only", script: `new Vue({data:{gofriend_\u0074oken:'secret'}})`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assertBootstrapRejected(t, []byte(`<script>`+testCase.script+`</script>`))
		})
	}
}

func TestExtractTokenAllowsUnrelatedOrdinaryIdentifiers(t *testing.T) {
	body := []byte(`<script>new Vue({el:'#app',ordinaryConfig:true,data:{ordinaryData:'value',gofriend_token:'right-token'}})</script>`)
	token, err := extractToken(body)
	if err != nil || token != "right-token" {
		t.Fatalf("token=%q err=%v, want right-token", token, err)
	}
}

func TestExtractTokenAllowsUnrelatedOrdinaryVueMethods(t *testing.T) {
	body := []byte(`<script>new Vue({data:{gofriend_token:'fixture-token'}, mounted(){}})</script>`)
	token, err := extractToken(body)
	if err != nil || token != "fixture-token" {
		t.Fatalf("token=%q err=%v, want fixture-token", token, err)
	}
}

func TestExtractTokenRequiresExactlyOneCandidateAcrossDocument(t *testing.T) {
	for _, body := range []string{
		`<script>new Vue({data:{gofriend_token:'same'}});new Vue({data:{gofriend_token:'same'}})</script>`,
		`<script>new Vue({data:{gofriend_token:'one'}})</script><script>new Vue({data:{gofriend_token:'two'}})</script>`,
	} {
		assertBootstrapRejected(t, []byte(body))
	}
}

func TestExtractTokenDecodesSupportedStringEscapesWithoutEvaluation(t *testing.T) {
	testCases := []struct {
		name    string
		literal string
		want    string
	}{
		{name: "ordinary", literal: `'quote\'slash\/backslash\\'`, want: `quote'slash/backslash\`},
		{name: "hex", literal: `'\x41\xE9'`, want: "Aé"},
		{name: "unicode", literal: `'\u0041'`, want: "A"},
		{name: "surrogate_pair", literal: `'\uD83D\uDE80'`, want: "🚀"},
		{name: "code_point", literal: `'\u{1F680}'`, want: "🚀"},
		{name: "lf_continuation", literal: "'line\\\ncontinued'", want: "linecontinued"},
		{name: "crlf_continuation", literal: "'line\\\r\ncontinued'", want: "linecontinued"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := `<script>new Vue({data:{gofriend_token:` + testCase.literal + `}})</script>`
			token, err := extractToken([]byte(body))
			if err != nil || token != testCase.want {
				t.Fatalf("token=%q err=%v, want %q", token, err, testCase.want)
			}
		})
	}
}

func TestExtractTokenRejectsMalformedJavaScriptAndEscapes(t *testing.T) {
	for _, script := range []string{
		`new Vue({data:{gofriend_token:'unterminated'}}`,
		`new Vue({data:{gofriend_token:'\xZZ'}})`,
		`new Vue({data:{gofriend_token:'\uZZZZ'}})`,
		`new Vue({data:{gofriend_token:'\u{110000}'}})`,
		`new Vue({data:{gofriend_token:'\uD83D'}})`,
		`new Vue({data:{gofriend_token:'\uDE80'}})`,
		`new Vue({data:{gofriend_token:'\1'}})`,
		`new Vue({data:{gofriend_token:'\08'}})`,
		`new Vue({data:{gofriend_token:'\q'}})`,
	} {
		assertBootstrapRejected(t, []byte(`<script>`+script+`</script>`))
	}

	assertBootstrapRejected(t, []byte(`<script>const broken = ;</script><script>new Vue({data:{gofriend_token:'secret'}})</script>`))
}

func TestDecodeJSStringLiteralRejectsOversizedOrdinaryContent(t *testing.T) {
	literal := []byte(`'` + strings.Repeat("x", MaxTokenBytes+1) + `'`)
	if _, err := decodeJSStringLiteral(literal); err == nil {
		t.Fatal("decode oversized ordinary string: got nil error, want rejection")
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
