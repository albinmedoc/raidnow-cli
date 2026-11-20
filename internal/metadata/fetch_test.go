package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHomepageFetcherGetsAndParsesCatalogWithoutSessionState(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodGet {
			t.Errorf("method = %q", request.Method)
		}
		if request.Header.Get("Cookie") != "" {
			t.Error("request sent cookies")
		}
		response.Header().Set("Set-Cookie", "secret=retained; Path=/")
		_, _ = response.Write([]byte(`<script>const aryPokemon={"150":{"en":"Mewtwo"}};</script>`))
	}))
	t.Cleanup(server.Close)

	serverURL, _ := url.Parse(server.URL)
	jar.SetCookies(serverURL, []*http.Cookie{{Name: "existing", Value: "secret"}})
	client := server.Client()
	client.Jar = jar

	catalog, err := NewHomepageFetcher(client, server.URL).Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := catalog.Lookup("150")
	if !ok || entry.NameEN != "Mewtwo" || requests.Load() != 1 {
		t.Fatalf("entry=%#v ok=%v requests=%d", entry, ok, requests.Load())
	}
	if cookies := jar.Cookies(serverURL); len(cookies) != 1 || cookies[0].Name != "existing" {
		t.Fatalf("cookie jar changed: %#v", cookies)
	}
}

func TestHomepageFetcherRejectsRedirectStatusOversizeAndInvalidMetadata(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
	}{
		{name: "redirect", handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/next", http.StatusFound) })},
		{name: "status", handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("SECRET"))
		})},
		{name: "oversize", handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", MaxHTMLBytes+1)))
		})},
		{name: "parse", handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<script>const aryPokemon={"SECRET":`))
		})},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(testCase.handler)
			t.Cleanup(server.Close)
			_, err := NewHomepageFetcher(server.Client(), server.URL).Catalog(context.Background())
			if !errors.Is(err, ErrMetadataUnavailable) || err.Error() != ErrMetadataUnavailable.Error() || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestHomepageFetcherUsesBoundedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
	t.Cleanup(server.Close)
	fetcher := NewHomepageFetcher(server.Client(), server.URL)
	fetcher.timeout = 20 * time.Millisecond
	started := time.Now()
	_, err := fetcher.Catalog(context.Background())
	if !errors.Is(err, ErrMetadataUnavailable) || time.Since(started) > time.Second {
		t.Fatalf("err=%v elapsed=%s", err, time.Since(started))
	}
}
