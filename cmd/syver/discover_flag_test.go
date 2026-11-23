package main

import (
	"context"
	"testing"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// runtimeConfigFor parses args with the named subcommand's real flag set and
// returns the util.Config newRuntimeConfigFromCLI builds from them.
func runtimeConfigFor(t *testing.T, name string, args ...string) (*util.Config, error) {
	t.Helper()
	var cfg *util.Config
	for _, c := range newApp().Commands {
		if c.Name != name {
			continue
		}
		cmd := &cli.Command{
			Name:  "syver",
			Flags: c.Flags,
			Action: func(_ context.Context, c *cli.Command) error {
				cfg = newRuntimeConfigFromCLI(c)
				return nil
			},
		}
		err := cmd.Run(context.Background(), append([]string{"syver"}, args...))
		return cfg, err
	}
	t.Fatalf("newApp() has no %q subcommand", name)
	return nil, nil
}

// validate, serve and render all take --discover, from the flag or either
// environment variable. Only render takes --no-discover.
func TestDiscoverFlagsPerCommand(t *testing.T) {
	for _, sub := range []string{"validate", "serve", "render"} {
		cfg, err := runtimeConfigFor(t, sub, "--discover", "d.yml")
		require.NoError(t, err, sub)
		require.Equal(t, "d.yml", cfg.DiscoverSpec, sub)

		for _, env := range []string{"SYVER_DISCOVER", "GOSS_DISCOVER"} {
			t.Run(sub+"/"+env, func(t *testing.T) {
				t.Setenv(env, "e.yml")
				cfg, err := runtimeConfigFor(t, sub)
				require.NoError(t, err)
				require.Equal(t, "e.yml", cfg.DiscoverSpec)
			})
		}
	}

	cfg, err := runtimeConfigFor(t, "render", "--no-discover")
	require.NoError(t, err)
	require.True(t, cfg.NoDiscover)

	for _, sub := range []string{"validate", "serve"} {
		_, err := runtimeConfigFor(t, sub, "--no-discover")
		require.Error(t, err, "%s must not accept --no-discover", sub)
	}
}
