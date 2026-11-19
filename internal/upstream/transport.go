package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync/atomic"
	"time"
)

const browserUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"

const maxResponseBytes = 1 << 20

func (client *Client) postForm(ctx context.Context, operation OperationDescriptor, values url.Values, timeout time.Duration) (json.RawMessage, error) {
	if ctx.Err() != nil {
		return nil, readNetworkFailure()
	}
	form := cloneValues(values)
	form.Set("type", operation.Name())

	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var dispatched atomic.Bool
	requestContext = httptrace.WithClientTrace(requestContext, &httptrace.ClientTrace{
		WroteRequest: func(httptrace.WroteRequestInfo) {
			dispatched.Store(true)
		},
	})
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, client.endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, invalidArguments("Ogiltig transportkonfiguration.")
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Origin", "https://raidnow.leekduck.com")
	request.Header.Set("Referer", "https://raidnow.leekduck.com/")
	request.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, transportFailure(operation.Kind(), dispatched.Load())
	}
	dispatched.Store(true)
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, upstreamRejection()
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if len(body) > maxResponseBytes {
		return nil, upstreamRejection()
	}
	if err != nil {
		return nil, transportFailure(operation.Kind(), dispatched.Load())
	}
	if len(bytes.TrimSpace(body)) == 0 {
		if operation.AllowsEmptyResponse() {
			return json.RawMessage{}, nil
		}
		return nil, upstreamRejection()
	}
	if !json.Valid(body) {
		return nil, upstreamRejection()
	}
	return json.RawMessage(bytes.Clone(body)), nil
}

func cloneValues(values url.Values) url.Values {
	clone := make(url.Values, len(values)+1)
	for key, entries := range values {
		clone[key] = append([]string(nil), entries...)
	}
	return clone
}
