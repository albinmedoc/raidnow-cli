package session

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tdewolff/parse/v2"
	jsparser "github.com/tdewolff/parse/v2/js"
	"golang.org/x/net/html"
)

func extractToken(body []byte) (token string, err error) {
	defer func() {
		if recover() != nil {
			token = ""
			err = errBootstrap
		}
	}()

	document, parseErr := html.Parse(bytes.NewReader(body))
	if parseErr != nil {
		return "", errBootstrap
	}

	var candidates []string
	if collectErr := collectScriptCandidates(document, false, &candidates); collectErr != nil {
		return "", errBootstrap
	}
	if len(candidates) != 1 || !tokenIsValid(candidates[0]) {
		return "", errBootstrap
	}
	return candidates[0], nil
}

func collectScriptCandidates(node *html.Node, inTemplate bool, candidates *[]string) error {
	if node.Type == html.ElementNode && node.Data == "template" {
		inTemplate = true
	}
	if !inTemplate && node.Type == html.ElementNode && node.Data == "script" && executableInlineScript(node) {
		var source strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode {
				source.WriteString(child.Data)
			}
		}
		found, err := candidatesFromScript([]byte(source.String()))
		if err != nil {
			return errBootstrap
		}
		*candidates = append(*candidates, found...)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := collectScriptCandidates(child, inTemplate, candidates); err != nil {
			return err
		}
	}
	return nil
}

func executableInlineScript(node *html.Node) bool {
	scriptType := ""
	for _, attribute := range node.Attr {
		switch attribute.Key {
		case "src":
			return false
		case "type":
			scriptType = strings.ToLower(strings.TrimSpace(attribute.Val))
		}
	}
	switch scriptType {
	case "", "text/javascript", "application/javascript", "module":
		return true
	default:
		return false
	}
}

func candidatesFromScript(source []byte) ([]string, error) {
	ast, err := jsparser.Parse(parse.NewInputBytes(source), jsparser.Options{})
	if err != nil {
		return nil, errBootstrap
	}
	visitor := &vueCandidateVisitor{}
	jsparser.Walk(visitor, ast)
	if visitor.err != nil {
		return nil, errBootstrap
	}
	return visitor.tokens, nil
}

type vueCandidateVisitor struct {
	tokens []string
	err    error
}

func (visitor *vueCandidateVisitor) Enter(node jsparser.INode) jsparser.IVisitor {
	if visitor.err != nil {
		return nil
	}
	newExpression, ok := node.(*jsparser.NewExpr)
	if !ok {
		return visitor
	}
	token, candidate, err := tokenFromVueExpression(newExpression)
	if err != nil {
		visitor.err = err
		return nil
	}
	if candidate {
		visitor.tokens = append(visitor.tokens, token)
	}
	return visitor
}

func (visitor *vueCandidateVisitor) Exit(jsparser.INode) {}

func tokenFromVueExpression(expression *jsparser.NewExpr) (string, bool, error) {
	callee, ok := expression.X.(*jsparser.Var)
	if !ok || string(callee.Data) != "Vue" || expression.Args == nil || len(expression.Args.List) != 1 || expression.Args.List[0].Rest {
		return "", false, nil
	}
	config, ok := expression.Args.List[0].Value.(*jsparser.ObjectExpr)
	if !ok {
		return "", false, nil
	}
	dataValue, count, valid := directStaticProperty(config, "data")
	if !valid || count != 1 {
		return "", false, nil
	}
	data, ok := dataValue.(*jsparser.ObjectExpr)
	if !ok {
		return "", false, nil
	}
	tokenValue, count, valid := directStaticProperty(data, "gofriend_token")
	if !valid || count != 1 {
		return "", false, nil
	}
	literal, ok := tokenValue.(*jsparser.LiteralExpr)
	if !ok || literal.TokenType != jsparser.StringToken {
		return "", false, nil
	}
	token, err := decodeJSStringLiteral(literal.Data)
	if err != nil {
		return "", false, errBootstrap
	}
	return token, true, nil
}

func directStaticProperty(object *jsparser.ObjectExpr, wanted string) (jsparser.IExpr, int, bool) {
	var value jsparser.IExpr
	count := 0
	for _, property := range object.List {
		if property.Spread || property.Name == nil || property.Name.IsComputed() || property.Init != nil {
			return nil, 0, false
		}
		if _, method := property.Value.(*jsparser.MethodDecl); method {
			return nil, 0, false
		}
		name, ok := staticPropertyName(property.Name)
		if !ok {
			return nil, 0, false
		}
		if name == wanted {
			count++
			value = property.Value
		}
	}
	return value, count, true
}

func staticPropertyName(name *jsparser.PropertyName) (string, bool) {
	switch name.Literal.TokenType {
	case jsparser.IdentifierToken:
		return string(name.Literal.Data), true
	case jsparser.StringToken:
		decoded, err := decodeJSStringLiteral(name.Literal.Data)
		return decoded, err == nil
	default:
		return "", false
	}
}

func decodeJSStringLiteral(literal []byte) (string, error) {
	if len(literal) < 2 || literal[0] != literal[len(literal)-1] || literal[0] != '\'' && literal[0] != '"' {
		return "", errBootstrap
	}
	quote := literal[0]
	input := literal[1 : len(literal)-1]
	var decoded strings.Builder
	for index := 0; index < len(input); {
		if input[index] != '\\' {
			r, size := utf8.DecodeRune(input[index:])
			if r == utf8.RuneError && size == 1 || r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' {
				return "", errBootstrap
			}
			decoded.WriteRune(r)
			index += size
			continue
		}
		index++
		if index >= len(input) {
			return "", errBootstrap
		}
		switch input[index] {
		case '\n':
			index++
			continue
		case '\r':
			index++
			if index < len(input) && input[index] == '\n' {
				index++
			}
			continue
		case '\'', '"', '\\', '/':
			decoded.WriteByte(input[index])
			index++
		case 'b':
			decoded.WriteByte('\b')
			index++
		case 'f':
			decoded.WriteByte('\f')
			index++
		case 'n':
			decoded.WriteByte('\n')
			index++
		case 'r':
			decoded.WriteByte('\r')
			index++
		case 't':
			decoded.WriteByte('\t')
			index++
		case 'v':
			decoded.WriteByte('\v')
			index++
		case '0':
			if index+1 < len(input) && input[index+1] >= '0' && input[index+1] <= '9' {
				return "", errBootstrap
			}
			decoded.WriteByte(0)
			index++
		case 'x':
			value, next, err := fixedHex(input, index+1, 2)
			if err != nil {
				return "", errBootstrap
			}
			decoded.WriteRune(rune(value))
			index = next
		case 'u':
			r, next, err := decodeUnicodeEscape(input, index+1)
			if err != nil {
				return "", errBootstrap
			}
			if utf16.IsSurrogate(r) {
				if r < 0xD800 || r > 0xDBFF || next+2 > len(input) || input[next] != '\\' || input[next+1] != 'u' {
					return "", errBootstrap
				}
				low, afterLow, lowErr := decodeUnicodeEscape(input, next+2)
				if lowErr != nil || low < 0xDC00 || low > 0xDFFF {
					return "", errBootstrap
				}
				r = utf16.DecodeRune(r, low)
				next = afterLow
			}
			decoded.WriteRune(r)
			index = next
		default:
			if bytes.HasPrefix(input[index:], []byte{0xE2, 0x80, 0xA8}) || bytes.HasPrefix(input[index:], []byte{0xE2, 0x80, 0xA9}) {
				index += 3
				continue
			}
			if input[index] >= '1' && input[index] <= '9' || input[index] == quote {
				return "", errBootstrap
			}
			return "", errBootstrap
		}
		if decoded.Len() > MaxTokenBytes {
			return "", errBootstrap
		}
	}
	return decoded.String(), nil
}

func decodeUnicodeEscape(input []byte, start int) (rune, int, error) {
	if start < len(input) && input[start] == '{' {
		end := bytes.IndexByte(input[start+1:], '}')
		if end < 0 || end == 0 || end > 6 {
			return 0, start, errBootstrap
		}
		end += start + 1
		value, err := strconv.ParseUint(string(input[start+1:end]), 16, 32)
		if err != nil || value > utf8.MaxRune || value >= 0xD800 && value <= 0xDFFF {
			return 0, start, errBootstrap
		}
		return rune(value), end + 1, nil
	}
	value, next, err := fixedHex(input, start, 4)
	return rune(value), next, err
}

func fixedHex(input []byte, start, length int) (uint64, int, error) {
	end := start + length
	if end > len(input) {
		return 0, start, errBootstrap
	}
	value, err := strconv.ParseUint(string(input[start:end]), 16, 32)
	if err != nil {
		return 0, start, errBootstrap
	}
	return value, end, nil
}
