package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

const DefaultEndpoint = "https://pokemongo-get.com/wp-content/themes/simplicity2-child/include/gofriend_enraid_masta.php?v=2025121601"

var operationNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

type OperationKind uint8

const (
	ReadOperation OperationKind = iota + 1
	WriteOperation
)

type OperationPolicy struct {
	Kind               OperationKind
	Timeout            time.Duration
	AllowEmptyResponse bool
}

type Config struct {
	Endpoint   string
	HTTPClient *http.Client
}

type Client struct {
	endpoint   string
	httpClient *http.Client
}

func (client *Client) String() string {
	return "upstream.Client"
}

func (client *Client) GoString() string {
	return "upstream.Client{}"
}

func NewClient(config Config) (*Client, error) {
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || (parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") || parsedEndpoint.Host == "" || parsedEndpoint.User != nil || parsedEndpoint.Fragment != "" {
		return nil, invalidArguments("Ogiltig transportkonfiguration.")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	clientCopy := *httpClient
	clientCopy.Jar = nil
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{endpoint: endpoint, httpClient: &clientCopy}, nil
}

func (client *Client) PostForm(ctx context.Context, operation string, values url.Values, policy OperationPolicy) (json.RawMessage, error) {
	if ctx == nil || !operationNamePattern.MatchString(operation) {
		return nil, invalidArguments("Ogiltig upstream-operation.")
	}
	if _, supplied := values["type"]; supplied {
		return nil, invalidArguments("Formuläret innehåller ett reserverat fält.")
	}
	if (policy.Kind != ReadOperation && policy.Kind != WriteOperation) || policy.Timeout <= 0 {
		return nil, invalidArguments("Ogiltig operationspolicy.")
	}
	if policy.AllowEmptyResponse && operation != "user_info_post" {
		return nil, invalidArguments("Tomt svar är inte tillåtet för operationen.")
	}
	return client.postForm(ctx, operation, values, policy)
}
