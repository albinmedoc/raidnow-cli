package cli

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/session"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
)

type TrainerPublisher interface {
	PublishTrainer(context.Context, string, domain.Trainer, upstream.CallOptions) error
}

func newTrainerCommand(publisher TrainerPublisher, config *RootConfig, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "trainer", Short: "Manage invocation-scoped trainer information", Args: usageNoArgs}
	publish := &cobra.Command{
		Use:   "publish",
		Short: "Publish trainer information for this anonymous session",
		Args:  usageNoArgs,
	}
	trainerInput := BindTrainerInputFlags(publish)
	publish.RunE = func(command *cobra.Command, _ []string) error {
		if config == nil || config.RequestTimeout < 0 {
			return newUsageError()
		}
		if config.TokenFile == "-" && trainerInput.codeFile == "-" {
			return newUsageError()
		}
		if isNilInterface(publisher) {
			return &CommandError{Command: "trainer publish", Err: errors.New("trainer publisher unavailable")}
		}
		token, err := session.ReadTokenInput(config.TokenFile, command.InOrStdin())
		if err != nil {
			return &CommandError{Command: "trainer publish", Err: output.NewError(output.CodeInvalidInput, "Ogiltig tokeninmatning.")}
		}
		trainer, err := trainerInput.Read(command)
		if err != nil {
			return &CommandError{Command: "trainer publish", Err: err}
		}
		if err := publisher.PublishTrainer(command.Context(), token, trainer, upstream.CallOptions{Timeout: config.RequestTimeout}); err != nil {
			return &CommandError{Command: "trainer publish", Err: sanitizedPublisherError(err)}
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("trainer publish", "published", struct {
				Published bool `json:"published"`
			}{Published: true}, nil, ""))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Tränaruppgifterna publicerades.")
		return err
	}
	command.AddCommand(publish)
	return command
}

func sanitizedPublisherError(err error) error {
	public := output.PublicError(err)
	message := "Ett oväntat internt fel inträffade."
	switch public.Code {
	case output.CodeAuthenticationUnsupported:
		message = "Den anonyma upstream-sessionen har upphört och måste hämtas på nytt."
	case output.CodeUpstreamRejection:
		message = "Upstream avvisade begäran eller returnerade ett ogiltigt svar."
	case output.CodeReadNetwork:
		message = "Kunde inte läsa svar från upstream."
	case output.CodeWriteOutcomeUnknown:
		message = "Upstream-anslutningen bröts och skrivningens utfall är okänt."
	case output.CodeInvalidArguments:
		message = "Ogiltiga kommandoradsargument."
	case output.CodeInvalidInput:
		message = "Ogiltig tränarinmatning."
	}
	return output.NewError(public.Code, message)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
