package cli

import (
	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/secureinput"
	"github.com/spf13/cobra"
)

const (
	maxTrainerCodeInputBytes   = 128
	invalidTrainerInputMessage = "Ogiltig tränarinmatning."
)

// TrainerInputFlags owns one command's trainer flag values. Bind a fresh value
// to each future command that needs trainer identity; no input is read at bind time.
type TrainerInputFlags struct {
	name     string
	level    int
	country  string
	codeFile string
}

func BindTrainerInputFlags(command *cobra.Command) *TrainerInputFlags {
	input := &TrainerInputFlags{}
	flags := command.Flags()
	flags.StringVar(&input.name, "trainer-name", "", "Trainer name for this invocation")
	flags.IntVar(&input.level, "trainer-level", 0, "Trainer level for this invocation")
	flags.StringVar(&input.country, "trainer-country", "", "Trainer country code for this invocation")
	flags.StringVar(&input.codeFile, "trainer-code-file", "", "Read trainer code from protected path or - for stdin")
	return input
}

func (input *TrainerInputFlags) Read(command *cobra.Command) (domain.Trainer, error) {
	if input == nil || command == nil {
		return domain.Trainer{}, invalidTrainerInputError()
	}
	code, err := secureinput.Read(input.codeFile, command.InOrStdin(), maxTrainerCodeInputBytes)
	if err != nil {
		return domain.Trainer{}, invalidTrainerInputError()
	}
	trainer, err := domain.NewTrainer(input.name, string(code), input.level, input.country)
	if err != nil {
		return domain.Trainer{}, invalidTrainerInputError()
	}
	return trainer, nil
}

func invalidTrainerInputError() error {
	return output.NewError(output.CodeInvalidInput, invalidTrainerInputMessage)
}
