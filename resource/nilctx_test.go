package resource

import (
	"testing"

	"github.com/krameff/syver/system"
)

// Resource.Validate is exported and just changed shape to take a context, so a
// library caller passing nil is a normal thing to try. It must not panic.
func TestValidateToleratesNilContext(t *testing.T) {
	c := &Command{Exec: "true", ExitStatus: 0}
	c.SetID("nil-ctx-probe")
	if got := c.Validate(nil, system.New("")); len(got) == 0 { //nolint:staticcheck // nil ctx is the point
		t.Fatal("no results")
	}
}
