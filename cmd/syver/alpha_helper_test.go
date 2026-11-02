package main

import (
	"testing"

	"github.com/urfave/cli/v3"
)

// newRunnableApp builds the app as main() does, including the use-alpha flag
// that main adds on macOS and Windows, and opts in to it. Without both, any
// test that reaches a subcommand action on those platforms hits
// fatalAlphaIfNeeded, which calls os.Exit and ends the whole test binary.
// The flag is not part of newApp(), so the environment variable alone does
// nothing.
func newRunnableApp(t *testing.T) *cli.Command {
	t.Helper()
	t.Setenv("SYVER_USE_ALPHA", "1")
	app := newApp()
	addAlphaFlagIfNeeded(app)
	return app
}
