package outputs

import (
	"sync"

	"github.com/fatih/color"
)

// forceNoColor disables ANSI colouring for the machine-readable outputs.
//
// json.go and junit.go each used to assign color.NoColor on every Output call.
// That is a write to a package global in fatih/color, and `serve` mode handles
// requests concurrently: two health probes rendering JSON at the same time is a
// write-write data race, reported by `go test -race` against a concurrent
// burst. It is a genuine runtime race in serve mode, not just a test artifact,
// and it is inherited from upstream.
//
// Both call sites only ever wrote the constant true, so collapsing them onto a
// sync.Once is behaviour-preserving as well as race-free, and Once gives every
// later caller a happens-before edge against the single write.
//
// The one behaviour this does not reproduce is re-forcing the flag after
// something set it back to false. Nothing does that after output has begun:
// getOutputer is the only writer of false, and both Serve and Validate call it
// once during setup, before any Output runs.
var noColorOnce sync.Once

func forceNoColor() {
	noColorOnce.Do(func() { color.NoColor = true })
}
