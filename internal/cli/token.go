package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/spf13/cobra"
)

type TokenBootstrapper interface {
	Token(context.Context) (string, error)
}

func newTokenCommand(bootstrap TokenBootstrapper, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{
		Use:   "token",
		Short: "Manage anonymous Raid NOW tokens",
		Args:  usageNoArgs,
	}
	command.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Fetch a new anonymous token",
		Args:  usageNoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			token, err := bootstrap.Token(command.Context())
			if err != nil {
				code := output.CodeReadNetwork
				var typed *output.CLIError
				if errors.As(err, &typed) && typed != nil && output.ExitCodeFor(typed) != 1 {
					code = typed.Code
				}
				safeError := output.NewError(code, "Kunde inte hämta en anonym token från Raid NOW.")
				return &CommandError{Command: "token get", Err: safeError}
			}
			if *jsonOutput {
				return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("token get", "secret", struct {
					Token          string `json:"token"`
					ContainsSecret bool   `json:"containsSecret"`
				}{Token: token, ContainsSecret: true}, []string{"Tokenen är hemlig; lagra och dela den säkert."}, ""))
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), token)
			return err
		},
	})
	return command
}
