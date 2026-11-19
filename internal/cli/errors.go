package cli

import (
	"errors"

	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/spf13/cobra"
)

const invalidArgumentsMessage = "Ogiltiga kommandoradsargument."

type UsageError struct {
	Err *output.CLIError
}

func (err *UsageError) Error() string {
	return err.Err.Error()
}

func (err *UsageError) Unwrap() error {
	return err.Err
}

func newUsageError() error {
	return &UsageError{Err: output.NewError(output.CodeInvalidArguments, invalidArgumentsMessage)}
}

func usageNoArgs(command *cobra.Command, args []string) error {
	if len(args) != 0 {
		return finishInvocation(command, newUsageError())
	}
	return nil
}

type CommandError struct {
	Command string
	Err     error
}

func (err *CommandError) Error() string {
	return err.Err.Error()
}

func (err *CommandError) Unwrap() error {
	return err.Err
}

func ErrorCommand(err error) string {
	var commandError *CommandError
	if errors.As(err, &commandError) && commandError != nil {
		return commandError.Command
	}
	return "raidnow"
}
