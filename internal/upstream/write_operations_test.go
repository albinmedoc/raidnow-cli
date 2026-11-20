package upstream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestPublishTrainerPostsExactNormalizedFormAndAcceptsAcknowledgements(t *testing.T) {
	responses := []string{"", `{"status":"success"}`}
	var forms []url.Values
	var headers []http.Header
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		forms = append(forms, request.PostForm)
		headers = append(headers, request.Header.Clone())
		_, _ = response.Write([]byte(responses[len(forms)-1]))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	trainer, err := domain.NewTrainer(" Ash123 ", "1234 5678\u30009012", 80, " WORLD ")
	if err != nil {
		t.Fatalf("new trainer: %v", err)
	}

	for range responses {
		if err := client.PublishTrainer(context.Background(), "opaque-token", trainer, CallOptions{}); err != nil {
			t.Fatalf("publish trainer: %v", err)
		}
	}
	want := url.Values{
		"type": {"user_info_post"}, "input_tid": {"Ash123"}, "input_level": {"80"},
		"input_code": {"123456789012"}, "input_country_code": {"world"}, "token": {"opaque-token"},
	}
	for _, form := range forms {
		if !reflect.DeepEqual(form, want) {
			t.Fatalf("form=%#v want=%#v", form, want)
		}
	}
	if got := headers[0].Get("Origin"); got != "https://raidnow.leekduck.com" {
		t.Fatalf("Origin=%q", got)
	}
}

func TestPublishTrainerAcceptsEmptyOnlyForHTTP200(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	assertExitCode(t, client.PublishTrainer(context.Background(), "token", trainer, CallOptions{}), output.ExitUpstreamRejection)
}

func TestPublishTrainerClassifiesStructuredResponses(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
		want int
	}{
		{name: "session", body: `{"status":"error","text":"The session has expired. secret-marker"}`, want: output.ExitAuthenticationUnsupported},
		{name: "rejected", body: `{"status":"error","text":"Lobby is full."}`, want: output.ExitUpstreamRejection},
		{name: "missing acknowledgement", body: `{"ok":true,"text":"secret-marker"}`, want: output.ExitUpstreamRejection},
		{name: "wrong success type", body: `{"status":true}`, want: output.ExitUpstreamRejection},
		{name: "array", body: `[]`, want: output.ExitUpstreamRejection},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(testCase.body)) }))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
			err := client.PublishTrainer(context.Background(), "opaque-token", trainer, CallOptions{})
			assertExitCode(t, err, testCase.want)
			assertNoSecret(t, err)
		})
	}
}

func TestPublishTrainerRejectsInvalidInputsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	valid, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	for _, testCase := range []struct {
		name    string
		ctx     context.Context
		token   string
		trainer domain.Trainer
		options CallOptions
	}{
		{name: "nil context", token: "token", trainer: valid},
		{name: "invalid token", ctx: context.Background(), token: "\n", trainer: valid},
		{name: "zero trainer", ctx: context.Background(), token: "token"},
		{name: "negative timeout", ctx: context.Background(), token: "token", trainer: valid, options: CallOptions{Timeout: -time.Second}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := client.PublishTrainer(testCase.ctx, testCase.token, testCase.trainer, testCase.options)
			assertExitCode(t, err, output.ExitInvalidArguments)
			assertNoSecret(t, err)
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("calls=%d want=0", calls.Load())
	}
}

func TestPublishTrainerHonorsTimeoutCancellationAndDoesNotRetry(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		response.WriteHeader(http.StatusOK)
		response.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	err := client.PublishTrainer(context.Background(), "token", trainer, CallOptions{Timeout: 10 * time.Millisecond})
	assertExitCode(t, err, output.ExitWriteOutcomeUnknown)
	if calls.Load() != 1 {
		t.Fatalf("calls=%d want=1", calls.Load())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.PublishTrainer(ctx, "token", trainer, CallOptions{})
	assertExitCode(t, err, output.ExitReadNetwork)
	if calls.Load() != 1 {
		t.Fatalf("canceled request dispatched; calls=%d", calls.Load())
	}
}

func TestPublishTrainerCancellationAfterDispatchIsUncertain(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	dispatched := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		close(dispatched)
		<-release
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.PublishTrainer(ctx, "token", trainer, CallOptions{Timeout: time.Second}) }()
	<-dispatched
	cancel()
	err := <-done
	close(release)
	assertExitCode(t, err, output.ExitWriteOutcomeUnknown)
	if calls.Load() != 1 {
		t.Fatalf("calls=%d want=1", calls.Load())
	}
}

func TestPublishTrainerIsStatelessAcrossConcurrentCalls(t *testing.T) {
	var mutex sync.Mutex
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		mutex.Lock()
		seen[request.PostForm.Get("token")] = request.PostForm.Get("input_tid") + ":" + request.PostForm.Get("input_code")
		mutex.Unlock()
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	var wait sync.WaitGroup
	for index, token := range []string{"token-a", "token-b"} {
		wait.Add(1)
		go func(index int, token string) {
			defer wait.Done()
			trainer, _ := domain.NewTrainer(fmt.Sprintf("Ash%d", index), fmt.Sprintf("12345678901%d", index), 50, "se")
			if err := client.PublishTrainer(context.Background(), token, trainer, CallOptions{}); err != nil {
				t.Errorf("publish: %v", err)
			}
		}(index, token)
	}
	wait.Wait()
	want := map[string]string{"token-a": "Ash0:123456789010", "token-b": "Ash1:123456789011"}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("seen=%#v want=%#v", seen, want)
	}
	for _, rendered := range []string{fmt.Sprintf("%v", client), fmt.Sprintf("%+v", client), fmt.Sprintf("%#v", client)} {
		if strings.Contains(rendered, "token-") || strings.Contains(rendered, "Ash") || strings.Contains(rendered, "123456") {
			t.Fatalf("client formatting leaked state: %q", rendered)
		}
	}
}
