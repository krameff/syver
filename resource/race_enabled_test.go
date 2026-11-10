//go:build race

package resource

// raceEnabled reports whether the race detector is on. It slows instrumented
// code several times over, so timing bounds are not asserted under it.
const raceEnabled = true
