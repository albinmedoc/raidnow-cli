package cli

import (
	"context"
	"encoding/json"
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
		Args:  cobra.NoArgs,
	}
	command.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Fetch a new anonymous token",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			token, err := bootstrap.Token(command.Context())
			if err != nil {
				command.Root().SilenceErrors = true
				command.Root().SilenceUsage = true
				publicError := output.NewError(output.CodeReadNetwork, "Kunde inte hämta en anonym token från Raid NOW.")
				if renderError := output.NewHumanRenderer(command.ErrOrStderr()).Error(publicError); renderError != nil {
					return output.NewError(output.CodeInternal, "Ett oväntat internt fel inträffade.")
				}
				return publicError
			}
			if *jsonOutput {
				return json.NewEncoder(command.OutOrStdout()).Encode(struct {
					Token string `json:"token"`
				}{Token: token})
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), token)
			return err
		},
	})
	return command
}
