package outputs

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/krameff/syver/matchers"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
)

// renderWith runs the named outputer over a passing zzz-id followed by a
// failing aaa-id whose transform chain carries a raw value of RAWMARKER.
// verbose is always passed so nagios prints its per-check lines; every other
// format either honours it or ignores it. Digits are stripped from the result
// so durations and timestamps cannot make two renders differ.
func renderWith(t *testing.T, name string, opts ...string) string {
	t.Helper()
	defer resetMetrics()
	o, err := GetOutputer(name)
	if err != nil {
		t.Fatalf("GetOutputer(%q): %v", name, err)
	}
	result := func(id string, outcome int) resource.TestResult {
		return resource.TestResult{
			Successful:   outcome == resource.SUCCESS,
			Result:       outcome,
			ResourceType: "Command",
			ResourceId:   id,
			Property:     "stdout",
			StartTime:    time.Now(),
			EndTime:      time.Now(),
			MatcherResult: matchers.MatcherResult{
				Actual:             "1",
				Expected:           "2",
				Message:            "to equal",
				TransformerChain:   []matchers.Transformer{matchers.ToNumeric{}},
				UntransformedValue: "RAWMARKER",
			},
		}
	}
	ch := make(chan []resource.TestResult, 2)
	ch <- []resource.TestResult{result("zzz-id", resource.SUCCESS)}
	ch <- []resource.TestResult{result("aaa-id", resource.FAIL)}
	close(ch)
	var b bytes.Buffer
	o.Output(&b, ch, util.OutputConfig{FormatOptions: append([]string{foVerbose}, opts...)})
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, b.String())
}

// Every format's advertised options must match what it actually does with
// exclude_raw and sort. Driven off Outputers() so a new format, or a format
// that starts or stops honouring either option, is checked without editing
// this test. The help text and CheckFormatOptions both read ValidOptionsFor,
// so a mismatch here means the help lies or a working option draws a warning.
func TestValidOptionsForMatchesWhatEachFormatHonours(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	for _, name := range Outputers() {
		if name == "discovery" { // not an Outputer; covered below
			continue
		}
		t.Run(name, func(t *testing.T) {
			advertised := ValidOptionsFor(name)

			base := renderWith(t, name)
			excluded := renderWith(t, name, foExcludeRaw)
			honoursExcludeRaw := strings.Count(excluded, "RAWMARKER") < strings.Count(base, "RAWMARKER")
			assert.Equal(t, honoursExcludeRaw, util.IsValueInList(foExcludeRaw, advertised),
				"%s: honours exclude_raw=%v but ValidOptionsFor says %v", name, honoursExcludeRaw, advertised)

			// rspecish names neither id in its progress line, but sort still
			// reorders it (".F" becomes "F."), so compare whole renders.
			honoursSort := renderWith(t, name, foSort) != base
			assert.Equal(t, honoursSort, util.IsValueInList(foSort, advertised),
				"%s: honours sort=%v but ValidOptionsFor says %v", name, honoursSort, advertised)
		})
	}

	t.Run("discovery", func(t *testing.T) {
		render := func(opts ...string) string {
			var b bytes.Buffer
			Discovery{}.Output(&b, map[string]bool{"x": true}, util.OutputConfig{FormatOptions: opts})
			return b.String()
		}
		assert.NotEqual(t, render(), render(foPretty), "discovery should honour pretty")
		assert.Equal(t, []string{foPretty}, ValidOptionsFor("discovery"))
	})
}

func TestCheckFormatOptions(t *testing.T) {
	t.Run("unknown option is reported, not rejected", func(t *testing.T) {
		unknown, ignored := CheckFormatOptions("rspecish", []string{"bogus", "sort"})
		assert.Equal(t, []string{"bogus"}, unknown)
		assert.Empty(t, ignored)
	})
	t.Run("option valid for another format is ignored", func(t *testing.T) {
		unknown, ignored := CheckFormatOptions("rspecish", []string{"pretty", "sort"})
		assert.Empty(t, unknown)
		assert.Equal(t, []string{"pretty"}, ignored)
	})
	t.Run("matching is case-insensitive like the formats", func(t *testing.T) {
		unknown, ignored := CheckFormatOptions("json", []string{"PRETTY", "Exclude_Raw"})
		assert.Empty(t, unknown)
		assert.Empty(t, ignored)
	})
	t.Run("empty values are skipped", func(t *testing.T) {
		unknown, ignored := CheckFormatOptions("json", []string{"", "  "})
		assert.Empty(t, unknown)
		assert.Empty(t, ignored)
	})
	t.Run("unknown format reports unknown names only", func(t *testing.T) {
		unknown, ignored := CheckFormatOptions("nope", []string{"pretty", "bogus"})
		assert.Equal(t, []string{"bogus"}, unknown)
		assert.Empty(t, ignored)
	})
}

func TestNormalizeFormatOptions(t *testing.T) {
	assert.Equal(t, []string{"pretty", "sort"}, NormalizeFormatOptions([]string{" pretty", " sort ", "", "\t"}))
	unknown, ignored := CheckFormatOptions("json", []string{" sort", "pretty "})
	assert.Empty(t, unknown)
	assert.Empty(t, ignored)
}
