package syver

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

func runDiscoveryPhase(ctx context.Context, sys *system.System, discovery DiscoveryConfig, maxConcurrent int) (map[string]bool, error) {
	cfg := SyverConfig{Discovery: discovery}
	return validateDiscovery(ctx, sys, cfg, maxConcurrent)
}

// loadSyverConfigWithDiscover is validate's load path: it works out the
// discovered map, then loads the spec with it. With --format discovery the
// spec IS the discovery suite, so nothing runs first.
func loadSyverConfigWithDiscover(ctx context.Context, c *util.Config) (*SyverConfig, error) {
	if c.OutputFormat == "discovery" {
		return getSyverConfig(c.VarsFiles, c.VarsInline, c.Spec, nil)
	}

	discovered, err := resolveDiscovered(ctx, c, false)
	if err != nil {
		return nil, err
	}
	return getSyverConfig(c.VarsFiles, c.VarsInline, c.Spec, discovered)
}

// resolveDiscovered runs the discovery phase for c and returns the map that
// becomes .Discovered, shared by validate, render and serve:
//
//   - with --discover, that file's discovery: tests run, and an inline
//     discovery: block in the main spec is ignored with an INFO log;
//   - otherwise the main spec is peeked, and its own discovery: block, if
//     any, runs;
//   - with no discovery: anywhere it returns nil.
//
// The peek is a separate, lenient decode (missing .Discovered keys render as
// zero values, decode warnings are quiet); the caller's real load is strict.
// Keep them separate: merging them changes error handling for specs whose
// templating depends on discovered values.
//
// validate returns a failed peek's error. render and serve pass lenientPeek,
// which treats a failed peek as "no discovery" and lets the caller's strict
// load report the problem, so a spec that does not use discovery fails with
// the same message it always did.
func resolveDiscovered(ctx context.Context, c *util.Config, lenientPeek bool) (map[string]bool, error) {
	sys := system.New(c.PackageManager)

	if c.DiscoverSpec != "" {
		discoverCfg, err := getSyverConfig(c.VarsFiles, c.VarsInline, c.DiscoverSpec, nil)
		if err != nil {
			return nil, fmt.Errorf("discover gossfile: %w", err)
		}
		if discoverCfg.Discovery.IsEmpty() {
			return nil, fmt.Errorf("discover gossfile %q has no discovery: tests", c.DiscoverSpec)
		}

		discovered, err := runDiscoveryPhase(ctx, sys, discoverCfg.Discovery, c.MaxConcurrent)
		if err != nil {
			return nil, fmt.Errorf("discover phase: %w", err)
		}

		if c.Spec != "" {
			peek, peekErr := getSyverConfigPeek(c.VarsFiles, c.VarsInline, c.Spec)
			if peekErr == nil && !peek.Discovery.IsEmpty() {
				log.Printf("[INFO] ignoring inline discovery: in %q; using --discover %q", c.Spec, c.DiscoverSpec)
			}
		}

		return discovered, nil
	}

	peek, err := getSyverConfigPeek(c.VarsFiles, c.VarsInline, c.Spec)
	if err != nil {
		if lenientPeek {
			return nil, nil
		}
		return nil, err
	}

	if peek.Discovery.IsEmpty() {
		return nil, nil
	}

	discovered, err := runDiscoveryPhase(ctx, sys, peek.Discovery, c.MaxConcurrent)
	if err != nil {
		return nil, fmt.Errorf("discover phase: %w", err)
	}
	return discovered, nil
}

var errNoDiscoverWithDiscover = errors.New("--no-discover and --discover cannot be used together")

// declaredDiscoveredFalse is render's --no-discover: no discovery check runs.
// Every register the spec's discovery: block declares is set to false, so a
// template gated on one renders its "not found" branch. A register the block
// does not declare is left out of the map, so referencing it still fails the
// strict (missingkey=error) load.
//
// A failed peek is treated as no declared registers, as resolveDiscovered's
// lenientPeek does, so the strict load reports the problem.
func declaredDiscoveredFalse(c *util.Config) (map[string]bool, error) {
	peek, err := getSyverConfigPeek(c.VarsFiles, c.VarsInline, c.Spec)
	if err != nil {
		return nil, nil
	}
	entries, err := peek.Discovery.Entries()
	if err != nil {
		return nil, err
	}
	discovered := make(map[string]bool, len(entries))
	for _, e := range entries {
		discovered[e.Register] = false
	}
	return discovered, nil
}
