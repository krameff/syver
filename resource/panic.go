package resource

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/krameff/syver/system"
)

// ValidateSafe is res.Validate with a panic barrier.
//
// Both schedulers (validateParallel and validateWithDependencies) run resources
// on bare goroutines. Nothing above them recovers -- net/http's per-connection
// recover does not extend to goroutines a handler started, and there is no
// recover() anywhere else in non-test code -- so a panic inside any one
// resource's Validate takes the whole process down. Under `validate` that is a
// confusing crash instead of a failing check; under `serve` it is the daemon
// dying on a request, from a spec it was handed at startup and has been serving
// happily until the panic's trigger appears.
//
// That is not hypothetical. An empty map matcher (`stdout: {}`) panicked with
// index-out-of-range, and a served spec containing one killed the daemon on the
// first probe. That specific trigger is fixed; this closes the class, so the
// next one costs a FAIL on one resource rather than the process.
//
// Deliberately per-resource, not per-worker: a worker-level recover would end
// the worker and silently drop every resource still queued behind it, which
// turns a crash into an under-reported pass. Here the panicking resource fails,
// its neighbours are unaffected, and the exit code and /healthz status follow
// from the FAIL like any other failure.
//
// This does not, and cannot, catch a runtime *throw* -- concurrent map writes,
// out-of-memory, a nil-interface method call. Those are not panics and recover()
// never sees them; see the stateMu comment in dependency_scheduler.go.
func ValidateSafe(ctx context.Context, res Resource, sys *system.System) (results []TestResult) {
	startTime := time.Now()
	defer func() {
		if rec := recover(); rec != nil {
			// The stack goes to the log, not into the result. It is the only
			// thing that identifies where the panic came from, and it must not be
			// lost -- but it is multi-line, machine-specific and different on
			// every run, so putting it in a TestResult would make junit/json
			// output unstable and unreadable. The result carries the message.
			log.Printf("[ERROR] Recovered panic validating %s %s: %v\n%s",
				res.TypeName(), resourceIDFor(res), rec, debug.Stack())
			results = panicResults(res, rec, startTime)
		}
	}()
	return res.Validate(ctx, sys)
}

// resourceIDFor names the spec entry that panicked, as precisely as the
// resource allows.
//
// ID() first, matching what SkipResourceResults and every normal result use, so
// a panic result sorts and reads alongside them. Resource does not require
// ResourceRead though -- the assertion has to be guarded, same as there -- and
// for a type that lacks it, YAMLKey still recovers the key by reflection.
// Falling straight to the type key would collapse every resource of that type
// onto one name, which is the one thing this result exists to avoid: it is the
// only pointer back to the entry that has to be fixed.
func resourceIDFor(res Resource) string {
	if rr, ok := res.(ResourceRead); ok {
		return rr.ID()
	}
	if key := YAMLKey(res); key != "" {
		return key
	}
	return res.TypeKey()
}

// panicResults builds the single failing result a panicking resource reports.
//
// The shape follows serve.go's could-not-run result: Successful false, Result
// FAIL, and the reason in Err rather than in MatcherResult. Every outputer
// renders Err (prettyPrintTestResult branches on it before it touches the
// matcher fields), so a result with an empty MatcherResult is well-formed in
// all nine formats.
func panicResults(res Resource, rec any, startTime time.Time) []TestResult {
	endTime := time.Now()
	validateErr := ValidateError(fmt.Sprintf("panic: %v", rec))

	result := TestResult{
		Successful:   false,
		Result:       FAIL,
		ResourceType: res.TypeName(),
		ResourceId:   resourceIDFor(res),
		Property:     "panic",
		Err:          &validateErr,
		StartTime:    startTime,
		EndTime:      endTime,
		Duration:     endTime.Sub(startTime),
	}
	if rr, ok := res.(ResourceRead); ok {
		result.Title = rr.GetTitle()
		result.Meta = rr.GetMeta()
	}

	return []TestResult{result}
}
