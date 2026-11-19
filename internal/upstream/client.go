package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

const DefaultEndpoint = "https://pokemongo-get.com/wp-content/themes/simplicity2-child/include/gofriend_enraid_masta.php?v=2025121601"

type OperationKind uint8

const (
	ReadOperation OperationKind = iota + 1
	WriteOperation
)

// CallOptions contains per-call settings that do not alter registered
// operation safety metadata. A zero Timeout uses the operation default.
type CallOptions struct {
	Timeout time.Duration
}

type Config struct {
	Endpoint string

	// HTTPClient may use a nil/default transport or a standard *http.Transport.
	// Other public RoundTripper implementations are rejected because they may
	// not preserve httptrace.WroteRequest dispatch classification.
	HTTPClient *http.Client

	// AllowInsecureHTTP permits a cleartext endpoint only for explicit local
	// test configurations. Production callers should leave it false.
	AllowInsecureHTTP bool
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

// NewClient constructs a stateless upstream client. Injected clients must use
// the standard HTTP transport so write dispatch remains traceable.
func NewClient(config Config) (*Client, error) {
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Host == "" || parsedEndpoint.User != nil || parsedEndpoint.Fragment != "" || parsedEndpoint.Opaque != "" {
		return nil, invalidArguments("Ogiltig transportkonfiguration.")
	}
	if parsedEndpoint.Scheme != "https" && !(parsedEndpoint.Scheme == "http" && config.AllowInsecureHTTP) {
		return nil, invalidArguments("Ogiltig transportkonfiguration.")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Transport: http.DefaultTransport}
	}
	if !supportedTransport(httpClient.Transport) {
		return nil, invalidArguments("Ogiltig transportkonfiguration.")
	}
	clientCopy := *httpClient
	if clientCopy.Transport == nil {
		clientCopy.Transport = http.DefaultTransport
	}
	clientCopy.Jar = nil
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{endpoint: endpoint, httpClient: &clientCopy}, nil
}

type traceAwareRoundTripper interface {
	http.RoundTripper
	upstreamTraceAware()
}

func supportedTransport(transport http.RoundTripper) bool {
	if transport == nil {
		return true
	}
	if standard, ok := transport.(*http.Transport); ok {
		return standard != nil
	}
	_, ok := transport.(traceAwareRoundTripper)
	return ok
}

func (client *Client) PostForm(ctx context.Context, operation Operation, values url.Values, options CallOptions) (json.RawMessage, error) {
	if ctx == nil {
		return nil, invalidArguments("Ogiltig upstream-operation.")
	}
	descriptor, err := operation.Descriptor()
	if err != nil {
		return nil, err
	}
	if _, supplied := values["type"]; supplied {
		return nil, invalidArguments("Formuläret innehåller ett reserverat fält.")
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = descriptor.DefaultTimeout()
	}
	if timeout <= 0 {
		return nil, invalidArguments("Ogiltig timeout.")
	}
	return client.postForm(ctx, descriptor, values, timeout)
}
