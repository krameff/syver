package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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
func isSetWarnEmpty(i interface{}, desc string, skip bool) bool {
	// Say nothing about a resource that is being skipped. The user has already
	// declared they do not want it checked -- some fixtures carry an empty list
	// precisely as a documented placeholder for an attribute that does not apply
	// on that platform -- so warning would be nagging about a decision they made
	// deliberately. skip is also true when the mandatory attribute already
	// failed, where the rest of the resource is moot anyway.
	if v, ok := i.([]interface{}); ok && len(v) == 0 && !skip {
		fmt.Fprintf(os.Stderr, "WARNING: %s is an empty list, which asserts nothing and always passes. Give it a value, or remove it.\n", desc)
	}
	return isSet(i)
}
