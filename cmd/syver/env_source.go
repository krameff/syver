package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

// nonEmptyEnvSource is a cli.ValueSource backed by a single environment
// variable that treats an exported-but-empty value as not-found.
//
// cli.EnvVars' own envVarValueSource calls os.LookupEnv directly, which
// returns found=true for an exported-but-empty variable -- and
// ValueSourceChain.LookupWithSource returns on the first found=true in
// the chain. That means a chain built with cli.EnvVars("SYVER_FMT",
// "GOSS_FMT") would let an exported-empty SYVER_FMT silently shadow a
// real GOSS_FMT value. nonEmptyEnvSource/nonEmptyEnvVars close that hole.
type nonEmptyEnvSource struct{ key string }

func (e nonEmptyEnvSource) Lookup() (string, bool) {
	v, ok := os.LookupEnv(e.key)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
func (e nonEmptyEnvSource) String() string { return fmt.Sprintf("environment variable %q", e.key) }
func (e nonEmptyEnvSource) GoString() string {
	return fmt.Sprintf("&nonEmptyEnvSource{key:%q}", e.key)
}
func (e nonEmptyEnvSource) IsFromEnv() bool { return true }
func (e nonEmptyEnvSource) Key() string     { return e.key }

// nonEmptyEnvVars mirrors cli.EnvVars but treats an exported-empty
// variable as not-found, so a blank SYVER_X never shadows a real GOSS_X.
// Pass the new SYVER_* name(s) first so they take priority over the
// legacy GOSS_* name(s) when both are set to a non-empty value.
func nonEmptyEnvVars(keys ...string) cli.ValueSourceChain {
	chain := cli.ValueSourceChain{}
	for _, k := range keys {
		chain.Chain = append(chain.Chain, nonEmptyEnvSource{key: k})
	}
	return chain
}
