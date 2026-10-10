package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// subcommandWithFlags returns a stand-in for one of newApp()'s subcommands that
// carries that subcommand's real flag set, so the test exercises the
// Validator newApp() actually wires up, not a copy of it.
func subcommandWithFlags(t *testing.T, name string) *cli.Command {
	t.Helper()
	for _, c := range newApp().Commands {
		if c.Name == name {
			return &cli.Command{
				Name:   "syver",
				Flags:  c.Flags,
				Action: func(context.Context, *cli.Command) error { return nil },
			}
		}
	}
	t.Fatalf("newApp() has no %q subcommand", name)
	return nil
}

// --max-concurrent 0 used to start no worker: a failing spec reported
// "Count: 0" and exited 0. Both subcommands that take the flag must now refuse
// a value below 1, from the command line and from either environment variable.
//
// Revert-proof: remove `Validator: syver.ValidateMaxConcurrent` from either
// flag in newApp() and that subcommand's rows fail.
func TestMaxConcurrentBelowOneIsRejected(t *testing.T) {
	for _, sub := range []string{"validate", "serve"} {
		for _, v := range []string{"0", "-3"} {
			err := subcommandWithFlags(t, sub).Run(context.Background(), []string{"syver", "--max-concurrent", v})
			require.Error(t, err, "%s --max-concurrent %s must be rejected", sub, v)
			require.Contains(t, err.Error(), "max-concurrent")
		}
		for _, env := range []string{"SYVER_MAX_CONCURRENT", "GOSS_MAX_CONCURRENT"} {
			t.Run(sub+"/"+env, func(t *testing.T) {
				t.Setenv(env, "0")
				err := subcommandWithFlags(t, sub).Run(context.Background(), []string{"syver"})
				require.Error(t, err, "%s with %s=0 must be rejected", sub, env)
			})
		}
		// The control: the default and an ordinary value still parse.
		require.NoError(t, subcommandWithFlags(t, sub).Run(context.Background(), []string{"syver"}))
		require.NoError(t, subcommandWithFlags(t, sub).Run(context.Background(), []string{"syver", "--max-concurrent", "1"}))
	}
}
