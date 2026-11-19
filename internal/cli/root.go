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

type RootConfig struct {
	TokenFile string
}

type RootOptions struct {
	Bootstrap TokenBootstrapper
	Config    *RootConfig
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(RootOptions{
		Bootstrap: session.NewBootstrap(http.DefaultClient, BootstrapURL),
		Config:    &RootConfig{},
	})
}

func NewRootCommandWithOptions(options RootOptions) *cobra.Command {
	if options.Bootstrap == nil {
		options.Bootstrap = session.NewBootstrap(http.DefaultClient, BootstrapURL)
	}
	if options.Config == nil {
		options.Config = &RootConfig{}
	}
	var jsonOutput bool
	command := &cobra.Command{
		Use:           "raidnow",
		Short:         shortDescription,
		Args:          usageNoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	command.SetFlagErrorFunc(func(*cobra.Command, error) error {
		return newUsageError()
	})
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Write compact JSON output when supported")
	command.PersistentFlags().StringVar(&options.Config.TokenFile, "token-file", options.Config.TokenFile, "Read the anonymous token from path or - for stdin")
	command.RunE = func(command *cobra.Command, _ []string) error {
		return command.Help()
	}
	command.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the raidnow version",
		Args:  usageNoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(command.OutOrStdout(), "raidnow %s\n", Version)
			return err
		},
	})
	command.AddCommand(newTokenCommand(options.Bootstrap, &jsonOutput))
	return command
}
