package syver

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestAccessorExhaustivenessGuard is FEAT-007's T-3/AC-5: removing an
// accessor-table entry must be caught, not silently ignored. init() already
// runs this check once at package load (and would have failed the whole
// test binary if it were broken there); this test exercises the same
// check function again after deliberately breaking each table, to prove
// the guard actually guards something rather than being vacuously true.
func TestAccessorExhaustivenessGuard(t *testing.T) {
	if err := checkAccessorExhaustiveness(); err != nil {
		t.Fatalf("accessor tables are broken before this test even starts: %v", err)
	}

	t.Run("missing configAccessors entry", func(t *testing.T) {
		saved := configAccessors["port"]
		delete(configAccessors, "port")
		t.Cleanup(func() { configAccessors["port"] = saved })

		err := checkAccessorExhaustiveness()
		if err == nil {
			t.Fatal("expected an error after removing configAccessors[\"port\"], got nil")
		}
		if !strings.Contains(err.Error(), "port") {
			t.Errorf("error %q does not mention the missing key", err.Error())
		}
	})

	t.Run("missing discoveryAccessors entry", func(t *testing.T) {
		saved := discoveryAccessors["file"]
		delete(discoveryAccessors, "file")
		t.Cleanup(func() { discoveryAccessors["file"] = saved })

		err := checkAccessorExhaustiveness()
		if err == nil {
			t.Fatal("expected an error after removing discoveryAccessors[\"file\"], got nil")
		}
		if !strings.Contains(err.Error(), "file") {
			t.Errorf("error %q does not mention the missing key", err.Error())
		}
	})

	t.Run("configAccessors entry with a nil Field", func(t *testing.T) {
		saved := configAccessors["user"]
		broken := saved
		broken.Field = nil
		configAccessors["user"] = broken
		t.Cleanup(func() { configAccessors["user"] = saved })

		err := checkAccessorExhaustiveness()
		if err == nil {
			t.Fatal("expected an error after nil-ing configAccessors[\"user\"].Field, got nil")
		}
	})

	t.Run("orphaned configAccessors entry with no matching descriptor", func(t *testing.T) {
		configAccessors["not-a-real-type"] = configAccessors["port"]
		t.Cleanup(func() { delete(configAccessors, "not-a-real-type") })

		err := checkAccessorExhaustiveness()
		if err == nil {
			t.Fatal("expected an error for an orphaned configAccessors entry, got nil")
		}
	})

	// Restored by the table's own t.Cleanup funcs; confirm it actually
	// went back to a clean state and isn't leaking into other tests.
	t.Cleanup(func() {
		if err := checkAccessorExhaustiveness(); err != nil {
			t.Errorf("accessor tables did not restore cleanly: %v", err)
		}
	})
}

// TestWiringReachesTheTaggedField checks each wiring line points at the
// struct fields whose yaml key is that line's resource key, on both
// SyverConfig and DiscoveryConfig, and that fieldOrder is the struct
// declaration order. The compiler only checks the two selectors agree on a
// map type, so `wire("file", <Packages selectors>)` would build cleanly and
// file every `file:` entry under package.
//
// REVERT-PROOF: swapping the selectors of two wiring lines, or reordering
// them, fails this test.
func TestWiringReachesTheTaggedField(t *testing.T) {
	yamlKey := func(f reflect.StructField) string {
		return strings.Split(f.Tag.Get("yaml"), ",")[0]
	}

	var c SyverConfig
	for _, key := range fieldOrder {
		got := reflect.ValueOf(configAccessors[key].Field(&c)).Pointer()
		v := reflect.ValueOf(&c).Elem()
		for i := range v.NumField() {
			if yamlKey(v.Type().Field(i)) == key && v.Field(i).Addr().Pointer() != got {
				t.Errorf("configAccessors[%q].Field does not point at SyverConfig.%s", key, v.Type().Field(i).Name)
			}
		}
	}

	dt := reflect.TypeFor[DiscoveryConfig]()
	var declared []string
	for i := range dt.NumField() {
		declared = append(declared, yamlKey(dt.Field(i)))
	}
	if !slices.Equal(fieldOrder, declared) {
		t.Errorf("fieldOrder = %v, want DiscoveryConfig's declaration order %v", fieldOrder, declared)
	}
	for _, key := range fieldOrder {
		var d DiscoveryConfig
		discoveryAccessors[key].Make(&d)
		v := reflect.ValueOf(d)
		for i := range v.NumField() {
			made := !v.Field(i).IsNil()
			if want := yamlKey(dt.Field(i)) == key; made != want {
				t.Errorf("discoveryAccessors[%q].Make: DiscoveryConfig.%s made=%v, want %v", key, dt.Field(i).Name, made, want)
			}
		}
	}
}
