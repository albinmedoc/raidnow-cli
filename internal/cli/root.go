package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const shortDescription = "Search for Raid NOW raids from your terminal"

var Version = "dev"

func NewRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "raidnow",
		Short: shortDescription,
	}
	command.RunE = func(command *cobra.Command, _ []string) error {
		return command.Help()
	}
	command.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the raidnow version",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(command.OutOrStdout(), "raidnow %s\n", Version)
			return err
		},
	})
	return command
}
