package system

import (
	"strings"
	"testing"
)

// TestValueVsKeyWarningKeepsItsLevelPrefix pins the one property of the
// FEAT-012 W2-5 diagnostic that decides whether it is ever SEEN.
//
// logs.go filters the standard logger through logutils.LevelFilter, which
// classifies a line by the bracketed token it starts with and passes anything
// it cannot classify straight through. So a message that loses its "[WARN]"
// prefix, lowercases it, or puts anything before it does not go quiet -- it
// goes the other way and prints at every log level including --log-level=ERROR.
// Nothing else in the suite would notice, because the line still appears.
func TestValueVsKeyWarningKeepsItsLevelPrefix(t *testing.T) {
	got := registryValueVsKeyWarning(`HKLM\SOFTWARE\Example\ProfileList`, "ProfileList")

	if !strings.HasPrefix(got, "[WARN] ") {
		t.Fatalf("diagnostic = %q; it must START with the literal \"[WARN] \" or "+
			"logutils cannot classify it and it prints at every log level", got)
	}
}

// TestValueVsKeyWarningPrintsAPathAnOperatorCanType is decision 2 in the
// function's own comment, stated as a check rather than a claim.
//
// The whole point of the message is that it hands the reader the corrected
// path. %q would escape every separator, so HKLM\SOFTWARE\... would print as
// HKLM\\SOFTWARE\\... -- which is not the path, and a reader who pastes it gets
// a different error from the one they started with.
func TestValueVsKeyWarningPrintsAPathAnOperatorCanType(t *testing.T) {
	const key = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`

	got := registryValueVsKeyWarning(key, "ProfileList")

	if strings.Contains(got, `\\`) {
		t.Errorf("diagnostic contains a doubled backslash, so a path was rendered "+
			"with %%q: %s", got)
	}
	if !strings.Contains(got, key+`\`) {
		t.Errorf("diagnostic does not contain the corrected path %q, which is the "+
			"only actionable thing in it: %s", key+`\`, got)
	}
}

// TestValueVsKeyWarningQuotesTheValueName is decision 3, and it is the opposite
// of the rule above on purpose.
//
// The names that land an author in this diagnostic are the ones that do not
// look like anything: empty, or padded with spaces because a YAML key was
// written with a trailing blank. Unquoted, those are invisible and the message
// reads as nonsense.
func TestValueVsKeyWarningQuotesTheValueName(t *testing.T) {
	for _, name := range []string{"", " Trailing ", "ProfileList"} {
		got := registryValueVsKeyWarning(`HKLM\SOFTWARE\Example`, name)
		if !strings.Contains(got, `"`+name+`"`) {
			t.Errorf("value name %q is not quoted in the diagnostic, so it cannot be "+
				"seen: %s", name, got)
		}
	}
}
