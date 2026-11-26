package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/albinmedoc/raidnow-cli/internal/htmlscript"
)

const (
	MaxHTMLBytes             = 4 << 20
	MaxExecutableScriptBytes = 2 << 20
	MaxEntries               = 4096
	MaxIDBytes               = 256
	MaxNameBytes             = 256
	MaxImageURLBytes         = 2048
)

var (
	ErrInvalidMetadata = errors.New("invalid pokemon metadata")
	aryPokemonDecl     = regexp.MustCompile(`\b(?:var|let|const)[ 	\r\n]+aryPokemon[ 	\r\n]*=`)
	readyWrapperPrefix = regexp.MustCompile(`(?s)^[ 	\r\n]*jQuery[ 	\r\n]*\([ 	\r\n]*function[ 	\r\n]*\([ 	\r\n]*\$[ 	\r\n]*\)[ 	\r\n]*\{[ 	\r\n]*$`)
)

// Parse extracts exactly one server-rendered aryPokemon JSON object.
func Parse(body []byte) (catalog Catalog, err error) {
	defer func() {
		if recover() != nil {
			catalog = Catalog{}
			err = ErrInvalidMetadata
		}
	}()
	if len(body) > MaxHTMLBytes || !utf8.Valid(body) {
		return Catalog{}, ErrInvalidMetadata
	}
	sources, parseErr := htmlscript.Sources(body)
	if parseErr != nil {
		return Catalog{}, ErrInvalidMetadata
	}

	var candidates [][]byte
	for _, source := range sources {
		objects, scanErr := aryPokemonObjects(source)
		if scanErr != nil {
			return Catalog{}, ErrInvalidMetadata
		}
		candidates = append(candidates, objects...)
	}
	if len(candidates) != 1 {
		return Catalog{}, ErrInvalidMetadata
	}

	var raw map[string]json.RawMessage
	if !hasUniqueJSONKeys(candidates[0]) || json.Unmarshal(candidates[0], &raw) != nil || len(raw) == 0 || len(raw) > MaxEntries {
		return Catalog{}, ErrInvalidMetadata
	}
	entries := make(map[string]Entry, len(raw))
	for id, value := range raw {
		if len(id) == 0 || len(id) > MaxIDBytes || strings.TrimSpace(id) == "" || !safeText(id) {
			return Catalog{}, ErrInvalidMetadata
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return Catalog{}, ErrInvalidMetadata
		}
		var fields struct {
			NameEN   string          `json:"en"`
			ImageURL string          `json:"image"`
			Shiny    json.RawMessage `json:"shiny"`
			MaxCP20  json.RawMessage `json:"maxcp20"`
			MaxCP25  json.RawMessage `json:"maxcp25"`
		}
		if json.Unmarshal(value, &fields) != nil || len(fields.NameEN) > MaxNameBytes || len(fields.ImageURL) > MaxImageURLBytes || !safeText(fields.NameEN) || !safeText(fields.ImageURL) {
			return Catalog{}, ErrInvalidMetadata
		}
		shinyAvailable, shinyErr := parseShiny(fields.Shiny)
		maxCP20, maxCP20Err := parseEncounterCP(fields.MaxCP20)
		maxCP25, maxCP25Err := parseEncounterCP(fields.MaxCP25)
		if shinyErr != nil || maxCP20Err != nil || maxCP25Err != nil {
			return Catalog{}, ErrInvalidMetadata
		}
		entries[id] = Entry{
			ID:             id,
			NameEN:         fields.NameEN,
			ImageURL:       fields.ImageURL,
			ShinyAvailable: shinyAvailable,
			MaxCP20:        maxCP20,
			MaxCP25:        maxCP25,
		}
	}
	return Catalog{entries: entries}, nil
}

func hasUniqueJSONKeys(source []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	if validateJSONValue(decoder) != nil {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

func validateJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			key, keyOK := keyToken.(string)
			if keyErr != nil || !keyOK {
				return ErrInvalidMetadata
			}
			if _, duplicate := keys[key]; duplicate {
				return ErrInvalidMetadata
			}
			keys[key] = struct{}{}
			if valueErr := validateJSONValue(decoder); valueErr != nil {
				return valueErr
			}
		}
		end, endErr := decoder.Token()
		if endErr != nil || end != json.Delim('}') {
			return ErrInvalidMetadata
		}
	case '[':
		for decoder.More() {
			if valueErr := validateJSONValue(decoder); valueErr != nil {
				return valueErr
			}
		}
		end, endErr := decoder.Token()
		if endErr != nil || end != json.Delim(']') {
			return ErrInvalidMetadata
		}
	default:
		return ErrInvalidMetadata
	}
	return nil
}

func parseShiny(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	switch string(bytes.TrimSpace(raw)) {
	case "0", `"0"`:
		return false, nil
	case "1", `"1"`:
		return true, nil
	default:
		return false, ErrInvalidMetadata
	}
}

func parseEncounterCP(raw json.RawMessage) (*int, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value int
	if json.Unmarshal(raw, &value) == nil {
		if value <= 0 {
			return nil, ErrInvalidMetadata
		}
		return &value, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return nil, ErrInvalidMetadata
}

func aryPokemonObjects(source []byte) ([][]byte, error) {
	if len(source) > MaxExecutableScriptBytes {
		return nil, ErrInvalidMetadata
	}
	matches := aryPokemonDecl.FindAllIndex(source, -1)
	objects := make([][]byte, 0, len(matches))
	for _, match := range matches {
		_, topLevel, prefixOK := maskJavaScript(source[:match[1]])
		if (!prefixOK || !topLevel[match[0]]) && !readyWrapperPrefix.Match(source[:match[0]]) {
			continue
		}
		start := skipJSONSpace(source, match[1])
		if start >= len(source) || source[start] != '{' {
			return nil, ErrInvalidMetadata
		}
		end, balanced := balancedObjectEnd(source, start)
		if !balanced {
			return nil, ErrInvalidMetadata
		}
		trailing := skipJSONSpace(source, end)
		if trailing < len(source) && source[trailing] != ';' {
			return nil, ErrInvalidMetadata
		}
		objects = append(objects, source[start:end])
	}
	return objects, nil
}

func maskJavaScript(source []byte) ([]byte, []bool, bool) {
	masked := append([]byte(nil), source...)
	topLevel := make([]bool, len(source)+1)
	braceDepth, parenDepth, bracketDepth := 0, 0, 0
	for index := 0; index < len(source); {
		topLevel[index] = braceDepth == 0 && parenDepth == 0 && bracketDepth == 0
		switch {
		case source[index] == '/' && index+1 < len(source) && source[index+1] == '/':
			end := index + 2
			for end < len(source) && source[end] != '\n' && source[end] != '\r' {
				end++
			}
			blank(masked, index, end)
			index = end
		case source[index] == '/' && index+1 < len(source) && source[index+1] == '*':
			end := bytes.Index(source[index+2:], []byte("*/"))
			if end < 0 {
				return nil, nil, false
			}
			end += index + 4
			blank(masked, index, end)
			index = end
		case source[index] == '\'' || source[index] == '"' || source[index] == '`':
			quote := source[index]
			end := index + 1
			closed := false
			for end < len(source) {
				if source[end] == '\\' {
					end += 2
					continue
				}
				if source[end] == quote {
					end++
					closed = true
					break
				}
				end++
			}
			if !closed {
				return nil, nil, false
			}
			blank(masked, index, end)
			index = end
		default:
			switch source[index] {
			case '{':
				braceDepth++
			case '}':
				braceDepth--
			case '(':
				parenDepth++
			case ')':
				parenDepth--
			case '[':
				bracketDepth++
			case ']':
				bracketDepth--
			}
			if braceDepth < 0 || parenDepth < 0 || bracketDepth < 0 {
				return nil, nil, false
			}
			index++
		}
	}
	if braceDepth != 0 || parenDepth != 0 || bracketDepth != 0 {
		return nil, nil, false
	}
	topLevel[len(source)] = braceDepth == 0 && parenDepth == 0 && bracketDepth == 0
	return masked, topLevel, true
}

func balancedObjectEnd(source []byte, start int) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(source); index++ {
		character := source[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index + 1, true
			}
		}
	}
	return 0, false
}

func blank(data []byte, start, end int) {
	for index := start; index < end; index++ {
		if data[index] != '\n' && data[index] != '\r' {
			data[index] = ' '
		}
	}
}

func skipJSONSpace(data []byte, index int) int {
	for index < len(data) {
		switch data[index] {
		case ' ', '\t', '\n', '\r':
			index++
		default:
			return index
		}
	}
	return index
}

func safeText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}
