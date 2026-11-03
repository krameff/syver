package outputs

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

// metaMaxDepth bounds how many path segments flattenMeta follows. meta: is
// free-form and can be cyclic (a self-referencing YAML anchor), so recursion
// must stop somewhere rather than hang the run.
const metaMaxDepth = 10

// metaTruncated is the value emitted for a path whose value lies beyond
// metaMaxDepth.
const metaTruncated = "[truncated: nested deeper than 10 levels]"

// metaProperty is one flattened meta: entry, as a name/value pair of strings.
type metaProperty struct {
	Name  string
	Value string
}

// sortedMetaKeys returns m's keys in sorted order. Map iteration order is
// random, and every format that renders meta must render it identically from
// run to run.
func sortedMetaKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flattenMeta turns a free-form meta: map into a deterministic list of string
// name/value pairs, one per leaf:
//
//   - a scalar renders with %v, and nil as the empty string
//   - a nested map extends the path with ".key", keys sorted at every level
//   - a slice or array extends the path with "[i]"
//   - an empty nested map or slice is kept as its path with an empty value
//   - a non-empty map or slice reached at metaMaxDepth segments is emitted once
//     with metaTruncated as its value and not followed further
//
// An empty or nil map yields nil.
func flattenMeta(m map[string]any) []metaProperty {
	var out []metaProperty
	for _, k := range sortedMetaKeys(m) {
		out = flattenMetaValue(out, k, m[k], 1)
	}
	return out
}

func flattenMetaValue(out []metaProperty, path string, v any, depth int) []metaProperty {
	if v == nil {
		return append(out, metaProperty{path, ""})
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		if rv.Len() == 0 {
			return append(out, metaProperty{path, ""})
		}
		if depth >= metaMaxDepth {
			return append(out, metaProperty{path, metaTruncated})
		}
		if sm, ok := v.(map[string]any); ok {
			for _, k := range sortedMetaKeys(sm) {
				out = flattenMetaValue(out, path+"."+k, sm[k], depth+1)
			}
			return out
		}
		// Any other map shape, such as map[any]any: render the keys with %v
		// and sort on that rendering.
		byName := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			byName[fmt.Sprint(iter.Key().Interface())] = iter.Value().Interface()
		}
		for _, k := range sortedMetaKeys(byName) {
			out = flattenMetaValue(out, path+"."+k, byName[k], depth+1)
		}
		return out
	case reflect.Slice, reflect.Array:
		if rv.Len() == 0 {
			return append(out, metaProperty{path, ""})
		}
		if depth >= metaMaxDepth {
			return append(out, metaProperty{path, metaTruncated})
		}
		for i := 0; i < rv.Len(); i++ {
			out = flattenMetaValue(out, path+"["+strconv.Itoa(i)+"]", rv.Index(i).Interface(), depth+1)
		}
		return out
	default:
		return append(out, metaProperty{path, fmt.Sprintf("%v", v)})
	}
}

// junitProperties renders m as a testcase-level <properties> element, or the
// empty string when m has no entries, so a result without meta: renders
// exactly as it did before properties were emitted.
func junitProperties(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	s := "<properties>\n"
	for _, p := range flattenMeta(m) {
		s += "<property name=\"" + escapeString(p.Name) + "\" value=\"" + escapeString(p.Value) + "\"/>\n"
	}
	return s + "</properties>\n"
}
