package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/session"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
)

type TrainerPublisher interface {
	PublishTrainer(context.Context, string, domain.Trainer, upstream.CallOptions) error
}

func newTrainerCommand(publisher TrainerPublisher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "trainer", Short: "Manage invocation-scoped trainer information", Args: usageNoArgs}
	command.RunE = func(command *cobra.Command, _ []string) error {
		return command.Help()
	}
	publish := &cobra.Command{
		Use:   "publish",
		Short: "Publish trainer information for this anonymous session",
		Args:  usageNoArgs,
	}
	trainerInput := bindTrainerInputFlags(publish)
	publish.RunE = func(command *cobra.Command, _ []string) error {
		if requestTimeout == nil || *requestTimeout < 0 || tokenFile == nil {
			return newUsageError()
		}
		if *tokenFile == "-" && trainerInput.codeFile == "-" {
			return newUsageError()
		}
		if publisher == nil {
			return errors.New("trainer publisher unavailable")
		}
		token, err := session.ReadTokenInput(*tokenFile, command.InOrStdin())
		if err != nil {
			return output.NewError(output.CodeInvalidInput, "Ogiltig tokeninmatning.")
		}
		trainer, err := trainerInput.read(command)
		if err != nil {
			return err
		}
		if err := publisher.PublishTrainer(command.Context(), token, trainer, upstream.CallOptions{Timeout: *requestTimeout}); err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("trainer publish", "published", struct {
				Published bool `json:"published"`
			}{Published: true}, nil))
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
		if strings.HasPrefix(public.Message, "Upstream avvisade begäran: ") {
			message = public.Message
		}
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
