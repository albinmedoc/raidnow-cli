package session

import (
	"strconv"
	"strings"

	"github.com/albinmedoc/raidnow-cli/internal/htmlscript"
)

func extractToken(body []byte) (token string, err error) {
	defer func() {
		if recover() != nil {
			token = ""
			err = errBootstrap
		}
	}()

	sources, parseErr := htmlscript.Sources(body)
	if parseErr != nil {
		return "", errBootstrap
	}
	var candidates []string
	for _, source := range sources {
		found, scanErr := tokenCandidates(source)
		if scanErr != nil {
			return "", errBootstrap
		}
		candidates = append(candidates, found...)
	}
	if len(candidates) != 1 || !AnonymousTokenIsValid(candidates[0]) {
		return "", errBootstrap
	}
	return candidates[0], nil
}

func tokenCandidates(source []byte) ([]string, error) {
	var candidates []string
	for index := 0; index < len(source); {
		switch {
		case source[index] == '/' && index+1 < len(source) && source[index+1] == '/':
			index = skipLineComment(source, index+2)
		case source[index] == '/' && index+1 < len(source) && source[index+1] == '*':
			var ok bool
			index, ok = skipBlockComment(source, index+2)
			if !ok {
				return nil, errBootstrap
			}
		case source[index] == '`':
			var ok bool
			index, ok = skipQuoted(source, index, '`')
			if !ok {
				return nil, errBootstrap
			}
		case source[index] == '\'' || source[index] == '"':
			next, ok := skipQuoted(source, index, source[index])
			if !ok {
				return nil, errBootstrap
			}
			if string(source[index+1:next-1]) == "gofriend_token" {
				value, after, candidateErr := tokenPropertyValue(source, next)
				if candidateErr != nil {
					return nil, errBootstrap
				}
				if after != next {
					candidates = append(candidates, value)
					index = after
					continue
				}
			}
			index = next
		case hasIdentifierAt(source, index, "gofriend_token"):
			value, next, candidateErr := tokenPropertyValue(source, index+len("gofriend_token"))
			if candidateErr != nil {
				return nil, errBootstrap
			}
			if next != index+len("gofriend_token") {
				candidates = append(candidates, value)
				index = next
				continue
			}
			index++
		default:
			index++
		}
	}
	return candidates, nil
}

func tokenPropertyValue(source []byte, index int) (string, int, error) {
	start := index
	index = skipSpace(source, index)
	if index >= len(source) || source[index] != ':' {
		return "", start, nil
	}
	index = skipSpace(source, index+1)
	if index >= len(source) || source[index] != '\'' && source[index] != '"' {
		return "", 0, errBootstrap
	}
	value, next, err := decodeASCIIJavaScriptString(source, index)
	if err != nil || !AnonymousTokenIsValid(value) {
		return "", 0, errBootstrap
	}
	return value, next, nil
}

func decodeASCIIJavaScriptString(source []byte, start int) (string, int, error) {
	if start >= len(source) || source[start] != '\'' && source[start] != '"' {
		return "", start, errBootstrap
	}
	quote := source[start]
	var decoded strings.Builder
	for index := start + 1; index < len(source); index++ {
		character := source[index]
		if character == quote {
			return decoded.String(), index + 1, nil
		}
		if character == '\n' || character == '\r' || character > 0x7f {
			return "", start, errBootstrap
		}
		if character != '\\' {
			decoded.WriteByte(character)
		} else {
			index++
			if index >= len(source) {
				return "", start, errBootstrap
			}
			switch source[index] {
			case '\'', '"', '\\', '/':
				decoded.WriteByte(source[index])
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
			case 'x', 'u':
				digits := 2
				if source[index] == 'u' {
					digits = 4
				}
				if index+digits >= len(source) {
					return "", start, errBootstrap
				}
				value, parseErr := strconv.ParseUint(string(source[index+1:index+1+digits]), 16, 8)
				if parseErr != nil || value > 0x7f {
					return "", start, errBootstrap
				}
				decoded.WriteByte(byte(value))
				index += digits
			default:
				return "", start, errBootstrap
			}
		}
		if decoded.Len() > MaxTokenBytes {
			return "", start, errBootstrap
		}
	}
	return "", start, errBootstrap
}

func skipQuoted(source []byte, start int, quote byte) (int, bool) {
	for index := start + 1; index < len(source); index++ {
		if source[index] == '\\' {
			index++
			continue
		}
		if source[index] == quote {
			return index + 1, true
		}
	}
	return len(source), false
}

func skipLineComment(source []byte, index int) int {
	for index < len(source) && source[index] != '\n' && source[index] != '\r' {
		index++
	}
	return index
}

func skipBlockComment(source []byte, index int) (int, bool) {
	for index+1 < len(source) {
		if source[index] == '*' && source[index+1] == '/' {
			return index + 2, true
		}
		index++
	}
	return len(source), false
}

func skipSpace(source []byte, index int) int {
	for index < len(source) {
		switch source[index] {
		case ' ', '\t', '\n', '\r':
			index++
		default:
			return index
		}
	}
	return index
}

func hasIdentifierAt(source []byte, index int, identifier string) bool {
	if index+len(identifier) > len(source) || string(source[index:index+len(identifier)]) != identifier {
		return false
	}
	return (index == 0 || !identifierByte(source[index-1])) &&
		(index+len(identifier) == len(source) || !identifierByte(source[index+len(identifier)]))
}

func identifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_' || value == '$'
}
