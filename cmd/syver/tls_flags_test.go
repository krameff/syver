package main

import (
	"context"
	"testing"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// runServeFlags runs serve's real flag set, Validators included, over args.
// It returns the config newRuntimeConfigFromCLI builds (nil if the flags were
// refused), whether --tls-cert counted as set, and the run's error.
func runServeFlags(t *testing.T, args ...string) (cfg *util.Config, certIsSet bool, err error) {
	t.Helper()
	var flags []cli.Flag
	for _, c := range newApp().Commands {
		if c.Name == "serve" {
			flags = c.Flags
		}
	}
	require.NotNil(t, flags, "newApp() has no serve subcommand")
	cmd := &cli.Command{
		Name:  "syver",
		Flags: flags,
		Action: func(_ context.Context, c *cli.Command) error {
			cfg = newRuntimeConfigFromCLI(c)
			certIsSet = c.IsSet("tls-cert")
			return nil
		},
	}
	err = cmd.Run(context.Background(), append([]string{"syver"}, args...))
	return cfg, certIsSet, err
}

// serveConfigFrom is runServeFlags for args that must be accepted.
func serveConfigFrom(t *testing.T, args ...string) *util.Config {
	t.Helper()
	cfg, _, err := runServeFlags(t, args...)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	return cfg
}

// clearTLSEnv blanks every TLS variable, which nonEmptyEnvVars reads as unset,
// so a value in the developer's own environment cannot leak into a row.
func clearTLSEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{"SYVER_TLS_CERT", "SYVER_TLS_KEY", "GOSS_TLS_CERT", "GOSS_TLS_KEY"} {
		t.Setenv(v, "")
	}
}

func TestServeTLSFlagsAndEnv(t *testing.T) {
	// Revert-proof: map tls-cert or tls-key to the wrong field in
	// newRuntimeConfigFromCLI.
	t.Run("flags", func(t *testing.T) {
		clearTLSEnv(t)
		cfg := serveConfigFrom(t, "--tls-cert", "a", "--tls-key", "b")
		assert.Equal(t, "a", cfg.ServeCertFile)
		assert.Equal(t, "b", cfg.ServeKeyFile)
	})

	// Revert-proof: remove the Sources line from either flag.
	t.Run("SYVER_TLS_* variables", func(t *testing.T) {
		clearTLSEnv(t)
		t.Setenv("SYVER_TLS_CERT", "s")
		t.Setenv("SYVER_TLS_KEY", "k")
		cfg := serveConfigFrom(t)
		assert.Equal(t, "s", cfg.ServeCertFile)
		assert.Equal(t, "k", cfg.ServeKeyFile)
	})

	// The goss names are deliberately not read: Syver's TLS differs from the
	// unmerged upstream proposal, so a GOSS_* name would promise compatibility
	// that does not exist.
	// Revert-proof: add "GOSS_TLS_CERT" (or _KEY) to that flag's
	// nonEmptyEnvVars chain.
	t.Run("GOSS_TLS_* variables are ignored", func(t *testing.T) {
		clearTLSEnv(t)
		t.Setenv("GOSS_TLS_CERT", "g")
		t.Setenv("GOSS_TLS_KEY", "g")
		cfg := serveConfigFrom(t)
		assert.Empty(t, cfg.ServeCertFile)
		assert.Empty(t, cfg.ServeKeyFile)
	})

	// Revert-proof: none in Syver code (urfave/cli's precedence); this pins the
	// documented order.
	t.Run("flag beats the variable", func(t *testing.T) {
		clearTLSEnv(t)
		t.Setenv("SYVER_TLS_CERT", "from-env")
		cfg := serveConfigFrom(t, "--tls-cert", "from-flag", "--tls-key", "k")
		assert.Equal(t, "from-flag", cfg.ServeCertFile)
	})
}

// An explicitly empty flag asked for HTTPS and named no file. It used to read as
// "not given", so `--tls-cert "" --tls-key ""` served plain HTTP silently.
func TestServeTLSFlagsRefuseExplicitEmpty(t *testing.T) {
	// Revert-proof: remove the Validator from --tls-cert (the error then
	// names --tls-key, or nothing at all).
	t.Run("both flags empty", func(t *testing.T) {
		clearTLSEnv(t)
		_, _, err := runServeFlags(t, "--tls-cert", "", "--tls-key", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--tls-cert was given an empty value")
	})

	// Revert-proof: remove the Validator from --tls-key.
	t.Run("key flag empty", func(t *testing.T) {
		clearTLSEnv(t)
		_, _, err := runServeFlags(t, "--tls-cert", "c", "--tls-key", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--tls-key was given an empty value")
	})

	// An exported-but-empty variable is unset, not an empty value, so it is
	// still plain HTTP rather than an error.
	// Revert-proof: build --tls-cert's Sources with cli.EnvVars instead of
	// nonEmptyEnvVars (the empty variable then counts as set and is refused).
	t.Run("exported-empty SYVER_TLS_CERT is unset", func(t *testing.T) {
		clearTLSEnv(t) // exports all four, empty
		cfg, certIsSet, err := runServeFlags(t)
		require.NoError(t, err)
		assert.False(t, certIsSet, "an empty SYVER_TLS_CERT must not count as set")
		assert.Empty(t, cfg.ServeCertFile)
		assert.Empty(t, cfg.ServeKeyFile)
	})
}

// The http resource's client-certificate fields must not pick up serve's own
// certificate.
// Revert-proof: map the flags to CertFile/KeyFile.
func TestServeTLSFlagsLeaveClientCertFieldsAlone(t *testing.T) {
	clearTLSEnv(t)
	cfg := serveConfigFrom(t, "--tls-cert", "a", "--tls-key", "b")
	assert.Empty(t, cfg.CertFile)
	assert.Empty(t, cfg.KeyFile)
}
