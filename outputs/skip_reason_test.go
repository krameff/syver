package outputs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
)

// skip-reason is emitted only on skipped results. These cases pin all three
// shapes for every format that has somewhere to put it: a skip with a reason, a
// skip without one (which must render exactly as it did before the attribute
// existed), and a non-skipped result (which must never carry the field).

const skipReasonText = `exists did not pass & "quoted" <tag>`

func skipReasonResult(result int, reason string) resource.TestResult {
	now := time.Now()
	return resource.TestResult{
		Successful:   result == resource.SUCCESS,
		Skipped:      result == resource.SKIP,
		SkipReason:   reason,
		Result:       result,
		ResourceType: "File",
		ResourceId:   "/etc/probe",
		Property:     "mode",
		StartTime:    now,
		EndTime:      now,
	}
}

func renderSkipReason(t *testing.T, format string, tr resource.TestResult) string {
	t.Helper()
	o, err := GetOutputer(format)
	if err != nil {
		t.Fatalf("GetOutputer(%q): %v", format, err)
	}
	ch := make(chan []resource.TestResult, 1)
	ch <- []resource.TestResult{tr}
	close(ch)
	var buf bytes.Buffer
	o.Output(&buf, ch, util.OutputConfig{})
	return buf.String()
}

func TestSkipReasonHumanized(t *testing.T) {
	forceNoColor()
	got := humanizeResult(skipReasonResult(resource.SKIP, "exists did not pass"), true, true)
	if want := "File: /etc/probe: mode: skipped (exists did not pass)"; got != want {
		t.Errorf("with reason: got %q, want %q", got, want)
	}
	got = humanizeResult(skipReasonResult(resource.SKIP, ""), true, true)
	if want := "File: /etc/probe: mode: skipped"; got != want {
		t.Errorf("without reason: got %q, want %q", got, want)
	}
}

func TestSkipReasonDocumentationAndRspecish(t *testing.T) {
	for _, format := range []string{"documentation", "rspecish"} {
		t.Run(format, func(t *testing.T) {
			out := renderSkipReason(t, format, skipReasonResult(resource.SKIP, skipReasonText))
			if !strings.Contains(out, "File: /etc/probe: mode: skipped ("+skipReasonText+")") {
				t.Errorf("skipped with reason: reason missing from\n%s", out)
			}
			out = renderSkipReason(t, format, skipReasonResult(resource.SKIP, ""))
			if !strings.Contains(out, "File: /etc/probe: mode: skipped\n") {
				t.Errorf("skipped without reason: bare line changed in\n%s", out)
			}
			out = renderSkipReason(t, format, skipReasonResult(resource.SUCCESS, skipReasonText))
			if strings.Contains(out, "exists did not pass") {
				t.Errorf("non-skipped result rendered a skip reason:\n%s", out)
			}
		})
	}
}

func TestSkipReasonJSON(t *testing.T) {
	results := func(out string) []map[string]any {
		t.Helper()
		var doc struct {
			Results []map[string]any `json:"results"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("unmarshal: %v\n%s", err, out)
		}
		if len(doc.Results) != 1 {
			t.Fatalf("want 1 result, got %d", len(doc.Results))
		}
		return doc.Results
	}
	for _, format := range []string{"json", "structured"} {
		t.Run(format, func(t *testing.T) {
			r := results(renderSkipReason(t, format, skipReasonResult(resource.SKIP, skipReasonText)))[0]
			if r["skip-reason"] != skipReasonText {
				t.Errorf("skipped with reason: skip-reason = %v", r["skip-reason"])
			}
			r = results(renderSkipReason(t, format, skipReasonResult(resource.SKIP, "")))[0]
			if _, ok := r["skip-reason"]; ok {
				t.Errorf("skipped without reason: skip-reason present (want omitted)")
			}
			// A field set on a non-skipped result is a programming error upstream,
			// but the outputs still must not invent a key for passing results;
			// omitempty is what keeps them byte-identical, so check the normal
			// (empty) case explicitly too.
			r = results(renderSkipReason(t, format, skipReasonResult(resource.SUCCESS, "")))[0]
			if _, ok := r["skip-reason"]; ok {
				t.Errorf("passing result: skip-reason present (want omitted)")
			}
			r = results(renderSkipReason(t, format, skipReasonResult(resource.FAIL, "")))[0]
			if _, ok := r["skip-reason"]; ok {
				t.Errorf("failing result: skip-reason present (want omitted)")
			}
		})
	}
}

func TestSkipReasonJUnit(t *testing.T) {
	out := renderSkipReason(t, "junit", skipReasonResult(resource.SKIP, skipReasonText))
	want := `<skipped message="exists did not pass &amp; &#34;quoted&#34; &lt;tag&gt;"/>`
	if !strings.Contains(out, want) {
		t.Errorf("skipped with reason: want %s in\n%s", want, out)
	}
	out = renderSkipReason(t, "junit", skipReasonResult(resource.SKIP, ""))
	if !strings.Contains(out, "<skipped/>") || strings.Contains(out, "message=") {
		t.Errorf("skipped without reason: want a bare <skipped/> in\n%s", out)
	}
	out = renderSkipReason(t, "junit", skipReasonResult(resource.SUCCESS, skipReasonText))
	if strings.Contains(out, "<skipped") {
		t.Errorf("non-skipped result rendered <skipped>:\n%s", out)
	}
}

func TestSkipReasonTAP(t *testing.T) {
	forceNoColor()
	out := renderSkipReason(t, "tap", skipReasonResult(resource.SKIP, "exists did not pass"))
	if want := "ok 1 - # SKIP File: /etc/probe: mode: skipped (exists did not pass)\n"; !strings.Contains(out, want) {
		t.Errorf("skipped with reason: want %q in\n%s", want, out)
	}
	out = renderSkipReason(t, "tap", skipReasonResult(resource.SKIP, ""))
	if want := "ok 1 - # SKIP File: /etc/probe: mode: skipped\n"; !strings.Contains(out, want) {
		t.Errorf("skipped without reason: want %q in\n%s", want, out)
	}
	out = renderSkipReason(t, "tap", skipReasonResult(resource.SUCCESS, "exists did not pass"))
	if strings.Contains(out, "exists did not pass") || strings.Contains(out, "SKIP") {
		t.Errorf("non-skipped result rendered a skip:\n%s", out)
	}
}
