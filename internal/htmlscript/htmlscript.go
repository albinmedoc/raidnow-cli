// Package htmlscript selects executable inline JavaScript from HTML.
package htmlscript

import (
	"bytes"
	"mime"
	"strings"

	"golang.org/x/net/html"
)

// Sources returns executable inline script bodies outside templates.
func Sources(body []byte) ([][]byte, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var sources [][]byte
	collect(document, false, &sources)
	return sources, nil
}

func collect(node *html.Node, inTemplate bool, sources *[][]byte) {
	if node.Type == html.ElementNode && node.Data == "template" {
		inTemplate = true
	}
	if !inTemplate && node.Type == html.ElementNode && node.Namespace == "" && node.Data == "script" && executableInline(node) {
		var source strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode {
				source.WriteString(child.Data)
			}
		}
		*sources = append(*sources, []byte(source.String()))
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collect(child, inTemplate, sources)
	}
}

func executableInline(node *html.Node) bool {
	scriptType := ""
	for _, attribute := range node.Attr {
		switch attribute.Key {
		case "src", "href":
			return false
		case "type":
			scriptType = strings.TrimSpace(attribute.Val)
		}
	}
	if scriptType == "" || strings.EqualFold(scriptType, "module") {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(scriptType)
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "text/javascript", "application/javascript", "text/ecmascript", "application/ecmascript":
		return true
	default:
		return false
	}
}
