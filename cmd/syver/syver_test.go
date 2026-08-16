package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// resolveSpecPathCommand builds a minimal *cli.Command carrying the same
// --syverfile flag definition production code uses (Name/Aliases), with
// an Action that captures resolveSpecPath(c)'s result -- isolates
// resolveSpecPath from the rest of the app's flags/subcommands.
func resolveSpecPathCommand(got *string) *cli.Command {
	return &cli.Command{
		Name: "syver",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "syverfile",
				Aliases: []string{"gossfile", "g"},
				Sources: nonEmptyEnvVars("SYVER_FILE", "GOSS_FILE"),
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			*got = resolveSpecPath(c)
			return nil
		},
	}
}

func runResolveSpecPath(t *testing.T, args ...string) string {
	t.Helper()
	var got string
	cmd := resolveSpecPathCommand(&got)
	fullArgs := append([]string{"syver"}, args...)
	require.NoError(t, cmd.Run(context.Background(), fullArgs))
	return got
}

func TestResolveSpecPath_ExplicitFlagValue(t *testing.T) {
	t.Chdir(t.TempDir())
	got := runResolveSpecPath(t, "--syverfile", "explicit-path.yaml")
	assert.Equal(t, "explicit-path.yaml", got)
}

func TestResolveSpecPath_ExplicitViaGossfileAlias(t *testing.T) {
	t.Chdir(t.TempDir())
	got := runResolveSpecPath(t, "--gossfile", "explicit-path.yaml")
	assert.Equal(t, "explicit-path.yaml", got)
}

func TestResolveSpecPath_ExplicitViaShortAlias(t *testing.T) {
	t.Chdir(t.TempDir())
	got := runResolveSpecPath(t, "-g", "explicit-path.yaml")
	assert.Equal(t, "explicit-path.yaml", got)
}

func TestResolveSpecPath_NoCandidatesReturnsDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	got := runResolveSpecPath(t)
	assert.Equal(t, "./syver.yaml", got)
}

func TestResolveSpecPath_ProbeOrder(t *testing.T) {
	tests := []struct {
		name    string
		present []string
		want    string
	}{
		{"only_syver_yaml", []string{"syver.yaml"}, "syver.yaml"},
		{"only_syver_yml", []string{"syver.yml"}, "syver.yml"},
		{"only_goss_yaml", []string{"goss.yaml"}, "goss.yaml"},
		{"only_goss_yml", []string{"goss.yml"}, "goss.yml"},
		{"syver_yaml_wins_over_syver_yml", []string{"syver.yml", "syver.yaml"}, "syver.yaml"},
		{"syver_yml_wins_over_goss_yaml", []string{"goss.yaml", "syver.yml"}, "syver.yml"},
		{"goss_yaml_wins_over_goss_yml", []string{"goss.yml", "goss.yaml"}, "goss.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for _, f := range tt.present {
				require.NoError(t, os.WriteFile(f, []byte("{}"), 0644))
			}
			got := runResolveSpecPath(t)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveSpecPath_MultipleCandidatesWarnsAndPicksFirst(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("syver.yaml", []byte("{}"), 0644))
	require.NoError(t, os.WriteFile("goss.yaml", []byte("{}"), 0644))

	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	got := runResolveSpecPath(t)

	assert.Equal(t, "syver.yaml", got)
	assert.Equal(t, 1, strings.Count(logOutput.String(), "[WARN]"), "exactly one [WARN] line expected")
	assert.Contains(t, logOutput.String(), "multiple config files present")
}

// Test_nonEmptyEnvVars_Precedence covers §5.3's hazard test: an
// exported-but-empty SYVER_X must not shadow a real GOSS_X.
func Test_nonEmptyEnvVars_Precedence(t *testing.T) {
	tests := []struct {
		name    string
		envSet  map[string]string
		wantVal string
		wantOK  bool
	}{
		{
			name:    "syver_alone_wins",
			envSet:  map[string]string{"SYVER_FMT": "json"},
			wantVal: "json",
			wantOK:  true,
		},
		{
			name:    "goss_alone_still_works",
			envSet:  map[string]string{"GOSS_FMT": "json"},
			wantVal: "json",
			wantOK:  true,
		},
		{
			name:    "syver_wins_over_goss_when_both_set",
			envSet:  map[string]string{"SYVER_FMT": "json", "GOSS_FMT": "documentation"},
			wantVal: "json",
			wantOK:  true,
		},
		{
			name:    "exported_empty_syver_does_not_shadow_goss",
			envSet:  map[string]string{"SYVER_FMT": "", "GOSS_FMT": "json"},
			wantVal: "json",
			wantOK:  true,
		},
		{
			name:    "neither_set",
			envSet:  map[string]string{},
			wantVal: "",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envSet {
				t.Setenv(k, v)
			}
			chain := nonEmptyEnvVars("SYVER_FMT", "GOSS_FMT")
			val, ok := chain.Lookup()
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantVal, val)
			}
		})
	}
}

// Test_nonEmptyEnvVars_GeneralizesBeyondFmt repeats the exported-empty
// shadow test for SYVER_SLEEP/GOSS_SLEEP to confirm the helper
// generalizes, not just the FMT example the plan called out.
func Test_nonEmptyEnvVars_GeneralizesBeyondFmt(t *testing.T) {
	t.Setenv("SYVER_SLEEP", "")
	t.Setenv("GOSS_SLEEP", "5s")

	chain := nonEmptyEnvVars("SYVER_SLEEP", "GOSS_SLEEP")
	val, ok := chain.Lookup()
	require.True(t, ok)
	assert.Equal(t, "5s", val)
}

func TestApp_NameIsSyverNotGoss(t *testing.T) {
	app := newApp()
	assert.Equal(t, "syver", app.Name)
}

func TestApp_SyverfileFlagHasGossfileAlias(t *testing.T) {
	app := newApp()
	var syverfileFlag cli.Flag
	for _, f := range app.Flags {
		if slices.Contains(f.Names(), "syverfile") {
			syverfileFlag = f
			break
		}
	}
	require.NotNil(t, syverfileFlag, "syverfile flag not found on app")
	assert.Contains(t, syverfileFlag.Names(), "gossfile")
	assert.Contains(t, syverfileFlag.Names(), "g")
}

func TestApp_AddSyverSubcommandHasGossAlias(t *testing.T) {
	app := newApp()
	var addCmd *cli.Command
	for _, c := range app.Commands {
		if c.Name == "add" {
			addCmd = c
			break
		}
	}
	require.NotNil(t, addCmd, "add command not found")

	var syverSub *cli.Command
	for _, c := range addCmd.Commands {
		if c.Name == "syver" {
			syverSub = c
			break
		}
	}
	require.NotNil(t, syverSub, "add syver subcommand not found")
	assert.Contains(t, syverSub.Aliases, "goss")
}
