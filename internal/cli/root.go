package cli

import (
	"fmt"
	"net/http"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/metadata"
	"github.com/albinmedoc/raidnow-cli/internal/session"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
)

const shortDescription = "Use Raid NOW anonymously from your terminal"

var (
	Version      = "dev"
	BootstrapURL = session.DefaultBootstrapURL
)

type RootOptions struct {
	Bootstrap      TokenBootstrapper
	Publisher      TrainerPublisher
	Operations     RaidOperations
	Metadata       MetadataFetcher
	TokenFile      string
	RequestTimeout time.Duration
}

func NewRootCommand() *cobra.Command {
	client := defaultUpstreamClient()
	return NewRootCommandWithOptions(RootOptions{
		Bootstrap:  session.NewBootstrap(http.DefaultClient, BootstrapURL),
		Publisher:  client,
		Operations: client,
		Metadata:   metadata.NewHomepageFetcher(http.DefaultClient, BootstrapURL),
	})
}

func NewRootCommandWithOptions(options RootOptions) *cobra.Command {
	if options.Bootstrap == nil {
		options.Bootstrap = session.NewBootstrap(http.DefaultClient, BootstrapURL)
	}
	if options.Publisher == nil {
		options.Publisher = defaultTrainerPublisher()
	}
	if options.Operations == nil {
		options.Operations = defaultUpstreamClient()
	}
	if options.Metadata == nil {
		options.Metadata = metadata.NewHomepageFetcher(http.DefaultClient, BootstrapURL)
	}
	jsonOutput := false
	tokenFile := options.TokenFile
	requestTimeout := options.RequestTimeout
	command := &cobra.Command{
		Use:           "raidnow",
		Short:         shortDescription,
		Args:          usageNoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	command.SetFlagErrorFunc(func(_ *cobra.Command, _ error) error {
		return newUsageError()
	})
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Write compact JSON output when supported")
	command.PersistentFlags().StringVar(&tokenFile, "token-file", tokenFile, "Read the anonymous token from path or - for stdin")
	command.PersistentFlags().DurationVar(&requestTimeout, "request-timeout", requestTimeout, "Override the upstream request timeout")
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
	command.AddCommand(newTrainerCommand(options.Publisher, &tokenFile, &requestTimeout, &jsonOutput))
	command.AddCommand(newQueueCommand(options.Operations, &tokenFile, &requestTimeout, &jsonOutput))
	command.AddCommand(newSearchCommand(options.Operations, &tokenFile, &requestTimeout, &jsonOutput))
	command.AddCommand(newBossesCommand(options.Operations, options.Metadata, &tokenFile, &requestTimeout, &jsonOutput))
	command.AddCommand(newLobbiesCommand(options.Operations, options.Metadata, &tokenFile, &requestTimeout, &jsonOutput))
	command.AddCommand(newRaidCommand(options.Operations, options.Metadata, &tokenFile, &requestTimeout, &jsonOutput))
	return command
}

func defaultTrainerPublisher() TrainerPublisher {
	return defaultUpstreamClient()
}

func defaultUpstreamClient() *upstream.Client {
	client, err := upstream.NewClient(upstream.Config{})
	if err != nil {
		return nil
	}
	return client
}
