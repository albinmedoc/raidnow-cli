package session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	DefaultBootstrapURL = "https://raidnow.leekduck.com/"

	// MaxBootstrapBodyBytes bounds the server-rendered page accepted by the CLI.
	MaxBootstrapBodyBytes = 1 << 20
	MaxTokenBytes         = 4096
	defaultTimeout        = 10 * time.Second
)

var (
	errBootstrap       = errors.New("anonymous token bootstrap failed")
	tokenPropertyStart = regexp.MustCompile(`(?:["']gofriend_token["']|(?:^|[^\pL\pN\pM\p{Pc}$\x{200C}\x{200D}])gofriend_token)\s*:\s*(["'])`)
)

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

func extractToken(body []byte) (string, error) {
	match := tokenPropertyStart.FindSubmatchIndex(body)
	if len(match) != 4 {
		return "", errBootstrap
	}
	quote := body[match[2]]
	value, err := decodeJSString(body[match[1]:], quote)
	if err != nil || len(value) == 0 || len(value) > MaxTokenBytes || strings.TrimSpace(value) == "" {
		return "", errBootstrap
	}
	if strings.ContainsAny(value, "\r\n\x00") || strings.ContainsRune(value, '\u2028') || strings.ContainsRune(value, '\u2029') {
		return "", errBootstrap
	}
	return value, nil
}

func decodeJSString(input []byte, quote byte) (string, error) {
	var decoded strings.Builder
	for index := 0; index < len(input); index++ {
		value := input[index]
		if value == quote {
			return decoded.String(), nil
		}
		if value != '\\' {
			decoded.WriteByte(value)
			if decoded.Len() > MaxTokenBytes {
				return "", errBootstrap
			}
			continue
		}
		index++
		if index >= len(input) {
			return "", errBootstrap
		}
		escaped := input[index]
		switch escaped {
		case '\'', '"', '\\', '/':
			decoded.WriteByte(escaped)
		case 'b':
			decoded.WriteByte('\b')
		case 'f':
			decoded.WriteByte('\f')
		case 'n':
			decoded.WriteByte('\n')
		case 'r':
			decoded.WriteByte('\r')
		case 't':
			decoded.WriteByte('\t')
		case 'v':
			decoded.WriteByte('\v')
		case '0':
			if index+1 < len(input) && input[index+1] >= '0' && input[index+1] <= '9' {
				return "", errBootstrap
			}
			decoded.WriteByte(0)
		case 'x':
			value, next, err := decodeHexEscape(input, index+1, 2)
			if err != nil {
				return "", errBootstrap
			}
			decoded.WriteRune(rune(value))
			index = next - 1
		case 'u':
			value, next, err := decodeHexEscape(input, index+1, 4)
			if err != nil {
				return "", errBootstrap
			}
			index = next - 1
			r := rune(value)
			if utf16.IsSurrogate(r) {
				if index+6 >= len(input) || input[index+1] != '\\' || input[index+2] != 'u' {
					return "", errBootstrap
				}
				low, afterLow, lowErr := decodeHexEscape(input, index+3, 4)
				if lowErr != nil {
					return "", errBootstrap
				}
				combined := utf16.DecodeRune(r, rune(low))
				if combined == utf8.RuneError {
					return "", errBootstrap
				}
				r = combined
				index = afterLow - 1
			}
			decoded.WriteRune(r)
		default:
			return "", errBootstrap
		}
		if decoded.Len() > MaxTokenBytes {
			return "", errBootstrap
		}
	}
	return "", errBootstrap
}

func decodeHexEscape(input []byte, start, length int) (uint64, int, error) {
	end := start + length
	if end > len(input) {
		return 0, start, errBootstrap
	}
	value, err := strconv.ParseUint(string(input[start:end]), 16, 16)
	if err != nil {
		return 0, start, errBootstrap
	}
	return value, end, nil
}
