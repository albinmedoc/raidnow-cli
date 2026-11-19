package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/albinmedoc/raidnow-cli/internal/htmlscript"
	"github.com/tdewolff/parse/v2"
	jsparser "github.com/tdewolff/parse/v2/js"
)

const (
	MaxHTMLBytes = 4 << 20
	// MaxExecutableScriptBytes bounds each inline script before JavaScript parsing.
	MaxExecutableScriptBytes = 2 << 20
	// MaxJavaScriptTokens bounds lexer work and syntax-tree complexity per inline script.
	MaxJavaScriptTokens = 100_000
	MaxEntries          = 4096
	MaxIDBytes          = 256
	MaxNameBytes        = 256
	MaxImageURLBytes    = 2048
	MaxJSONDepth        = 32
	// MaxJSONStructuralNodes bounds combined object, array, member, element, and scalar nodes.
	MaxJSONStructuralNodes = 50_000
)

var ErrInvalidMetadata = errors.New("invalid pokemon metadata")

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
		rawObjects, lexErr := aryPokemonJSONObjects(source)
		if lexErr != nil {
			return Catalog{}, ErrInvalidMetadata
		}
		ast, astErr := jsparser.Parse(parse.NewInputBytes(source), jsparser.Options{})
		if astErr != nil {
			return Catalog{}, ErrInvalidMetadata
		}
		objects := topLevelAryPokemonObjects(ast)
		if len(objects) != len(rawObjects) {
			return Catalog{}, ErrInvalidMetadata
		}
		for index, object := range objects {
			if object == nil || validateJSON(rawObjects[index]) != nil {
				return Catalog{}, ErrInvalidMetadata
			}
			candidates = append(candidates, rawObjects[index])
		}
	}
	if len(candidates) != 1 {
		return Catalog{}, ErrInvalidMetadata
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(candidates[0], &raw); err != nil || len(raw) == 0 || len(raw) > MaxEntries {
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
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(value, &fields); err != nil {
			return Catalog{}, ErrInvalidMetadata
		}
		name, fieldErr := optionalString(fields["en"], MaxNameBytes)
		if fieldErr != nil {
			return Catalog{}, ErrInvalidMetadata
		}
		imageURL, fieldErr := optionalString(fields["image"], MaxImageURLBytes)
		if fieldErr != nil {
			return Catalog{}, ErrInvalidMetadata
		}
		entries[id] = Entry{id: id, name: name, imageURL: imageURL}
	}
	return Catalog{entries: entries}, nil
}

func optionalString(raw json.RawMessage, limit int) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", ErrInvalidMetadata
	}
	value = strings.TrimSpace(value)
	if len(value) > limit || !safeText(value) {
		return "", ErrInvalidMetadata
	}
	return value, nil
}

func safeText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

func validateJSON(data []byte) error {
	parser := strictJSONParser{data: data}
	if err := parser.parseValue(1); err != nil {
		return err
	}
	parser.skipWhitespace()
	if parser.position != len(parser.data) {
		return ErrInvalidMetadata
	}
	return nil
}

type strictJSONParser struct {
	data       []byte
	position   int
	structures int
}

func (parser *strictJSONParser) parseValue(depth int) error {
	if depth > MaxJSONDepth || parser.consumeStructure() != nil {
		return ErrInvalidMetadata
	}
	parser.skipWhitespace()
	if parser.position >= len(parser.data) {
		return ErrInvalidMetadata
	}
	switch parser.data[parser.position] {
	case '{':
		return parser.parseObject(depth)
	case '[':
		return parser.parseArray(depth)
	case '"':
		_, err := parser.parseString()
		return err
	case 't':
		return parser.consumeLiteral("true")
	case 'f':
		return parser.consumeLiteral("false")
	case 'n':
		return parser.consumeLiteral("null")
	default:
		return parser.parseNumber()
	}
}

func (parser *strictJSONParser) parseObject(depth int) error {
	parser.position++
	parser.skipWhitespace()
	if parser.consumeByte('}') {
		return nil
	}
	seen := make(map[string]struct{})
	for {
		key, err := parser.parseString()
		if err != nil || parser.consumeStructure() != nil {
			return ErrInvalidMetadata
		}
		if _, duplicate := seen[key]; duplicate {
			return ErrInvalidMetadata
		}
		seen[key] = struct{}{}
		parser.skipWhitespace()
		if !parser.consumeByte(':') {
			return ErrInvalidMetadata
		}
		if err := parser.parseValue(depth + 1); err != nil {
			return err
		}
		parser.skipWhitespace()
		if parser.consumeByte('}') {
			return nil
		}
		if !parser.consumeByte(',') {
			return ErrInvalidMetadata
		}
		parser.skipWhitespace()
	}
}

func (parser *strictJSONParser) parseArray(depth int) error {
	parser.position++
	parser.skipWhitespace()
	if parser.consumeByte(']') {
		return nil
	}
	for {
		if parser.consumeStructure() != nil {
			return ErrInvalidMetadata
		}
		if err := parser.parseValue(depth + 1); err != nil {
			return err
		}
		parser.skipWhitespace()
		if parser.consumeByte(']') {
			return nil
		}
		if !parser.consumeByte(',') {
			return ErrInvalidMetadata
		}
	}
}

func (parser *strictJSONParser) parseString() (string, error) {
	parser.skipWhitespace()
	if !parser.consumeByte('"') {
		return "", ErrInvalidMetadata
	}
	start := parser.position - 1
	for parser.position < len(parser.data) {
		value := parser.data[parser.position]
		parser.position++
		switch value {
		case '"':
			var decoded string
			if err := json.Unmarshal(parser.data[start:parser.position], &decoded); err != nil || !safeText(decoded) {
				return "", ErrInvalidMetadata
			}
			return decoded, nil
		case '\\':
			if parser.position >= len(parser.data) {
				return "", ErrInvalidMetadata
			}
			escape := parser.data[parser.position]
			parser.position++
			if escape != 'u' {
				if !strings.ContainsRune(`"\\/bfnrt`, rune(escape)) {
					return "", ErrInvalidMetadata
				}
				continue
			}
			codeUnit, ok := parser.consumeHexCodeUnit()
			if !ok || codeUnit >= 0xDC00 && codeUnit <= 0xDFFF {
				return "", ErrInvalidMetadata
			}
			if codeUnit >= 0xD800 && codeUnit <= 0xDBFF {
				if parser.position+2 > len(parser.data) || parser.data[parser.position] != '\\' || parser.data[parser.position+1] != 'u' {
					return "", ErrInvalidMetadata
				}
				parser.position += 2
				low, ok := parser.consumeHexCodeUnit()
				if !ok || low < 0xDC00 || low > 0xDFFF {
					return "", ErrInvalidMetadata
				}
			}
		default:
			if value < 0x20 {
				return "", ErrInvalidMetadata
			}
		}
	}
	return "", ErrInvalidMetadata
}

func (parser *strictJSONParser) consumeHexCodeUnit() (uint16, bool) {
	if parser.position+4 > len(parser.data) {
		return 0, false
	}
	var value uint16
	for range 4 {
		value <<= 4
		character := parser.data[parser.position]
		parser.position++
		switch {
		case character >= '0' && character <= '9':
			value += uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value += uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value += uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func (parser *strictJSONParser) parseNumber() error {
	start := parser.position
	parser.consumeByte('-')
	if parser.consumeByte('0') {
		if parser.position < len(parser.data) && parser.data[parser.position] >= '0' && parser.data[parser.position] <= '9' {
			return ErrInvalidMetadata
		}
	} else if !parser.consumeDigits('1', '9') {
		return ErrInvalidMetadata
	}
	if parser.consumeByte('.') && !parser.consumeDigits('0', '9') {
		return ErrInvalidMetadata
	}
	if parser.position < len(parser.data) && (parser.data[parser.position] == 'e' || parser.data[parser.position] == 'E') {
		parser.position++
		if parser.position < len(parser.data) && (parser.data[parser.position] == '+' || parser.data[parser.position] == '-') {
			parser.position++
		}
		if !parser.consumeDigits('0', '9') {
			return ErrInvalidMetadata
		}
	}
	if parser.position == start {
		return ErrInvalidMetadata
	}
	return nil
}

func (parser *strictJSONParser) consumeDigits(first, last byte) bool {
	start := parser.position
	for parser.position < len(parser.data) && parser.data[parser.position] >= first && parser.data[parser.position] <= last {
		parser.position++
		first = '0'
	}
	return parser.position > start
}

func (parser *strictJSONParser) consumeLiteral(literal string) error {
	if !bytes.HasPrefix(parser.data[parser.position:], []byte(literal)) {
		return ErrInvalidMetadata
	}
	parser.position += len(literal)
	return nil
}

func (parser *strictJSONParser) consumeByte(value byte) bool {
	if parser.position >= len(parser.data) || parser.data[parser.position] != value {
		return false
	}
	parser.position++
	return true
}

func (parser *strictJSONParser) consumeStructure() error {
	parser.structures++
	if parser.structures > MaxJSONStructuralNodes {
		return ErrInvalidMetadata
	}
	return nil
}

func (parser *strictJSONParser) skipWhitespace() {
	for parser.position < len(parser.data) {
		switch parser.data[parser.position] {
		case ' ', '	', '\n', '\r':
			parser.position++
		default:
			return
		}
	}
}

func topLevelAryPokemonObjects(ast *jsparser.AST) []*jsparser.ObjectExpr {
	var objects []*jsparser.ObjectExpr
	for _, statement := range ast.List {
		switch node := statement.(type) {
		case *jsparser.VarDecl:
			for _, binding := range node.List {
				variable, ok := binding.Binding.(*jsparser.Var)
				if !ok || string(variable.Data) != "aryPokemon" {
					continue
				}
				if object, ok := binding.Default.(*jsparser.ObjectExpr); ok {
					objects = append(objects, object)
				} else {
					objects = append(objects, nil)
				}
			}
		case *jsparser.ExprStmt:
			assignment, ok := node.Value.(*jsparser.BinaryExpr)
			if !ok || assignment.Op != jsparser.EqToken {
				continue
			}
			variable, ok := assignment.X.(*jsparser.Var)
			if !ok || string(variable.Data) != "aryPokemon" {
				continue
			}
			if object, ok := assignment.Y.(*jsparser.ObjectExpr); ok {
				objects = append(objects, object)
			} else {
				objects = append(objects, nil)
			}
		}
	}
	return objects
}

type scriptToken struct {
	typeID     jsparser.TokenType
	aryPokemon bool
}

type objectCandidate struct {
	start       int
	braceDepth  int
	objectIndex int
}

func aryPokemonJSONObjects(source []byte) ([][]byte, error) {
	if len(source) > MaxExecutableScriptBytes {
		return nil, ErrInvalidMetadata
	}
	input := parse.NewInputBytes(source)
	lexer := jsparser.NewLexer(input)
	history := make([]scriptToken, 0, 3)
	var active []objectCandidate
	var objects [][]byte
	var completed []bool
	braceDepth := 0
	tokenCount := 0
	for {
		typeID, data := lexer.Next()
		if (typeID == jsparser.DivToken || typeID == jsparser.DivEqToken) && regexMayStartAfter(history) {
			typeID, data = lexer.RegExp()
		}
		if typeID == jsparser.ErrorToken {
			if errors.Is(lexer.Err(), io.EOF) {
				break
			}
			return nil, ErrInvalidMetadata
		}
		if typeID == jsparser.WhitespaceToken || typeID == jsparser.LineTerminatorToken || typeID == jsparser.CommentToken || typeID == jsparser.CommentLineTerminatorToken {
			continue
		}
		tokenCount++
		if tokenCount > MaxJavaScriptTokens {
			return nil, ErrInvalidMetadata
		}

		end := input.Offset()
		token := scriptToken{
			typeID:     typeID,
			aryPokemon: typeID == jsparser.IdentifierToken && bytes.Equal(data, []byte("aryPokemon")),
		}
		switch typeID {
		case jsparser.OpenBraceToken:
			braceDepth++
			if aryPokemonAssignmentStarts(history) {
				objectIndex := len(objects)
				objects = append(objects, nil)
				completed = append(completed, false)
				active = append(active, objectCandidate{start: end - len(data), braceDepth: braceDepth, objectIndex: objectIndex})
			}
		case jsparser.CloseBraceToken:
			for len(active) > 0 && active[len(active)-1].braceDepth == braceDepth {
				candidate := active[len(active)-1]
				active = active[:len(active)-1]
				objects[candidate.objectIndex] = source[candidate.start:end]
				completed[candidate.objectIndex] = true
			}
			braceDepth--
		}
		if len(history) == cap(history) {
			copy(history, history[1:])
			history = history[:len(history)-1]
		}
		history = append(history, token)
	}
	finished := objects[:0]
	for index, object := range objects {
		if completed[index] {
			finished = append(finished, object)
		}
	}
	return finished, nil
}

func aryPokemonAssignmentStarts(history []scriptToken) bool {
	if len(history) < 2 || !history[len(history)-2].aryPokemon || history[len(history)-1].typeID != jsparser.EqToken {
		return false
	}
	if len(history) < 3 {
		return true
	}
	previous := history[len(history)-3].typeID
	return previous != jsparser.DotToken && previous != jsparser.OptChainToken
}

func regexMayStartAfter(tokens []scriptToken) bool {
	if len(tokens) == 0 {
		return true
	}
	switch tokens[len(tokens)-1].typeID {
	case jsparser.IdentifierToken, jsparser.PrivateIdentifierToken,
		jsparser.StringToken, jsparser.TemplateToken, jsparser.TemplateEndToken, jsparser.RegExpToken,
		jsparser.IntegerToken, jsparser.DecimalToken, jsparser.HexadecimalToken, jsparser.OctalToken, jsparser.BinaryToken,
		jsparser.TrueToken, jsparser.FalseToken, jsparser.NullToken, jsparser.ThisToken,
		jsparser.CloseParenToken, jsparser.CloseBracketToken, jsparser.CloseBraceToken,
		jsparser.IncrToken, jsparser.DecrToken:
		return false
	default:
		return true
	}
}
