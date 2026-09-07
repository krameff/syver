package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// These tests exist because of a report from a Windows session that
// "--vars-inline is accepted but its values never reach the template, while
// SYVER_VARS_INLINE works". That did not reproduce: measured on Windows
// Server 2025, the flag works in JSON and YAML flow form, in both PowerShell
// quoting styles, and before or after the subcommand. The likely real cause was
// cmd.exe, which does not treat ' as a quote character, so
// `--vars-inline '{inline: bar}'` is split and syver sees a stray argument.
//
// The report was still worth acting on, because nothing tested the claim either
// way. store_test.go covers varsFromString and loadVars, and util/config_test.go
// covers the config option, but NOTHING asserted that the flag and the env var
// both populate the same field. A divergence between those two paths -- exactly
// what was reported -- would have gone unnoticed.
//
// THESE DRIVE THE PRODUCTION FLAGS, NOT A COPY OF THEM, and that distinction
// is the whole point. Re-declaring the flag here would produce a test that
// passes even if `--vars-inline` were deleted from newApp() entirely -- the
// same vacuity that let the .Funcs() ordering in template.go go unprotected
// until it was revert-proved. So the flag set comes from newApp() and the value
// is read back through newRuntimeConfigFromCLI, which is the real CLI-to-config
// mapping at syver.go:86. Only the Action is test-owned, so nothing runs a
// validate.
//
// Revert-proof: delete the vars-inline flag from newApp()'s Flags and every
// test below fails with "flag provided but not defined".
func varsInlineCommand(got *string) *cli.Command {
	return &cli.Command{
		Name:  "syver",
		Flags: newApp().Flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			*got = newRuntimeConfigFromCLI(c).VarsInline
			return nil
		},
	}
}

func runVarsInline(t *testing.T, args ...string) string {
	t.Helper()
	var got string
	cmd := varsInlineCommand(&got)
	require.NoError(t, cmd.Run(context.Background(), append([]string{"syver"}, args...)))
	return got
}

func TestVarsInline_FlagReachesConfig(t *testing.T) {
	// The reported failure. If the flag is ever wired so its value is dropped,
	// this is what catches it.
	got := runVarsInline(t, "--vars-inline", `{"inline":"bar"}`)
	assert.Equal(t, `{"inline":"bar"}`, got)
}

func TestVarsInline_YAMLFlowFormSurvivesIntact(t *testing.T) {
	// The form integration-tests/test.sh actually passes. It contains spaces and
	// a comma, so it is the shape most likely to be mangled by argument
	// handling rather than by syver.
	const v = "{inline: bar, overwrite: bar}"
	assert.Equal(t, v, runVarsInline(t, "--vars-inline", v))
}

func TestVarsInline_EnvVarReachesConfig(t *testing.T) {
	t.Setenv("SYVER_VARS_INLINE", `{"inline":"from-env"}`)
	assert.Equal(t, `{"inline":"from-env"}`, runVarsInline(t))
}

func TestVarsInline_LegacyGossEnvVarStillHonoured(t *testing.T) {
	t.Setenv("GOSS_VARS_INLINE", `{"inline":"from-goss-env"}`)
	assert.Equal(t, `{"inline":"from-goss-env"}`, runVarsInline(t))
}

func TestVarsInline_FlagBeatsEnvVar(t *testing.T) {
	// An explicit flag must win. Without this, a stale exported variable could
	// silently override what the operator typed on the command line.
	t.Setenv("SYVER_VARS_INLINE", `{"inline":"from-env"}`)
	got := runVarsInline(t, "--vars-inline", `{"inline":"from-flag"}`)
	assert.Equal(t, `{"inline":"from-flag"}`, got)
}

func TestVarsInline_EmptySyverEnvDoesNotShadowGoss(t *testing.T) {
	// The nonEmptyEnvVars contract. An exported-but-empty SYVER_* must be
	// treated as unset rather than shadowing a real GOSS_* value, which is the
	// whole reason env_source.go exists.
	t.Setenv("SYVER_VARS_INLINE", "")
	t.Setenv("GOSS_VARS_INLINE", `{"inline":"from-goss-env"}`)
	assert.Equal(t, `{"inline":"from-goss-env"}`, runVarsInline(t))
}

func TestVarsInline_UnsetIsEmpty(t *testing.T) {
	// The control. Without this the assertions above could pass for the wrong
	// reason if the helper ever returned a default.
	assert.Empty(t, runVarsInline(t))
}

// --- the flag rejects a bad value at parse time -----------------------------

// A malformed value always failed, but it failed LATER, while loading vars, and
// by then a shell-mangled command line has usually left a stray argument that
// cli reads as a subcommand -- so the user saw a "No help topic" error naming
// that fragment, with nothing pointing at the flag. The Validator makes the
// error name
// --vars-inline and quote the value actually received, which is what reveals
// that the shell split the argument.
//
// Revert-proof: remove `Validator: syver.ValidateVarsInline` from newApp() and
// both tests below fail, because the command then parses cleanly and only fails
// much later.
func TestVarsInline_MalformedValueIsRejectedAtParseTime(t *testing.T) {
	var got string
	cmd := varsInlineCommand(&got)
	err := cmd.Run(context.Background(), []string{"syver", "--vars-inline", "{inline:"})

	require.Error(t, err, "a malformed inline value must be rejected")
	assert.Contains(t, err.Error(), "vars-inline",
		"the error must name the flag, or it does not help someone whose shell split the argument")
	assert.Contains(t, err.Error(), "{inline:",
		"the error must quote the value actually received, which is what shows the argument was split")
}

func TestVarsInline_MalformedValueSurvivesAStrayArgument(t *testing.T) {
	// The real cmd.exe shape: the flag gets a fragment AND a stray argument is
	// left over. The flag error must win, otherwise the stray one is read as a
	// subcommand and the message points nowhere useful.
	var got string
	cmd := varsInlineCommand(&got)
	err := cmd.Run(context.Background(), []string{"syver", "--vars-inline", "{inline:", "bar}"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "vars-inline")
	assert.NotContains(t, err.Error(), "No help topic",
		"the stray argument must not hijack the error")
}

func TestVarsInline_ValidValuesStillAccepted(t *testing.T) {
	// The control. A validator that rejected everything would satisfy the two
	// tests above and break the tool.
	for _, v := range []string{`{"inline":"bar"}`, "{inline: bar, overwrite: bar}", ""} {
		var got string
		cmd := varsInlineCommand(&got)
		args := []string{"syver"}
		if v != "" {
			args = append(args, "--vars-inline", v)
		}
		require.NoError(t, cmd.Run(context.Background(), args), "value %q must be accepted", v)
	}
}
