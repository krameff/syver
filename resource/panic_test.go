package resource

import (
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/krameff/syver/system"
)

// panickingResource implements the full Resource + ResourceRead pair, so the
// synthesised result can be checked for the identity fields an operator needs
// to find the offending entry in their spec.
type panickingResource struct {
	id      string
	title   string
	meta    meta
	panicOn any
	ran     bool
}

func (p *panickingResource) Validate(ctx context.Context, sys *system.System) []TestResult {
	p.ran = true
	if p.panicOn != nil {
		panic(p.panicOn)
	}
	return []TestResult{{Successful: true, Result: SUCCESS, ResourceType: p.TypeName(), ResourceId: p.id, Property: "ok"}}
}

func (p *panickingResource) SetID(id string)        { p.id = id }
func (p *panickingResource) SetSkip()               {}
func (p *panickingResource) TypeKey() string        { return "panicker" }
func (p *panickingResource) TypeName() string       { return "Panicker" }
func (p *panickingResource) GetRegister() string    { return "" }
func (p *panickingResource) GetDependsOn() []string { return nil }
func (p *panickingResource) ID() string             { return p.id }
func (p *panickingResource) GetTitle() string       { return p.title }
func (p *panickingResource) GetMeta() meta          { return p.meta }

// bareResource is Resource without ResourceRead. Resource does not require
// ResourceRead -- SkipResourceResults guards the assertion for exactly this
// reason -- so the fallback path is a real one and an embedder can land on it.
type bareResource struct{}

func (bareResource) Validate(ctx context.Context, sys *system.System) []TestResult {
	panic("bare boom")
}
func (bareResource) SetID(string)           {}
func (bareResource) SetSkip()               {}
func (bareResource) TypeKey() string        { return "bare" }
func (bareResource) TypeName() string       { return "Bare" }
func (bareResource) GetRegister() string    { return "" }
func (bareResource) GetDependsOn() []string { return nil }

// quietLogs captures the standard logger, so a passing run does not print the
// recovered panic's stack trace and so the tests below can assert on it.
//
// Not t.Parallel()-safe, and none of its callers are: log's output is a
// process-wide global.
func quietLogs(t *testing.T) *strings.Builder {
	t.Helper()
	var buf strings.Builder
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestValidateSafeTurnsAPanicIntoAFailingResult(t *testing.T) {
	logs := quietLogs(t)

	res := &panickingResource{id: "boom-id", title: "a title", meta: meta{"k": "v"}, panicOn: "kaboom"}
	results := ValidateSafe(context.Background(), res, system.New(""))

	if len(results) != 1 {
		t.Fatalf("got %d results, want exactly 1: a panicking resource must report its own "+
			"failure, not zero results (which every verdict rule reads as healthy)", len(results))
	}
	r := results[0]
	if r.Result != FAIL || r.Successful {
		t.Errorf("result = %d successful=%v, want FAIL/false", r.Result, r.Successful)
	}
	if r.Err == nil || !strings.Contains(r.Err.Error(), "kaboom") {
		t.Errorf("Err = %v, want the panic value in it; without it the output says a check "+
			"failed but not why", r.Err)
	}
	if r.ResourceType != "Panicker" || r.ResourceId != "boom-id" {
		t.Errorf("got %s/%s, want Panicker/boom-id: the result has to name the spec entry "+
			"that panicked or it cannot be acted on", r.ResourceType, r.ResourceId)
	}
	if r.Title != "a title" || r.Meta["k"] != "v" {
		t.Errorf("title/meta not carried through: got %q / %v", r.Title, r.Meta)
	}
	if r.EndTime.Before(r.StartTime) {
		t.Error("EndTime precedes StartTime; the json/junit formatters emit both")
	}
	// The stack is the only thing that says where the panic came from, and it
	// deliberately does not go into the result -- so it has to go somewhere.
	if !strings.Contains(logs.String(), "Recovered panic") {
		t.Errorf("nothing logged; the stack trace was dropped. Log was: %q", logs.String())
	}
}

func TestValidateSafeHandlesAResourceWithoutResourceRead(t *testing.T) {
	quietLogs(t)

	results := ValidateSafe(context.Background(), bareResource{}, system.New(""))
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ResourceId != "bare" {
		t.Errorf("ResourceId = %q, want the type key %q as the fallback", results[0].ResourceId, "bare")
	}
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "bare boom") {
		t.Errorf("Err = %v, want the panic value", results[0].Err)
	}
}

// The barrier must be invisible when nothing panics -- same results, same
// order. A recover() that also swallowed or reshaped normal results would pass
// every panic test above and break every real run.
func TestValidateSafeIsTransparentWhenNothingPanics(t *testing.T) {
	res := &panickingResource{id: "fine"}
	results := ValidateSafe(context.Background(), res, system.New(""))

	if !res.ran {
		t.Fatal("Validate was never called")
	}
	if len(results) != 1 || results[0].Result != SUCCESS || results[0].ResourceId != "fine" {
		t.Errorf("results = %+v, want the resource's own untouched SUCCESS result", results)
	}
}
