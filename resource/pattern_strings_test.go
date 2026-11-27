package resource

import (
	"context"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

var (
	readerFuncType = reflect.TypeOf((func() (io.Reader, error))(nil))
	contextType    = reflect.TypeOf((*context.Context)(nil)).Elem()
	sysPtrType     = reflect.TypeOf((*system.System)(nil))
	configType     = reflect.TypeOf(util.Config{})
)

// readerTypes returns, for each per-type constructor field of sysType
// (NewFile, NewCommand, ...: func(context.Context, string, *System,
// util.Config) X), the type name it constructs and how many methods of X
// return (io.Reader, error).
func readerTypes(sysType reflect.Type) map[string]int {
	out := map[string]int{}
	for i := 0; i < sysType.NumField(); i++ {
		f := sysType.Field(i)
		ft := f.Type
		if !strings.HasPrefix(f.Name, "New") || ft.Kind() != reflect.Func {
			continue
		}
		if ft.NumIn() != 4 || ft.NumOut() != 1 ||
			ft.In(0) != contextType || ft.In(1).Kind() != reflect.String ||
			ft.In(2) != sysPtrType || ft.In(3) != configType {
			continue
		}
		iface := ft.Out(0)
		n := 0
		for m := 0; m < iface.NumMethod(); m++ {
			mt := iface.Method(m).Type
			if mt.NumIn() == readerFuncType.NumIn() && mt.NumOut() == 2 &&
				mt.Out(0) == readerFuncType.Out(0) && mt.Out(1) == readerFuncType.Out(1) {
				n++
			}
		}
		out[strings.TrimPrefix(f.Name, "New")] = n
	}
	return out
}

// patternAttrsProblems checks every descriptor's PatternAttrs against the
// system interfaces in sysType. The system side is matched to a descriptor
// by the Go type name of what the descriptor's New returns (Syverfile, not
// its dispatch name Gossfile).
func patternAttrsProblems(sysType reflect.Type, descs map[string]Descriptor) []string {
	readers := readerTypes(sysType)
	byGoName := map[string]Descriptor{}
	for _, d := range descs {
		byGoName[reflect.TypeOf(d.New()).Elem().Name()] = d
	}

	var problems []string
	for name, n := range readers {
		d, ok := byGoName[name]
		if !ok {
			problems = append(problems, "system.New"+name+" has no matching resource descriptor")
			continue
		}
		switch {
		case n > 0 && len(d.PatternAttrs) == 0:
			problems = append(problems, d.Key+" has (io.Reader, error) methods but no PatternAttrs")
		case n == 0 && d.PatternAttrs != nil:
			problems = append(problems, d.Key+" has no (io.Reader, error) methods but sets PatternAttrs")
		}
	}
	for goName, d := range byGoName {
		if _, ok := readers[goName]; !ok && d.PatternAttrs != nil {
			problems = append(problems, d.Key+" has no system type but sets PatternAttrs")
		}
		tags := yamlTags(reflect.TypeOf(d.New()).Elem())
		for _, a := range d.PatternAttrs {
			if !tags[a] {
				problems = append(problems, d.Key+": PatternAttrs names "+a+", which is not a yaml key of the type")
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func yamlTags(t reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name != "" {
			out[name] = true
		}
	}
	return out
}

// TestPatternAttrsMatchSystemReaders derives which types must declare
// PatternAttrs from system.System: a type whose system interface has a method
// returning (io.Reader, error) is matched line by line, so it must name the
// attributes that take patterns.
func TestPatternAttrsMatchSystemReaders(t *testing.T) {
	assert.Empty(t, patternAttrsProblems(reflect.TypeOf(system.System{}), Descriptors()))

	exact := map[string][]string{}
	for _, d := range Descriptors() {
		if d.PatternAttrs != nil {
			exact[d.Name] = d.PatternAttrs
		}
	}
	assert.Equal(t, map[string][]string{
		"File":    {"contains", "contents"},
		"Command": {"stdout", "stderr"},
		"HTTP":    {"body", "headers"},
	}, exact)
}

// fakePortReader is a system interface with a reader method, standing in for
// a new type that gains one.
type fakePortReader interface {
	Exists() (bool, error)
	Output() (io.Reader, error)
}

// TestPatternAttrsCheckCatchesANewReaderType proves the derived check fails
// for a type that gains an (io.Reader, error) method without PatternAttrs.
func TestPatternAttrsCheckCatchesANewReaderType(t *testing.T) {
	type fakeSystem struct {
		NewPort func(context.Context, string, *system.System, util.Config) fakePortReader
	}
	problems := patternAttrsProblems(reflect.TypeOf(fakeSystem{}), Descriptors())
	assert.Contains(t, problems, "port has (io.Reader, error) methods but no PatternAttrs")

	type renamedSystem struct {
		NewPortX func(context.Context, string, *system.System, util.Config) system.Port
	}
	problems = patternAttrsProblems(reflect.TypeOf(renamedSystem{}), Descriptors())
	assert.Contains(t, problems, "system.NewPortX has no matching resource descriptor")
}

func patterns(list ...any) []any { return list }

func onlyKey(k string, v any) map[string]any { return map[string]any{k: v} }

// TestPatternStrings_FindsEveryPositionAndOnlyThose builds resources in Go and
// checks the exact list, order included.
func TestPatternStrings_FindsEveryPositionAndOnlyThose(t *testing.T) {
	attrsOf := func(key string) []string {
		d, ok := Descriptors()[key]
		require.True(t, ok, key)
		return d.PatternAttrs
	}

	file := &File{
		Mode:     onlyKey("equal", onlyKey("have-patterns", patterns("/^decoy-equal"))),
		Owner:    onlyKey("and", patterns(onlyKey("not", onlyKey("have-patterns", patterns("/^owner-nested"))))),
		Contains: patterns("/^contains"),
		Contents: patterns("/^contents", 5, "plain"),
	}
	assert.Equal(t, []PatternRef{
		{Attr: "owner have-patterns", Pattern: "/^owner-nested"},
		{Attr: "contains", Pattern: "/^contains"},
		{Attr: "contents", Pattern: "/^contents"},
		{Attr: "contents", Pattern: "plain"},
	}, PatternStrings(file, attrsOf("file")))

	cmd := &Command{
		ExitStatus: onlyKey("gjson", map[string]any{"some.path": onlyKey("have-patterns", patterns("/^gjson"))}),
		Stdout:     patterns("/^stdout"),
		Stderr:     onlyKey("not", onlyKey("have-patterns", patterns("/^stderr-map"))),
	}
	assert.Equal(t, []PatternRef{
		{Attr: "exit-status have-patterns", Pattern: "/^gjson"},
		{Attr: "stdout", Pattern: "/^stdout"},
		{Attr: "stderr have-patterns", Pattern: "/^stderr-map"},
	}, PatternStrings(cmd, attrsOf("command")))

	web := &HTTP{
		Status:  onlyKey("gjson", onlyKey("have-patterns", patterns("/^decoy-gjson-key"))),
		Headers: patterns("/^headers"),
		Body:    patterns("/^body"),
	}
	assert.Equal(t, []PatternRef{
		{Attr: "headers", Pattern: "/^headers"},
		{Attr: "body", Pattern: "/^body"},
	}, PatternStrings(web, attrsOf("http")))

	match := &Matching{
		Content: onlyKey("have-patterns", patterns("/^decoy-content")),
		Matches: onlyKey("have-patterns", patterns("/^matches")),
	}
	assert.Equal(t, []PatternRef{
		{Attr: "matches have-patterns", Pattern: "/^matches"},
	}, PatternStrings(match, attrsOf("matching")))

	skipped := &File{Skip: true, Contents: patterns("/^skipped")}
	assert.Equal(t, []PatternRef{
		{Attr: "contents", Pattern: "/^skipped"},
	}, PatternStrings(skipped, attrsOf("file")))

	// A list at a non-pattern attribute is contain-elements: its items are
	// matchers, and a string item is an equal, not a pattern.
	bare := &Matching{Matches: patterns("/^equal-item", onlyKey("have-patterns", patterns("/^in-list")))}
	assert.Equal(t, []PatternRef{
		{Attr: "matches have-patterns", Pattern: "/^in-list"},
	}, PatternStrings(bare, attrsOf("matching")))

	// Every keyword that takes sub-matchers is followed.
	hp := func(p string) map[string]any { return onlyKey("have-patterns", patterns(p)) }
	nested := &Matching{Matches: onlyKey("or", patterns(
		onlyKey("have-key", hp("/^have-key")),
		onlyKey("contain-element", hp("/^contain-element")),
		onlyKey("contain-elements", patterns(hp("/^contain-elements"))),
		onlyKey("consist-of", patterns(hp("/^consist-of"))),
		onlyKey("match-regexp", "/^not-a-pattern"),
		map[string]any{"have-patterns": patterns("/^multi-key"), "equal": 1},
	))}
	assert.Equal(t, []PatternRef{
		{Attr: "matches have-patterns", Pattern: "/^have-key"},
		{Attr: "matches have-patterns", Pattern: "/^contain-element"},
		{Attr: "matches have-patterns", Pattern: "/^contain-elements"},
		{Attr: "matches have-patterns", Pattern: "/^consist-of"},
	}, PatternStrings(nested, attrsOf("matching")))
}
