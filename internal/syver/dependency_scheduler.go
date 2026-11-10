package syver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type scheduledResource struct {
	ref      string
	resource resource.Resource
	deps     []string
}

func buildSchedule(resources []resource.Resource) ([]scheduledResource, error) {
	index := make(map[string]resource.Resource, len(resources))
	refsByKey := make(map[string][]string)

	for _, res := range resources {
		ref := resource.Ref(res)
		if _, exists := index[ref]; exists {
			return nil, fmt.Errorf("duplicate resource reference %q", ref)
		}
		index[ref] = res
		refsByKey[resource.YAMLKey(res)] = append(refsByKey[resource.YAMLKey(res)], ref)
	}

	schedule := make([]scheduledResource, 0, len(resources))
	for _, res := range resources {
		canonicalDeps := make([]string, 0, len(res.GetDependsOn()))
		for _, dep := range res.GetDependsOn() {
			depRef, err := resolveDependencyRef(dep, index, refsByKey)
			if err != nil {
				return nil, err
			}
			canonicalDeps = append(canonicalDeps, depRef)
		}

		schedule = append(schedule, scheduledResource{
			ref:      resource.Ref(res),
			resource: res,
			deps:     canonicalDeps,
		})
	}

	return schedule, detectDependencyCycles(schedule)
}

func resolveDependencyRef(dep string, index map[string]resource.Resource, refsByKey map[string][]string) (string, error) {
	if strings.Contains(dep, ":") {
		parts := strings.SplitN(dep, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", fmt.Errorf("invalid depends-on reference %q", dep)
		}
		ref := parts[0] + ":" + parts[1]
		if _, ok := index[ref]; !ok {
			return "", fmt.Errorf("depends-on reference %q not found", dep)
		}
		return ref, nil
	}

	matches := refsByKey[dep]
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("depends-on reference %q not found", dep)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("depends-on reference %q is ambiguous; use type:key (matches: %s)", dep, strings.Join(matches, ", "))
	}
}

func detectDependencyCycles(schedule []scheduledResource) error {
	state := make(map[string]int, len(schedule))
	byRef := make(map[string]scheduledResource, len(schedule))
	for _, item := range schedule {
		byRef[item.ref] = item
	}

	var visit func(ref string) error
	visit = func(ref string) error {
		switch state[ref] {
		case 1:
			return fmt.Errorf("dependency cycle detected at %q", ref)
		case 2:
			return nil
		}
		state[ref] = 1
		for _, dep := range byRef[ref].deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[ref] = 2
		return nil
	}

	for ref := range byRef {
		if err := visit(ref); err != nil {
			return err
		}
	}
	return nil
}

func hasDependencies(resources []resource.Resource) bool {
	for _, res := range resources {
		if len(res.GetDependsOn()) > 0 {
			return true
		}
	}
	return false
}

func validateWithDependencies(ctx context.Context, sys *system.System, resources []resource.Resource, maxConcurrent int) (<-chan []resource.TestResult, error) {
	schedule, err := buildSchedule(resources)
	if err != nil {
		return nil, err
	}

	out := make(chan []resource.TestResult)
	go func() {
		defer close(out)

		// stateMu guards status and completed. Both are written from every worker
		// goroutine below, and every resource runnable in the same dependency wave
		// races. Unsynchronised, that is `fatal error: concurrent map writes` --
		// a runtime throw, not a panic, so recover() cannot catch it and there is
		// no handler-level mitigation. Under `serve` a single unauthenticated GET
		// /healthz against a spec using depends-on killed the daemon outright.
		var stateMu sync.Mutex
		status := make(map[string]int, len(schedule))
		completed := make(map[string]bool, len(schedule))
		pending := append([]scheduledResource(nil), schedule...)

		workers := workerCount(maxConcurrent)

		for len(pending) > 0 {
			var runnable []scheduledResource
			var nextPending []scheduledResource
			progressed := false

			// An item is decided only once every prerequisite has completed, and
			// its skip reason names the first prerequisite, in declared order,
			// that did not pass. Deciding as soon as any one prerequisite failed
			// would make the reason depend on which of the others happened to be
			// complete in this pass, and so on pending order.
			for _, item := range pending {
				if completed[item.ref] {
					continue
				}

				ready := true
				skipReason := ""
				for _, dep := range item.deps {
					if !completed[dep] {
						ready = false
						continue
					}
					if skipReason != "" {
						continue
					}
					switch status[dep] {
					case resource.FAIL:
						skipReason = fmt.Sprintf("dependency %q failed", dep)
					case resource.SKIP:
						skipReason = fmt.Sprintf("dependency %q was skipped", dep)
					}
				}

				if !ready {
					nextPending = append(nextPending, item)
					continue
				}
				if skipReason != "" {
					out <- resource.SkipResourceResults(item.resource, skipReason)
					status[item.ref] = resource.SKIP
					completed[item.ref] = true
					progressed = true
					continue
				}
				runnable = append(runnable, item)
			}

			if len(runnable) == 0 {
				// A skip in this pass can complete a prerequisite of something
				// still pending (a chain a -> b -> c, with c listed before b), so
				// go round again before concluding nothing can run.
				if progressed {
					pending = nextPending
					continue
				}
				// buildSchedule rejects missing references and cycles, so this is
				// defensive: nothing pending can ever have all its prerequisites
				// complete.
				for _, item := range nextPending {
					out <- resource.SkipResourceResults(item.resource, "unsatisfied dependency")
				}
				return
			}

			work := make(chan scheduledResource)
			go func(items []scheduledResource) {
				for _, item := range items {
					work <- item
				}
				close(work)
			}(runnable)

			var wg sync.WaitGroup
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for item := range work {
						// ValidateSafe, not Validate -- same reason as in
						// validateParallel. Here a lost worker would also strand
						// every dependent still waiting on this ref to complete.
						results := resource.ValidateSafe(ctx, item.resource, sys)
						passed := true
						for _, result := range results {
							if result.Result == resource.FAIL {
								passed = false
								break
							}
						}
						stateMu.Lock()
						if passed {
							status[item.ref] = resource.SUCCESS
						} else {
							status[item.ref] = resource.FAIL
						}
						completed[item.ref] = true
						stateMu.Unlock()
						out <- results
					}
				}()
			}
			wg.Wait()

			pending = nextPending
		}
	}()

	return out, nil
}

func applyDisabledTypes(resources []resource.Resource, skipList []string) {
	for _, t := range resources {
		if util.IsValueInList(t.TypeName(), skipList) || util.IsValueInList(t.TypeKey(), skipList) {
			t.SetSkip()
		}
	}
}
