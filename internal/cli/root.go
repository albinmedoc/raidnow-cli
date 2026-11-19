package cli

import (
	"fmt"
	"net/http"

	"github.com/albinmedoc/raidnow-cli/internal/session"
	"github.com/spf13/cobra"
)

const shortDescription = "Search for Raid NOW raids from your terminal"

var (
	Version      = "dev"
	BootstrapURL = session.DefaultBootstrapURL
)

type RootOptions struct {
	Bootstrap TokenBootstrapper
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(RootOptions{
		Bootstrap: session.NewBootstrap(http.DefaultClient, BootstrapURL),
	})
}

func NewRootCommandWithOptions(options RootOptions) *cobra.Command {
	if options.Bootstrap == nil {
		options.Bootstrap = session.NewBootstrap(http.DefaultClient, BootstrapURL)
	}
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "raidnow",
		Short: shortDescription,
		Args:  cobra.NoArgs,
	}
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Write compact JSON output when supported")
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
	command.AddCommand(newTokenCommand(options.Bootstrap, &jsonOutput))
	return command
}
