package main

import (
	"context"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// runCapturingConfig runs newApp() with args, replacing the validate and serve
// actions so they only record the config they would have run with.
func runCapturingConfig(t *testing.T, args ...string) *util.Config {
	t.Helper()
	app := newRunnableApp(t)
	var got *util.Config
	for _, c := range app.Commands {
		if c.Name == "validate" || c.Name == "serve" {
			c.Action = func(_ context.Context, c *cli.Command) error {
				got = newRuntimeConfigFromCLI(c)
				return nil
			}
		}
	}
	require.NoError(t, app.Run(context.Background(), append([]string{"syver"}, args...)), "args %q", args)
	require.NotNil(t, got, "no action ran for %q", args)
	return got
}

// Serve's -l is --listen-addr, and the root log-level flag also carried an "l"
// alias. urfave/cli drops an inherited flag that shares any name with a local
// one, so every form of log-level failed under serve with "flag provided but
// not defined".
//
// Revert-proof: remove serve's logLevelFlag(...) line and the serve rows fail
// to parse; make newRuntimeConfigFromCLI use c.String("log-level") and the
// root-before-serve row reports INFO.
func TestLogLevelAndListenAddrFlags(t *testing.T) {
	tests := []struct {
		args       []string
		wantLevel  string
		wantListen string
	}{
		{[]string{"serve", "-L", "DEBUG"}, "DEBUG", ":8080"},
		{[]string{"serve", "--log-level", "DEBUG"}, "DEBUG", ":8080"},
		{[]string{"serve", "--loglevel", "DEBUG"}, "DEBUG", ":8080"},
		{[]string{"serve", "-l", ":9090"}, "INFO", ":9090"},
		{[]string{"serve", "-L", "WARN", "-l", ":9090"}, "WARN", ":9090"},
		{[]string{"serve", "--listen-addr", ":9090", "--log-level", "ERROR"}, "ERROR", ":9090"},
		{[]string{"serve"}, "INFO", ":8080"},
		{[]string{"-L", "DEBUG", "serve"}, "DEBUG", ":8080"},
		{[]string{"-l", "DEBUG", "serve", "-l", ":9090"}, "DEBUG", ":9090"},
		{[]string{"--log-level", "WARN", "serve", "-L", "DEBUG"}, "DEBUG", ":8080"},
		{[]string{"validate", "--log-level", "DEBUG"}, "DEBUG", ""},
		{[]string{"-l", "DEBUG", "validate"}, "DEBUG", ""},
		{[]string{"validate", "-l", "DEBUG"}, "DEBUG", ""},
		{[]string{"validate"}, "INFO", ""},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			cfg := runCapturingConfig(t, tt.args...)
			assert.Equal(t, tt.wantLevel, cfg.LogLevel, "log level")
			assert.Equal(t, tt.wantListen, cfg.ListenAddress, "listen address")
		})
	}
}

func TestLogLevelFromEnvironment(t *testing.T) {
	for _, env := range []string{"SYVER_LOGLEVEL", "GOSS_LOGLEVEL"} {
		for _, sub := range []string{"serve", "validate"} {
			t.Run(env+"/"+sub, func(t *testing.T) {
				t.Setenv(env, "WARN")
				assert.Equal(t, "WARN", runCapturingConfig(t, sub).LogLevel)
				// The command line still wins over the environment.
				assert.Equal(t, "DEBUG", runCapturingConfig(t, sub, "-L", "DEBUG").LogLevel)
			})
		}
	}
}
