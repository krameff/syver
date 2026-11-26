package outputs

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// diffContext is the number of unchanged lines shown around each change.
const diffContext = 1

// newLineDiffer returns the diff engine used by unifiedDiff. DiffTimeout is 0
// (no deadline): the library default is one second, after which it returns a
// valid but longer diff, so output would depend on how fast the host is.
func newLineDiffer() *diffmatchpatch.DiffMatchPatch {
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 0
	return dmp
}

type diffLine struct {
	op   byte // ' ', '-' or '+'
	text string
}

// unifiedDiff renders the line diff of want against got as a unified diff with
// headers "--- test" and "+++ actual", in the format the failure output has
// always used (it was produced by pmezard/go-difflib until 0.22.0).
//
// Both inputs get a trailing "\n" before diffing. go-difflib's SplitLines
// appended one to the last line, which shows as a blank context line after a
// change on the last line; keeping it keeps the output byte-identical.
func unifiedDiff(want, got string) string {
	dmp := newLineDiffer()
	a, b, lines := dmp.DiffLinesToChars(want+"\n", got+"\n")
	diffs := dmp.DiffCharsToLines(dmp.DiffMain(a, b, false), lines)

	var ops []diffLine
	for _, d := range diffs {
		op := byte(' ')
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			op = '+'
		case diffmatchpatch.DiffDelete:
			op = '-'
		}
		for _, l := range strings.SplitAfter(d.Text, "\n") {
			if l != "" {
				ops = append(ops, diffLine{op, l})
			}
		}
	}

	var out strings.Builder
	out.WriteString("--- test\n+++ actual\n")
	for i := 0; i < len(ops); {
		for i < len(ops) && ops[i].op == ' ' {
			i++
		}
		if i == len(ops) {
			break
		}
		start := max(i-diffContext, 0)
		end := hunkEnd(ops, i)
		writeHunk(&out, ops, start, end)
		i = end
	}
	return out.String()
}

// hunkEnd returns the index just past the hunk whose first change is at i. Two
// changes separated by 2*diffContext or fewer unchanged lines share a hunk.
func hunkEnd(ops []diffLine, i int) int {
	end := i
	for end < len(ops) {
		if ops[end].op != ' ' {
			end++
			continue
		}
		j := end
		for j < len(ops) && ops[j].op == ' ' {
			j++
		}
		if j == len(ops) || j-end > 2*diffContext {
			return end + min(diffContext, j-end)
		}
		end = j
	}
	return end
}

func writeHunk(out *strings.Builder, ops []diffLine, start, end int) {
	aStart, bStart := 1, 1
	for _, o := range ops[:start] {
		if o.op != '+' {
			aStart++
		}
		if o.op != '-' {
			bStart++
		}
	}
	var na, nb int
	for _, o := range ops[start:end] {
		if o.op != '+' {
			na++
		}
		if o.op != '-' {
			nb++
		}
	}
	fmt.Fprintf(out, "@@ -%s +%s @@\n", hunkRange(aStart, na), hunkRange(bStart, nb))
	for _, o := range ops[start:end] {
		out.WriteByte(o.op)
		out.WriteString(o.text)
		if !strings.HasSuffix(o.text, "\n") {
			out.WriteByte('\n')
		}
	}
}

// hunkRange formats one side of a hunk header the way GNU diff does: a length
// of 1 prints the start line only, and an empty side names the line before it.
func hunkRange(start, n int) string {
	switch n {
	case 1:
		return fmt.Sprint(start)
	case 0:
		return fmt.Sprintf("%d,0", start-1)
	}
	return fmt.Sprintf("%d,%d", start, n)
}
