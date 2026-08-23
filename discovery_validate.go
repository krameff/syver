package syver

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
)

func validateDiscovery(ctx context.Context, sys *system.System, syverConfig SyverConfig, maxConcurrent int) (map[string]bool, error) {
	entries, err := syverConfig.Discovery.Entries()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no discovery tests found")
	}

	type discoveryResult struct {
		register string
		passed   bool
	}

	out := make(chan discoveryResult, len(entries))
	work := make(chan DiscoveryEntry)

	go func() {
		for _, entry := range entries {
			work <- entry
		}
		close(work)
	}()

	workerCount := runtime.NumCPU() * 5
	if workerCount > maxConcurrent {
		workerCount = maxConcurrent
	}
	if workerCount < 1 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range work {
				results := entry.Resource.Validate(ctx, sys)
				passed := true
				for _, result := range results {
					if result.Result == resource.FAIL {
						passed = false
						break
					}
				}
				out <- discoveryResult{
					register: entry.Register,
					passed:   passed,
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	discovered := make(map[string]bool, len(entries))
	for result := range out {
		discovered[result.register] = result.passed
	}

	return discovered, nil
}
