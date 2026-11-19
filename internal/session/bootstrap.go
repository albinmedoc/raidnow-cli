package session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
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

func extractToken(body []byte) (string, error) {
	for _, script := range scriptContents(body) {
		if token, ok := tokenFromScript(script); ok {
			return validateToken(token)
		}
	}
	return "", errBootstrap
}

func validateToken(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", errBootstrap
	}
	if len(value) == 0 || len(value) > MaxTokenBytes || strings.TrimSpace(value) == "" {
		return "", errBootstrap
	}
	for _, value := range value {
		if unicode.IsControl(value) || value == '\u2028' || value == '\u2029' {
			return "", errBootstrap
		}
	}
	return value, nil
}

func scriptContents(body []byte) [][]byte {
	var scripts [][]byte
	for index := 0; index < len(body); {
		if strings.HasPrefix(string(body[index:]), "<!--") {
			end := strings.Index(string(body[index+4:]), "-->")
			if end < 0 {
				break
			}
			index += 4 + end + 3
			continue
		}
		if body[index] != '<' {
			index++
			continue
		}
		nameStart := index + 1
		for nameStart < len(body) && isHTMLSpace(body[nameStart]) {
			nameStart++
		}
		nameEnd := nameStart
		for nameEnd < len(body) && isASCIIAlpha(body[nameEnd]) {
			nameEnd++
		}
		if !strings.EqualFold(string(body[nameStart:nameEnd]), "script") {
			index++
			continue
		}
		tagEnd := htmlTagEnd(body, nameEnd)
		if tagEnd < 0 {
			break
		}
		closeStart := indexFold(body, []byte("</script"), tagEnd+1)
		if closeStart < 0 {
			break
		}
		closeEnd := htmlTagEnd(body, closeStart+len("</script"))
		if closeEnd < 0 {
			break
		}
		scripts = append(scripts, body[tagEnd+1:closeStart])
		index = closeEnd + 1
	}
	return scripts
}

func tokenFromScript(script []byte) (string, bool) {
	for index := 0; index < len(script); {
		next, kind := skipJSToken(script, index)
		if kind == jsTokenIgnored {
			index = next
			continue
		}
		keyStart, keyEnd := index, next
		if kind == jsTokenIdentifier && string(script[keyStart:keyEnd]) != "gofriend_token" {
			index = next
			continue
		}
		if kind == jsTokenString {
			key, err := decodeJSString(script[keyStart+1:keyEnd], script[keyStart])
			if err != nil || key != "gofriend_token" {
				index = next
				continue
			}
		} else if kind != jsTokenIdentifier {
			index = next
			continue
		}
		position := skipJSTrivia(script, keyEnd)
		if position >= len(script) || script[position] != ':' {
			index = next
			continue
		}
		position = skipJSTrivia(script, position+1)
		if position >= len(script) || (script[position] != '\'' && script[position] != '"') {
			index = next
			continue
		}
		valueEnd := quotedJSEnd(script, position)
		if valueEnd < 0 {
			return "", false
		}
		value, err := decodeJSString(script[position+1:valueEnd+1], script[position])
		return value, err == nil
	}
	return "", false
}

const (
	jsTokenOther = iota
	jsTokenIgnored
	jsTokenIdentifier
	jsTokenString
)

func skipJSToken(script []byte, index int) (int, int) {
	if isJSSpace(script[index]) || (script[index] == '/' && index+1 < len(script) && (script[index+1] == '/' || script[index+1] == '*')) {
		return skipJSTrivia(script, index), jsTokenIgnored
	}
	if script[index] == '\'' || script[index] == '"' || script[index] == '`' {
		end := quotedJSEnd(script, index)
		if end < 0 {
			return len(script), jsTokenIgnored
		}
		if script[index] == '`' {
			return end + 1, jsTokenIgnored
		}
		return end + 1, jsTokenString
	}
	r, size := utf8.DecodeRune(script[index:])
	if !isJSIdentifierStart(r) {
		return index + size, jsTokenOther
	}
	end := index + size
	for end < len(script) {
		r, size = utf8.DecodeRune(script[end:])
		if !isJSIdentifierPart(r) {
			break
		}
		end += size
	}
	return end, jsTokenIdentifier
}

func skipJSTrivia(script []byte, index int) int {
	for index < len(script) {
		if isJSSpace(script[index]) {
			index++
			continue
		}
		if index+1 >= len(script) || script[index] != '/' {
			break
		}
		switch script[index+1] {
		case '/':
			index += 2
			for index < len(script) && script[index] != '\n' && script[index] != '\r' {
				index++
			}
		case '*':
			end := strings.Index(string(script[index+2:]), "*/")
			if end < 0 {
				return len(script)
			}
			index += 2 + end + 2
		default:
			return index
		}
	}
	return index
}

func quotedJSEnd(script []byte, start int) int {
	quote := script[start]
	for index := start + 1; index < len(script); index++ {
		if script[index] == '\\' {
			index++
			continue
		}
		if script[index] == quote {
			return index
		}
		if quote != '`' && (script[index] == '\n' || script[index] == '\r') {
			return -1
		}
	}
	return -1
}

func htmlTagEnd(body []byte, start int) int {
	var quote byte
	for index := start; index < len(body); index++ {
		if quote != 0 {
			if body[index] == quote {
				quote = 0
			}
			continue
		}
		if body[index] == '\'' || body[index] == '"' {
			quote = body[index]
		} else if body[index] == '>' {
			return index
		}
	}
	return -1
}

func indexFold(haystack, needle []byte, start int) int {
	for index := start; index+len(needle) <= len(haystack); index++ {
		if strings.EqualFold(string(haystack[index:index+len(needle)]), string(needle)) {
			return index
		}
	}
	return -1
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isHTMLSpace(value byte) bool {
	return value == ' ' || value == '	' || value == '\n' || value == '\r' || value == '\f'
}

func isJSSpace(value byte) bool {
	return value == ' ' || value == '	' || value == '\n' || value == '\r' || value == '\f' || value == '\v'
}

func isJSIdentifierStart(value rune) bool {
	return value == '$' || value == '_' || unicode.IsLetter(value)
}

func isJSIdentifierPart(value rune) bool {
	return isJSIdentifierStart(value) || unicode.IsDigit(value) || unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Mc, value) || unicode.Is(unicode.Pc, value) || value == '\u200c' || value == '\u200d'
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
