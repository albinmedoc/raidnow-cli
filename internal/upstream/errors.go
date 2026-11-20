package upstream

import (
	"strings"
	"unicode"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func invalidArguments(message string) error {
	return output.NewError(output.CodeInvalidArguments, message)
}

func readNetworkFailure() error {
	return output.NewError(output.CodeReadNetwork, "Kunde inte läsa svar från upstream.")
}

func writeOutcomeUnknown() error {
	return output.NewError(output.CodeWriteOutcomeUnknown, "Upstream-anslutningen bröts och skrivningens utfall är okänt.")
}

func authenticationUnsupported() error {
	return output.NewError(output.CodeAuthenticationUnsupported, "Den anonyma upstream-sessionen har upphört och måste hämtas på nytt.")
}

func upstreamRejection() error {
	return output.NewError(output.CodeUpstreamRejection, "Upstream avvisade begäran eller returnerade ett ogiltigt svar.")
}

func upstreamRejectionDetail(text string, secrets ...string) error {
	detail := strings.TrimSpace(strings.Map(func(value rune) rune {
		if unicode.IsControl(value) {
			return ' '
		}
		return value
	}, text))
	detail = strings.TrimSpace(output.Redact(detail, secrets...))
	runes := []rune(detail)
	if len(runes) > 256 {
		detail = string(runes[:256])
	}
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return upstreamRejection()
	}
	return output.NewError(output.CodeUpstreamRejection, "Upstream avvisade begäran: "+detail)
}

func transportFailure(kind OperationKind, dispatched bool) error {
	if kind == WriteOperation && dispatched {
		return writeOutcomeUnknown()
	}
	return readNetworkFailure()
}
