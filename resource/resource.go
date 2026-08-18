package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/krameff/syver/system"
)

type Resource interface {
	Validate(sys *system.System) []TestResult
	SetID(string)
	SetSkip()
	TypeKey() string
	TypeName() string
	GetRegister() string
	GetDependsOn() []string
}

var (
	resourcesMu sync.Mutex
	resources   = map[string]Resource{}
)

func registerResource(key string, resource Resource) {
	resourcesMu.Lock()
	resources[key] = resource
	resourcesMu.Unlock()
}

func Resources() map[string]Resource {
	return resources
}

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
