package util

import (
	"reflect"
	"testing"
)

func TestWithVarsBytes(t *testing.T) {
	vs := `{"hello":"world"}`
	c, err := NewConfig(WithVarsBytes([]byte(vs)))
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.VarsInline != vs {
		t.Fatalf("expected %q got %q", vs, c.VarsInline)
	}
}

func TestWithVarsString(t *testing.T) {
	vs := `{"hello":"world"}`
	c, err := NewConfig(WithVarsString(vs))
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.VarsInline != vs {
		t.Fatalf("expected %q got %q", vs, c.VarsInline)
	}
}

func TestWithVarsFiles(t *testing.T) {
	files := []string{"/nonexisting"}
	c, err := NewConfig(WithVarsFiles(files))
	if err != nil {
		t.Fatal(err.Error())
	}

	if !reflect.DeepEqual(c.VarsFiles, files) {
		t.Fatalf("expected %v got %q", files, c.VarsFiles)
	}

	files = []string{"/nonexisting", "/second", "third"}
	c, err = NewConfig(WithVarsFiles(files))
	if err != nil {
		t.Fatal(err.Error())
	}

	if !reflect.DeepEqual(c.VarsFiles, files) {
		t.Fatalf("expected %v got %q", files, c.VarsFiles)
	}
}

func TestWithVarsFile(t *testing.T) {
	c, err := NewConfig(WithVarsFile("/nonexisting"))
	if err != nil {
		t.Fatal(err.Error())
	}

	if !reflect.DeepEqual(c.VarsFiles, []string{"/nonexisting"}) {
		t.Fatalf("expected %v got %q", []string{"/nonexisting"}, c.VarsFiles)
	}
}

func TestWithVarsData(t *testing.T) {
	c, err := NewConfig(WithVarsData(map[string]string{"hello": "world"}))
	if err != nil {
		t.Fatal(err.Error())
	}

	if c.VarsInline != `{"hello":"world"}` {
		t.Fatalf("expected %q got %q", `{"hello":"world"}`, c.VarsInline)
	}
}

// A max concurrency below 1 starts no worker, so a run checks nothing and
// reports success. The option refuses it rather than store it.
//
// Revert-proof: drop the mc < 1 check from WithMaxConcurrency and the 0 and -1
// cases fail.
func TestWithMaxConcurrencyRejectsBelowOne(t *testing.T) {
	for _, mc := range []int{0, -1} {
		if _, err := NewConfig(WithMaxConcurrency(mc)); err == nil {
			t.Errorf("NewConfig(WithMaxConcurrency(%d)) succeeded, want an error", mc)
		}
	}
	c, err := NewConfig(WithMaxConcurrency(3))
	if err != nil || c.MaxConcurrent != 3 {
		t.Errorf("NewConfig(WithMaxConcurrency(3)) = %v, %v; want MaxConcurrent 3 and no error", c, err)
	}
}
