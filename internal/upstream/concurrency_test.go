package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientFormattingDoesNotExposeEndpointQuery(t *testing.T) {
	client := mustClient(t, Config{Endpoint: "https://example.com/form?token=secret-marker&trainer=123456789012"})
	formatted := fmt.Sprintf("%v %#v %+v", client, client, client)
	if strings.Contains(formatted, "secret-marker") || strings.Contains(formatted, "123456789012") || strings.Contains(formatted, "token=") {
		t.Fatalf("client formatting leaked endpoint query: %s", formatted)
	}
}

func TestDispatchTrackingTransportMarksOnlyEnteredRoundTrips(t *testing.T) {
	var called atomic.Bool
	transport := dispatchTrackingTransport{base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called.Store(true)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})}
	var dispatched atomic.Bool
	request, err := http.NewRequestWithContext(context.WithValue(context.Background(), dispatchStateKey{}, &dispatched), http.MethodPost, "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = response.Body.Close()
	if !called.Load() || !dispatched.Load() {
		t.Fatalf("called=%t dispatched=%t, want both true", called.Load(), dispatched.Load())
	}
}

func TestConcurrentCallsKeepRequestStateIsolated(t *testing.T) {
	const callCount = 40
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
			return
		}
		id := values.Get("id")
		if values.Get("type") != "operation_"+id {
			t.Errorf("type = %q, want operation_%s", values.Get("type"), id)
		}
		if got := values["field[]"]; len(got) != 2 || got[0] != "first_"+id || got[1] != "second_"+id {
			t.Errorf("field values = %#v for id %s", got, id)
		}
		_, _ = fmt.Fprintf(response, `{"id":%q}`, id)
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client()})

	var wait sync.WaitGroup
	errorsChannel := make(chan error, callCount)
	for index := range callCount {
		id := strconv.Itoa(index)
		wait.Add(1)
		go func() {
			defer wait.Done()
			values := url.Values{"id": {id}, "field[]": {"first_" + id, "second_" + id}}
			result, err := client.PostForm(context.Background(), "operation_"+id, values, OperationPolicy{Kind: ReadOperation, Timeout: time.Second})
			if err != nil {
				errorsChannel <- err
				return
			}
			if !strings.Contains(string(result), `"`+id+`"`) {
				errorsChannel <- fmt.Errorf("result %q does not contain id %s", result, id)
			}
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Error(err)
	}
	if got := requests.Load(); got != callCount {
		t.Fatalf("requests = %d, want %d", got, callCount)
	}
}
