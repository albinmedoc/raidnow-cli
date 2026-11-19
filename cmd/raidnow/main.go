package main

import (
	"os"

	"github.com/albinmedoc/raidnow-cli/internal/cli"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(output.ExitCodeFor(err))
	}
}
