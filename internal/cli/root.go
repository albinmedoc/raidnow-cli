package cli

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/session"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const shortDescription = "Search for Raid NOW raids from your terminal"

var (
	Version      = "dev"
	BootstrapURL = session.DefaultBootstrapURL
)

type RootConfig struct {
	TokenFile      string
	RequestTimeout time.Duration
}

type RootOptions struct {
	Bootstrap TokenBootstrapper
	Publisher TrainerPublisher
	Config    *RootConfig
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(RootOptions{
		Bootstrap: session.NewBootstrap(http.DefaultClient, BootstrapURL),
		Publisher: defaultTrainerPublisher(),
		Config:    &RootConfig{},
	})
}

func NewRootCommandWithOptions(options RootOptions) *cobra.Command {
	if options.Bootstrap == nil {
		options.Bootstrap = session.NewBootstrap(http.DefaultClient, BootstrapURL)
	}
	config := RootConfig{}
	if options.Config != nil {
		config = *options.Config
	}
	if options.Publisher == nil {
		options.Publisher = defaultTrainerPublisher()
	}
	var jsonOutput bool
	command := &cobra.Command{
		Use:           "raidnow",
		Short:         shortDescription,
		Args:          usageNoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	command.SetFlagErrorFunc(func(command *cobra.Command, _ error) error {
		return finishInvocation(command, newUsageError())
	})
	command.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Write compact JSON output when supported")
	command.PersistentFlags().StringVar(&config.TokenFile, "token-file", config.TokenFile, "Read the anonymous token from path or - for stdin")
	command.PersistentFlags().DurationVar(&config.RequestTimeout, "request-timeout", config.RequestTimeout, "Override the upstream request timeout")
	command.RunE = withInvocationReset(func(command *cobra.Command, _ []string) error {
		return command.Help()
	})
	defaultHelp := command.HelpFunc()
	command.SetHelpFunc(func(command *cobra.Command, args []string) {
		defer resetInvocation(command.Root())
		defaultHelp(command, args)
	})
	command.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the raidnow version",
		Args:  usageNoArgs,
		RunE: withInvocationReset(func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(command.OutOrStdout(), "raidnow %s\n", Version)
			return err
		}),
	})
	command.AddCommand(newTokenCommand(options.Bootstrap, &jsonOutput))
	command.AddCommand(newTrainerCommand(options.Publisher, &config, &jsonOutput))
	return command
}

type invocationError struct {
	err        error
	jsonOutput bool
}

func (err *invocationError) Error() string { return err.err.Error() }
func (err *invocationError) Unwrap() error { return err.err }

func withInvocationReset(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) (err error) {
	return func(command *cobra.Command, args []string) (err error) {
		defer func() { err = finishInvocation(command, err) }()
		return run(command, args)
	}
}

func finishInvocation(command *cobra.Command, err error) error {
	root := command.Root()
	jsonOutput, _ := root.PersistentFlags().GetBool("json")
	resetInvocation(root)
	if err == nil {
		return nil
	}
	return &invocationError{err: err, jsonOutput: jsonOutput}
}

func resetInvocation(root *cobra.Command) {
	var resetFlags = func(flags *pflag.FlagSet) {
		flags.VisitAll(func(flag *pflag.Flag) {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		})
	}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		resetFlags(command.LocalNonPersistentFlags())
		resetFlags(command.PersistentFlags())
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
}

func ErrorUsesJSON(err error) bool {
	var invocation *invocationError
	return errors.As(err, &invocation) && invocation != nil && invocation.jsonOutput
}

func defaultTrainerPublisher() TrainerPublisher {
	publisher, err := upstream.NewClient(upstream.Config{})
	if err != nil {
		return nil
	}
	return publisher
}
