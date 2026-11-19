package cli

import "errors"

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
