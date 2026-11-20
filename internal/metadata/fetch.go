package metadata

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

var ErrMetadataUnavailable = errors.New("pokemon metadata unavailable")

type HomepageFetcher struct {
	client  *http.Client
	url     string
	timeout time.Duration
}

func NewHomepageFetcher(client *http.Client, url string) *HomepageFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &HomepageFetcher{client: client, url: url, timeout: 10 * time.Second}
}

func (fetcher *HomepageFetcher) Catalog(ctx context.Context) (Catalog, error) {
	timeout := fetcher.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, fetcher.url, nil)
	if err != nil {
		return Catalog{}, ErrMetadataUnavailable
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; raidnow-cli/metadata)")
	request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
	client := *fetcher.client
	client.Jar = nil
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return Catalog{}, ErrMetadataUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Catalog{}, ErrMetadataUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxHTMLBytes+1))
	if err != nil || len(body) > MaxHTMLBytes {
		return Catalog{}, ErrMetadataUnavailable
	}
	catalog, err := Parse(body)
	if err != nil {
		return Catalog{}, ErrMetadataUnavailable
	}
	return catalog, nil
}
