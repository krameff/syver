package resource

import (
	"reflect"
	"sort"
	"strings"
)

// PatternRef is one text pattern a resource will match line by line.
type PatternRef struct {
	Attr    string // yaml key: "contents", "stdout"; "matches have-patterns" for a nested list
	Pattern string // as decoded (rendered, for validate, serve and render)
}

var matcherType = reflect.TypeOf((*matcher)(nil)).Elem()

// PatternStrings returns r's patterns: list values of attrs, plus every
// have-patterns list reachable through the matcher grammar of any
// matcher-typed field. Order: struct field order, then list order. Fields of
// plain type any (matching's content) are data and are not searched. Items
// that are not strings are skipped.
func PatternStrings(r Resource, attrs []string) []PatternRef {
	v := reflect.ValueOf(r)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	patternAttr := make(map[string]bool, len(attrs))
	for _, a := range attrs {
		patternAttr[a] = true
	}

	var out []PatternRef
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type != matcherType || !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" {
			continue
		}
		value := v.Field(i).Interface()
		if list, ok := value.([]any); ok && patternAttr[name] {
			out = appendPatternStrings(out, name, list)
			continue
		}
		out = walkMatcherPatterns(out, name+" have-patterns", value)
	}
	return out
}

func appendPatternStrings(out []PatternRef, attr string, list []any) []PatternRef {
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, PatternRef{Attr: attr, Pattern: s})
		}
	}
	return out
}

// walkMatcherPatterns collects the have-patterns lists in a matcher value. It
// mirrors the recursion of matcherToGomegaMatcher in gomega.go and descends
// only where that function reads sub-matchers: a keyword added there that
// takes sub-matchers must be added here too. equal's value is data, and
// gjson's keys are JSON paths, so neither is entered.
func walkMatcherPatterns(out []PatternRef, attr string, value any) []PatternRef {
	switch x := value.(type) {
	case []any:
		// A bare list is contain-elements of matchers.
		for _, item := range x {
			out = walkMatcherPatterns(out, attr, item)
		}
	case map[string]any:
		if len(x) != 1 {
			return out // more than one matcher at a level is a decode error elsewhere
		}
		for key, sub := range x {
			switch key {
			case "have-patterns":
				if list, ok := sub.([]any); ok {
					out = appendPatternStrings(out, attr, list)
				}
			case "not", "have-key", "contain-element":
				out = walkMatcherPatterns(out, attr, sub)
			case "and", "or", "contain-elements", "consist-of":
				if list, ok := sub.([]any); ok {
					for _, item := range list {
						out = walkMatcherPatterns(out, attr, item)
					}
				}
			case "gjson":
				if paths, ok := sub.(map[string]any); ok {
					keys := make([]string, 0, len(paths))
					for k := range paths {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						out = walkMatcherPatterns(out, attr, paths[k])
					}
				}
			}
		}
	}
	return out
}
