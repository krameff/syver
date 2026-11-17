package syver

import (
	"context"
	"fmt"
	"log"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

func runDiscoveryPhase(ctx context.Context, sys *system.System, discovery DiscoveryConfig, maxConcurrent int) (map[string]bool, error) {
	cfg := SyverConfig{Discovery: discovery}
	return validateDiscovery(ctx, sys, cfg, maxConcurrent)
}

func loadSyverConfigWithDiscover(ctx context.Context, c *util.Config) (*SyverConfig, error) {
	if c.OutputFormat == "discovery" {
		return getSyverConfig(c.VarsFiles, c.VarsInline, c.Spec, nil)
	}

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

		return getSyverConfig(c.VarsFiles, c.VarsInline, c.Spec, discovered)
	}

	peek, err := getSyverConfigPeek(c.VarsFiles, c.VarsInline, c.Spec)
	if err != nil {
		return nil, err
	}

	if peek.Discovery.IsEmpty() {
		return getSyverConfig(c.VarsFiles, c.VarsInline, c.Spec, nil)
	}

	discovered, err := runDiscoveryPhase(ctx, sys, peek.Discovery, c.MaxConcurrent)
	if err != nil {
		return nil, fmt.Errorf("discover phase: %w", err)
	}

	return getSyverConfig(c.VarsFiles, c.VarsInline, c.Spec, discovered)
}
