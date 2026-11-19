package output

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const redacted = "[REDACTED]"

var trainerCodePattern = regexp.MustCompile(`[0-9]{12}|[0-9]{4}([ -][0-9]{4}){2}`)

func RenderError(destination io.Writer, err error, secrets ...string) error {
	publicError := PublicError(err)
	message := Redact(publicError.Message, secrets...)
	_, writeError := fmt.Fprintf(destination, "Fel [%s]: %s\n", publicError.Code, message)
	return writeError
}

func Redact(text string, secrets ...string) string {
	nonempty := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			nonempty = append(nonempty, secret)
		}
	}
	sort.Slice(nonempty, func(left, right int) bool {
		if len(nonempty[left]) == len(nonempty[right]) {
			return nonempty[left] < nonempty[right]
		}
		return len(nonempty[left]) > len(nonempty[right])
	})
	for _, secret := range nonempty {
		text = strings.ReplaceAll(text, secret, redacted)
	}
	return redactTrainerCodes(text)
}

func redactTrainerCodes(text string) string {
	matches := trainerCodePattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var result strings.Builder
	start := 0
	for _, match := range matches {
		left, right := match[0], match[1]
		if (left > 0 && isASCIIAlphanumeric(text[left-1])) || (right < len(text) && isASCIIAlphanumeric(text[right])) {
			continue
		}
		result.WriteString(text[start:left])
		result.WriteString(redacted)
		start = right
	}
	if start == 0 {
		return text
	}
	result.WriteString(text[start:])
	return result.String()
}

func isASCIIAlphanumeric(value byte) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= 'a' && value <= 'z')
}
