package syver

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/krameff/syver/outputs"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

// panicResource panics on Validate. `id` is unexported deliberately: that is
// the field resource.YAMLKey reflects on to build a dependency ref, so a fake
// with an exported one would not be schedulable by validateWithDependencies.
type panicResource struct {
	id        string
	dependsOn []string
	boom      bool
}

func (p *panicResource) Validate(ctx context.Context, sys *system.System) []resource.TestResult {
	if p.boom {
		// An index out of range, rather than panic("..."): a synthetic panic
		// proves the barrier catches panics, but the class this defends against
		// is the accidental kind, and this is the exact shape of the one that
		// motivated it -- the empty map matcher read element 0 of a slice it had
		// not checked was non-empty.
		_ = firstOf(nil)
	}
	return []resource.TestResult{{
		Successful:   true,
		Result:       resource.SUCCESS,
		ResourceType: p.TypeName(),
		ResourceId:   p.id,
		Property:     "ok",
	}}
}

// firstOf takes its slice as a parameter so the out-of-range read cannot be
// proven statically; a literal `[]string{}[0]` is a compile error, and the nil
// map spelling is a staticcheck failure (SA5000). Neither is available, and
// both would be the wrong shape anyway: this is meant to look like a bug, not
// like a test fixture.
func firstOf(ss []string) string { return ss[0] }

func (p *panicResource) SetID(id string)        { p.id = id }
func (p *panicResource) SetSkip()               {}
func (p *panicResource) TypeKey() string        { return "panicker" }
func (p *panicResource) TypeName() string       { return "Panicker" }
func (p *panicResource) GetRegister() string    { return "" }
func (p *panicResource) GetDependsOn() []string { return p.dependsOn }

func quietLogs(t *testing.T) {
	t.Helper()
	log.SetOutput(&bytes.Buffer{})
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
}

func drain(t *testing.T, out <-chan []resource.TestResult) map[string]resource.TestResult {
	t.Helper()
	byID := map[string]resource.TestResult{}
	for group := range out {
		for _, r := range group {
			byID[r.ResourceId] = r
		}
	}
	return byID
}

// validateParallel's workers are bare goroutines. net/http's per-connection
// recover does not reach a goroutine a handler started, so before the barrier
// a panic in any one resource took the whole process with it -- under `serve`,
// the daemon dying on a request, from a spec it had been serving until the
// trigger appeared.
//
// The neighbours matter as much as the failure: a recover placed on the worker
// instead of the resource would end the worker and silently drop everything
// still queued behind it, turning a crash into an under-reported pass.
func TestPanicInOneResourceDoesNotKillValidateParallel(t *testing.T) {
	quietLogs(t)

	resources := []resource.Resource{
		&panicResource{id: "before"},
		&panicResource{id: "exploding", boom: true},
		&panicResource{id: "after"},
	}

	got := drain(t, validateParallel(context.Background(), system.New(""), resources, 50))

	if len(got) != 3 {
		t.Fatalf("got %d results, want 3: the panicking resource must fail alone, not take "+
			"its neighbours' results with it (got %v)", len(got), got)
	}
	bad, ok := got["exploding"]
	if !ok {
		t.Fatal("no result for the panicking resource; it vanished instead of failing")
	}
	if bad.Result != resource.FAIL || bad.Successful {
		t.Errorf("panicking resource reported %d/successful=%v, want FAIL/false", bad.Result, bad.Successful)
	}
	if bad.Err == nil || !strings.Contains(bad.Err.Error(), "index out of range") {
		t.Errorf("Err = %v, want the panic message", bad.Err)
	}
	for _, id := range []string{"before", "after"} {
		if got[id].Result != resource.SUCCESS {
			t.Errorf("%s reported %d, want SUCCESS: its neighbour panicking must not affect it", id, got[id].Result)
		}
	}
}

// The dependency scheduler runs its own worker pool, so it needs its own
// barrier -- and here a lost worker is worse: it never marks the ref complete,
// so every dependent stays blocked and the scheduler drains them as
// "unsatisfied dependency".
func TestPanicInOneResourceDoesNotKillTheDependencyScheduler(t *testing.T) {
	quietLogs(t)

	resources := []resource.Resource{
		&panicResource{id: "root", boom: true},
		&panicResource{id: "leaf", dependsOn: []string{"panicker:root"}},
		&panicResource{id: "independent"},
	}

	out, err := validateWithDependencies(context.Background(), system.New(""), resources, 50)
	if err != nil {
		t.Fatalf("validateWithDependencies: %v", err)
	}

	// A slice, not a map keyed by id: SkipResourceResults falls back to the
	// type key for a resource that does not implement resource.ResourceRead
	// (the meta type it returns is unexported, so a fake outside that package
	// cannot), and this fake's skip result is therefore named "panicker".
	var got []resource.TestResult
	for group := range out {
		got = append(got, group...)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3: %+v", len(got), got)
	}

	var root, leaf, independent resource.TestResult
	for _, r := range got {
		switch r.ResourceId {
		case "root":
			root = r
		case "independent":
			independent = r
		default:
			leaf = r
		}
	}

	if root.Result != resource.FAIL {
		t.Errorf("root reported %d, want FAIL", root.Result)
	}
	if root.Err == nil || !strings.Contains(root.Err.Error(), "index out of range") {
		t.Errorf("root Err = %v, want the panic message", root.Err)
	}
	// A panicking dependency is a failing dependency: its dependent is skipped
	// with a reason, which is the scheduler's normal behaviour for a FAIL and
	// the thing that proves the ref was marked complete rather than stranded.
	// A worker lost to the panic would instead drain it as "unsatisfied
	// dependency" -- the scheduler's give-up path -- or hang.
	if leaf.Result != resource.SKIP {
		t.Errorf("leaf reported %d, want SKIP", leaf.Result)
	}
	if leaf.Err == nil || !strings.Contains(leaf.Err.Error(), `dependency "panicker:root" failed`) {
		t.Errorf("leaf Err = %v, want it to name the failed dependency; "+
			"\"unsatisfied dependency\" instead would mean the scheduler gave up rather "+
			"than scheduling past the panic", leaf.Err)
	}
	if independent.Result != resource.SUCCESS {
		t.Errorf("independent reported %d, want SUCCESS", independent.Result)
	}
}

// A synthesised result has an empty MatcherResult, which is a shape no normal
// check produces. Every formatter has to render it -- `serve` picks the format
// from the request's Accept header, so a formatter that panicked on this shape
// would hand the daemon the very crash the barrier just prevented, one layer up.
func TestPanicResultRendersInEveryOutputFormat(t *testing.T) {
	quietLogs(t)

	results := drain(t, validateParallel(context.Background(),
		system.New(""), []resource.Resource{&panicResource{id: "exploding", boom: true}}, 50))
	panicked := results["exploding"]

	for _, name := range outputs.Outputers() {
		o, err := outputs.GetOutputer(name)
		if err != nil {
			// discovery is listed by Outputers() but is not a real Outputer.
			continue
		}
		t.Run(name, func(t *testing.T) {
			in := make(chan []resource.TestResult, 1)
			in <- []resource.TestResult{panicked}
			close(in)

			var b bytes.Buffer
			code := o.Output(&b, in, util.OutputConfig{})

			if b.Len() == 0 && name != "silent" {
				t.Errorf("%s rendered nothing for a panic result", name)
			}
			// prometheus renders metrics, not a verdict, and pins its code at 0
			// for both outcomes -- see outputs/exitcode_test.go.
			if name != "prometheus" && code == 0 {
				t.Errorf("%s returned exit code 0 for a panic result; a resource that "+
					"crashed must not report success", name)
			}
		})
	}
}
