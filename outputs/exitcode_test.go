package outputs

import (
	"io"
	"testing"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
)

// Every registered outputer must report failure through its exit code. This is
// driven off Outputers() rather than a hand-written list precisely because the
// bug it pins survived six years: structured counted failures into
// Summary.Failed and then ended with a bare `return 0`, so it was the one
// formatter that reported a failing run as a success.
//
// That is not cosmetic. `validate --format structured` exited 0 on a failing
// spec, so any CI step using it passed unconditionally; and because `serve`
// negotiates the format from the request's Accept header, a caller could flip
// /healthz to 200 on a host where every check was failing just by asking for
// application/vnd.goss-structured.
//
// A new outputer that forgets this is caught here rather than in production.
func TestEveryOutputerSignalsFailureInItsExitCode(t *testing.T) {
	// The invariant is NOT "every formatter returns non-zero on failure". It is
	// "the exit code agrees with what the format is a rendering of".
	//
	// structured renders the VERDICT -- it has a Summary.Failed field and is the
	// machine-readable twin of rspecish/json -- so returning 0 while the body
	// said failed-count: 2 was an internal contradiction, and that is the bug
	// this test was written for.
	//
	// prometheus renders METRICS. It has no verdict field at all; the outcome is
	// a label value (syver_tests_outcomes_total{outcome="fail"}), and its -1 is
	// reserved for "could not emit". Making it non-zero would be the same
	// contradiction in the other direction: a well-formed exposition reported as
	// a transport failure. Over HTTP that means Prometheus marks the target down
	// and the metrics are lost exactly when they matter; on the CLI it breaks
	// node_exporter textfile collectors that read the status as "did the write
	// succeed". So it is pinned at 0 for BOTH outcomes rather than skipped --
	// a skip pins nothing, and would let someone "close the same hole" here and
	// break every scrape with no test to stop them.
	metricsOnly := map[string]bool{"prometheus": true}

	for _, name := range Outputers() {
		// discovery is not an Outputer at all: Discovery.Output takes a
		// map[string]bool, not a result channel, so it is never registered and
		// GetOutputer rejects it. Outputers() lists it anyway. The reason is
		// structural, not semantic -- do not restate it as "discovery has no
		// verdict", because that rationale would survive the fact changing.
		if name == "discovery" {
			if _, err := GetOutputer(name); err == nil {
				t.Errorf("discovery is now a real Outputer; give it a verdict contract here")
			}
			continue
		}
		t.Run(name, func(t *testing.T) {
			// The prometheus outputer increments counters in a package-level
			// registry, and prometheus_test.go asserts their exact values. Without
			// this reset, merely exercising it here shifts those counts and breaks
			// TestPrometheusOutput depending on run order -- which is exactly what
			// it did the first time.
			defer resetMetrics()

			o, err := GetOutputer(name)
			if err != nil {
				t.Fatalf("GetOutputer(%q): %v", name, err)
			}

			failCode := runOutputer(o, resource.FAIL)
			if metricsOnly[name] {
				if failCode != 0 {
					t.Errorf("%s returned %d for a FAILING result, want 0: it renders metrics, "+
						"not a verdict, and a non-zero code makes a scrape look like a dead target",
						name, failCode)
				}
			} else if failCode == 0 {
				t.Errorf("%s returned 0 for a FAILING result; a failing run must not "+
					"report success (validate would exit 0, and serve would answer 200)", name)
			}

			if code := runOutputer(o, resource.SUCCESS); code != 0 {
				t.Errorf("%s returned %d for a PASSING result, want 0", name, code)
			}
		})
	}
}

func runOutputer(o Outputer, outcome int) int {
	ch := make(chan []resource.TestResult, 1)
	ch <- []resource.TestResult{{
		Successful:   outcome == resource.SUCCESS,
		Result:       outcome,
		ResourceType: "Command",
		ResourceId:   "exit-code-probe",
		Property:     "exit-status",
		StartTime:    time.Now(),
		EndTime:      time.Now(),
	}}
	close(ch)
	return o.Output(io.Discard, ch, util.OutputConfig{})
}
