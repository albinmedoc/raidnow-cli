package main

import (
	"io"
	"os"
	"strings"

	"github.com/albinmedoc/raidnow-cli/internal/cli"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	return runCommand(arguments, stdout, stderr, cli.NewRootCommand())
}

func runWithOptions(arguments []string, stdout, stderr io.Writer, options cli.RootOptions) int {
	return runCommand(arguments, stdout, stderr, cli.NewRootCommandWithOptions(options))
}

func runCommand(arguments []string, stdout, stderr io.Writer, command *cobra.Command) int {
	command.SetArgs(arguments)
	command.SetOut(stdout)
	command.SetErr(stderr)
	executed, err := command.ExecuteC()
	if err == nil {
		return output.ExitSuccess
	}
	jsonOutput, _ := command.PersistentFlags().GetBool("json")
	if jsonOutput {
		if renderError := output.RenderJSON(stdout, output.NewFailure(commandName(executed, command), "failed", err)); renderError != nil {
			return 1
		}
	} else if renderError := output.RenderError(stderr, err); renderError != nil {
		return 1
	}
	return output.ExitCodeFor(err)
}

func commandName(executed, root *cobra.Command) string {
	if executed == nil {
		executed = root
	}
	return strings.TrimPrefix(executed.CommandPath(), root.Name()+" ")
}
