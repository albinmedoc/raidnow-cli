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

type trainerInputFlags struct {
	name     string
	level    int
	country  string
	codeFile string
}

func bindTrainerInputFlags(command *cobra.Command) *trainerInputFlags {
	input := &trainerInputFlags{}
	flags := command.Flags()
	flags.StringVar(&input.name, "trainer-name", "", "Trainer name for this invocation")
	flags.IntVar(&input.level, "trainer-level", 0, "Trainer level for this invocation")
	flags.StringVar(&input.country, "trainer-country", "", "Trainer country code for this invocation")
	flags.StringVar(&input.codeFile, "trainer-code-file", "", "Read trainer code from protected path or - for stdin")
	return input
}

func (input *trainerInputFlags) read(command *cobra.Command) (domain.Trainer, error) {
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
