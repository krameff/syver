package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/krameff/syver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// Padded --format-options values must be trimmed, not rejected. A value such
// as GOSS_FMT_OPTIONS="pretty, sort" splits into "pretty" and " sort"; both
// must pass the check and reach the outputter trimmed. Covers the flag and
// both environment variables, through validate's real flag set. The stand-in
// Action calls syver.Validate directly, not the subcommand's own Action, so
// the Windows alpha gate (fatalAlphaIfNeeded) never runs here.
func TestPaddedFormatOptionsAreTrimmed(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "goss.yaml")
	require.NoError(t, os.WriteFile(spec, []byte("file:\n  "+filepath.ToSlash(spec)+":\n    exists: true\n"), 0o600))

	run := func(t *testing.T, args ...string) {
		t.Helper()
		var flags []cli.Flag
		for _, c := range newApp().Commands {
			if c.Name == "validate" {
				flags = c.Flags
			}
		}
		require.NotNil(t, flags)
		var out bytes.Buffer
		cmd := &cli.Command{
			Name:  "syver",
			Flags: flags,
			Action: func(ctx context.Context, c *cli.Command) error {
				cfg := newRuntimeConfigFromCLI(c)
				cfg.Spec = spec
				cfg.LogLevel = "INFO" // a root flag, absent from this stand-in
				cfg.OutputWriter = &out
				code, err := syver.Validate(ctx, cfg)
				require.NoError(t, err)
				assert.Equal(t, 0, code)
				assert.Equal(t, []string{"pretty", "sort"}, cfg.FormatOptions)
				return nil
			},
		}
		require.NoError(t, cmd.Run(context.Background(), append([]string{"syver", "-f", "json"}, args...)))
		assert.True(t, strings.HasPrefix(out.String(), "{\n"), "pretty should take effect, got:\n%s", out.String())
	}

	t.Run("flag", func(t *testing.T) { run(t, "-o", " pretty", "-o", "sort ") })
	for _, env := range []string{"SYVER_FMT_OPTIONS", "GOSS_FMT_OPTIONS"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "pretty, sort")
			run(t)
		})
	}
}
