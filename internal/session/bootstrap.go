package session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

const (
	DefaultBootstrapURL = "https://raidnow.leekduck.com/"

	// MaxBootstrapBodyBytes bounds the server-rendered page accepted by the CLI.
	MaxBootstrapBodyBytes = 1 << 20
	MaxTokenBytes         = 4096
	defaultTimeout        = 10 * time.Second
)

var errBootstrap = errors.New("anonymous token bootstrap failed")

type Bootstrap struct {
	client  *http.Client
	baseURL string
	timeout time.Duration
}

func NewBootstrap(client *http.Client, baseURL string) *Bootstrap {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = DefaultBootstrapURL
	}
	return &Bootstrap{client: client, baseURL: baseURL, timeout: defaultTimeout}
}

func (bootstrap *Bootstrap) Token(ctx context.Context) (string, error) {
	timeout := bootstrap.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, bootstrap.baseURL, nil)
	if err != nil {
		return "", errBootstrap
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; raidnow-cli/anonymous-bootstrap)")
	request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")

	client := *bootstrap.client
	client.Jar = nil
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := client.Do(request)
	if err != nil {
		return "", errBootstrap
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", errBootstrap
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, MaxBootstrapBodyBytes+1))
	if err != nil || len(body) > MaxBootstrapBodyBytes {
		return "", errBootstrap
	}
	token, err := extractToken(body)
	if err != nil {
		return "", errBootstrap
	}
	return token, nil
}
