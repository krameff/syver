package syver

import (
	"context"
	"testing"

	"github.com/krameff/syver/system"
)

// Discovery runs its resources on bare goroutines like the main schedulers do,
// so a panicking resource must fail its discovery check rather than end the
// process, and must not take its neighbours with it.
func TestDiscoveryPanicIsAFailedResult(t *testing.T) {
	quietLogs(t)

	entries := []DiscoveryEntry{
		{Register: "boom", Resource: &panicResource{id: "boom", boom: true}},
		{Register: "fine", Resource: &panicResource{id: "fine"}},
	}
	got := runDiscoveryEntries(context.Background(), system.New(""), entries, 2)

	if passed, ok := got["boom"]; !ok || passed {
		t.Errorf("boom = %v (present %v), want a recorded failure", passed, ok)
	}
	if !got["fine"] {
		t.Errorf("fine = false, want true: a neighbour's panic must not affect it")
	}
}
