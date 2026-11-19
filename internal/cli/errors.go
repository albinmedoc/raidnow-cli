package cli

import (
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/spf13/cobra"
)

const invalidArgumentsMessage = "Ogiltiga kommandoradsargument."

func newUsageError() error {
	return output.NewError(output.CodeInvalidArguments, invalidArgumentsMessage)
}

func usageNoArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return newUsageError()
	}
	return nil
}
