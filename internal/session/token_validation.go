package session

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// AnonymousTokenIsValid applies the shared anonymous-token contract used by
// bootstrap, protected file input, and every upstream invocation.
func AnonymousTokenIsValid(value string) bool {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > MaxTokenBytes || strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

func tokenIsValid(value string) bool {
	return AnonymousTokenIsValid(value)
}
