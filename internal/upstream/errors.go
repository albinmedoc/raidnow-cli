package upstream

import "github.com/albinmedoc/raidnow-cli/internal/output"

func invalidArguments(message string) error {
	return output.NewError(output.CodeInvalidArguments, message)
}

func readNetworkFailure() error {
	return output.NewError(output.CodeReadNetwork, "Kunde inte läsa svar från upstream.")
}

func writeOutcomeUnknown() error {
	return output.NewError(output.CodeWriteOutcomeUnknown, "Upstream-anslutningen bröts och skrivningens utfall är okänt.")
}

func upstreamRejection() error {
	return output.NewError(output.CodeUpstreamRejection, "Upstream avvisade begäran eller returnerade ett ogiltigt svar.")
}

func transportFailure(kind OperationKind, dispatched bool) error {
	if kind == WriteOperation && dispatched {
		return writeOutcomeUnknown()
	}
	return readNetworkFailure()
}
