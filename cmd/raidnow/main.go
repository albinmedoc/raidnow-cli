package main

import (
	"io"
	"os"

	"github.com/albinmedoc/raidnow-cli/internal/cli"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	command := cli.NewRootCommand()
	command.SetArgs(arguments)
	command.SetOut(stdout)
	command.SetErr(stderr)
	err := command.Execute()
	if err == nil {
		return output.ExitSuccess
	}
	if output.ExitCodeFor(err) == 1 {
		err = output.NewError(output.CodeInvalidArguments, "Ogiltiga kommandoradsargument.")
	}
	jsonOutput, _ := command.PersistentFlags().GetBool("json")
	if jsonOutput {
		if renderError := output.RenderJSON(stdout, output.NewFailure(cli.ErrorCommand(err), "failed", err, "")); renderError != nil {
			return 1
		}
	} else if renderError := output.NewHumanRenderer(stderr).Error(err); renderError != nil {
		return 1
	}
	return output.ExitCodeFor(err)
}
