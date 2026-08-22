package resource

import "reflect"

// YAMLKey returns the gossfile map key for a resource.
//
// Contract (FEAT-007 §4.5, documented now because it becomes load-bearing
// once Phase 2's external plugin types exist): this reads the unexported
// "id" struct field by reflection first, falling back to ResourceRead.ID()
// only if that field doesn't exist or isn't a string. Every builtin type in
// this package has that field (id string), so the fallback is never
// exercised today. A foreign package's unexported fields are unreadable via
// reflection from outside that package -- an external Resource
// implementation (Source: SourceExternal) will always hit the ID()
// fallback instead, never the "id" field. That's fine as long as its ID()
// is accurate, but it means `depends-on:` references into an external type
// depend entirely on that type's ID() being correct; there's no second,
// reflection-based check backing it up the way builtin types get.
func YAMLKey(res Resource) string {
	value := reflect.ValueOf(res)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	field := value.FieldByName("id")
	if field.IsValid() && field.Kind() == reflect.String {
		return field.String()
	}
	if rr, ok := res.(ResourceRead); ok {
		return rr.ID()
	}
	return ""
}

// Ref returns the canonical dependency reference for a resource.
func Ref(res Resource) string {
	return res.TypeKey() + ":" + YAMLKey(res)
}
