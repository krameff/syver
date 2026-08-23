package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/krameff/syver/system"
)

type Resource interface {
	Validate(ctx context.Context, sys *system.System) []TestResult
	SetID(string)
	SetSkip()
	TypeKey() string
	TypeName() string
	GetRegister() string
	GetDependsOn() []string
}

// registerResource/Resources() moved to descriptor.go (FEAT-007): the
// registry is now the Descriptor table, with these kept as deprecated
// shims for backward compatibility. See descriptor.go for both.

type ResourceRead interface {
	ID() string
	GetTitle() string
	GetMeta() meta
}

type matcher any
type meta map[string]any

func contains(a []string, s string) bool {
	for _, e := range a {
		if m, _ := filepath.Match(e, s); m {
			return true
		}
	}
	return false
}

func deprecateAtoI(depr any, desc string) any {
	s, ok := depr.(string)
	if !ok {
		return depr
	}
	fmt.Fprintf(os.Stderr, "DEPRECATION WARNING: %s should be an integer not a string\n", desc)
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return float64(i)
}

func shouldSkip(results []TestResult) bool {
	if len(results) < 1 {
		return false
	}
	if results[0].Err != nil || results[0].Result != SUCCESS || results[0].MatcherResult.Actual == false {
		return true
	}
	return false
}

// isSet reports whether an optional attribute carries an expectation.
//
// It is the guard every optional attribute uses, so that "no expectation"
// means the same thing everywhere. Two spellings reach here:
//
//	mode:            # absent or null -> nil
//	contents: []     # present but empty
//
// The empty list is not merely a stylistic choice: it is what `syver add`
// and `syver autoadd` emit for an attribute they found nothing to assert
// about, so it appears throughout generated gossfiles. Treating it as an
// expectation would make every generated spec assert a vacuous "contains
// at least nothing", which always passes and only inflates the test count.
//
// Note this is deliberately not applied to the mandatory attribute of each
// resource (file.exists, command.exit-status, http.status and so on). Those
// always report, so that a resource always produces at least one result.
// withID stamps the resource id onto the context every Validate carries.
//
// It tolerates a nil parent because Validate is exported and just changed shape
// to take a context: a library caller updating to the new signature and passing
// nil is a normal thing to try, and context.WithValue panics on a nil parent.
// system/command.go has a matching nil guard, but that one sits a level below
// this call and so was unreachable from here.
func withID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, idKey{}, id)
}

func isSet(i interface{}) bool {
	switch v := i.(type) {
	case []interface{}:
		return len(v) > 0
	default:
		return i != nil
	}
}

// isSetWarnEmpty is isSet plus a warning. isSet quietly drops an empty list,
// which is right -- it asserts nothing, so it must not inflate the count or
// produce a vacuous pass. But quietly is the problem when a human wrote it:
// `opts: []` looks like an assertion and behaves like an absent attribute, and
// nothing said so. This warns and then defers to isSet, so behaviour is
// unchanged: still skipped, still passing, still the same exit code.
//
// desc follows the "<id>: <type>.<property>" shape the deprecation warnings
// use. Wired only where a list is a plausible thing to write; on a scalar
// attribute the type assertion below could never match anyway.
// warnedEmpty tracks which empty-list attributes have already been reported.
//
// The warning describes a STATIC property of the spec, but it is emitted from
// Validate, which runs once per sweep. Under `serve` that is not one line, it is
// one line per empty attribute per probe -- at the default 5s cache, tens of
// thousands a day all saying the same thing, and unsuppressable because this
// writes to stderr directly rather than through the levelled logger. Anything
// genuinely worth reading gets buried.
//
// Once per process per attribute is the right frequency for a fact that cannot
// change while the process runs.
var warnedEmpty sync.Map

func warnEmptyOnce(desc string) {
	if _, seen := warnedEmpty.LoadOrStore(desc, struct{}{}); seen {
		return
	}
	fmt.Fprintf(os.Stderr, "WARNING: %s is an empty list, which asserts nothing and always passes. Give it a value, or remove it.\n", desc)
}

func isSetWarnEmpty(i interface{}, desc string, skip bool) bool {
	// Say nothing about a resource the user has declared they do not want
	// checked: some specs carry an empty list precisely as a documented
	// placeholder for an attribute that does not apply on that platform, and
	// warning there nags about a deliberate decision.
	//
	// Callers pass the resource's own Skip field, NOT the local `skip` that
	// Validate promotes when the mandatory attribute fails. A vacuous `opts: []`
	// is a static property of the spec; whether the user hears about it must not
	// depend on the state of the host they happen to be running against. Keying
	// off the promoted value would hide the warning on exactly the run where
	// they are already looking at that resource, and then stay silent forever
	// once the mandatory attribute starts passing. It would also be inconsistent:
	// command and service never promote skip at all.
	if v, ok := i.([]interface{}); ok && len(v) == 0 && !skip {
		warnEmptyOnce(desc)
	}
	return isSet(i)
}
