package outputs

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// The expected strings in goDifflibOracle are go-difflib's own output for each
// input, recorded before it was removed: 16 inputs captured from this
// package's callers during the test suite, 3 multi-line samples, and edge
// cases. The renderer must reproduce them byte for byte.
func TestUnifiedDiffMatchesGoDifflib(t *testing.T) {
	for _, c := range goDifflibOracle {
		t.Run(c.name, func(t *testing.T) {
			if got := unifiedDiff(c.want, c.got); got != c.diff {
				t.Errorf("diff differs from go-difflib\nwant:\n%s\ngot:\n%s", c.diff, got)
			}
		})
	}
}

// largeDiffInputs must stay identical to Large() in the oracle generator: 70000
// distinct lines, three of them changed. The line diff maps each distinct line
// to one rune, so a large file exercises that mapping well past 16 bits.
func largeDiffInputs() (string, string) {
	var a, b strings.Builder
	for i := 0; i < 70000; i++ {
		fmt.Fprintf(&a, "line-%06d\n", i)
		switch i {
		case 10, 35000, 69990:
			fmt.Fprintf(&b, "changed-%06d\n", i)
		default:
			fmt.Fprintf(&b, "line-%06d\n", i)
		}
	}
	return a.String(), b.String()
}

func TestUnifiedDiffLargeInputMatchesGoDifflib(t *testing.T) {
	want, got := largeDiffInputs()
	d := unifiedDiff(want, got)
	if sum := fmt.Sprintf("%x", sha256.Sum256([]byte(d))); sum != goDifflibLargeSHA256 {
		t.Errorf("diff of 70000 lines differs from go-difflib (sha256 %s, want %s):\n%s", sum, goDifflibLargeSHA256, d)
	}
}

func TestUnifiedDiffHasNoDeadline(t *testing.T) {
	if got := newLineDiffer().DiffTimeout; got != 0 {
		t.Errorf("DiffTimeout = %v, want 0: a deadline makes the diff depend on host speed", got)
	}
}

// hunkRange is tested directly because its empty-range branch cannot be
// reached through unifiedDiff: both inputs gain a trailing line, so neither
// side of a hunk is ever empty. Expected values follow go-difflib's
// formatRangeUnified (start is 1-based here, 0-based there).
func TestHunkRangeMatchesGoDifflib(t *testing.T) {
	for _, c := range []struct {
		start, n int
		want     string
	}{
		{1, 1, "1"},
		{7, 1, "7"},
		{1, 2, "1,2"},
		{5, 3, "5,3"},
		{1, 0, "0,0"},
		{8, 0, "7,0"},
	} {
		if got := hunkRange(c.start, c.n); got != c.want {
			t.Errorf("hunkRange(%d, %d) = %q, want %q", c.start, c.n, got, c.want)
		}
	}
}
