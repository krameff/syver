package syver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

// validateJSONResults runs spec through the real Validate entry point with the
// json outputer and returns the results keyed by "resource-id property".
func validateJSONResults(t *testing.T, spec string) map[string]map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goss.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	var out bytes.Buffer
	cfg, err := util.NewConfig(
		util.WithSpecFile(path),
		util.WithOutputFormat("json"),
		util.WithResultWriter(&out),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if _, err := Validate(t.Context(), cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	var doc struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out.String())
	}
	byKey := make(map[string]map[string]any, len(doc.Results))
	for _, r := range doc.Results {
		byKey[fmt.Sprintf("%v %v", r["resource-id"], r["property"])] = r
	}
	return byKey
}

func TestValidateSkipReasonOnDependsOnSkip(t *testing.T) {
	missing := filepath.ToSlash(filepath.Join(t.TempDir(), "missing"))
	got := validateJSONResults(t, fmt.Sprintf(`file:
  prereq:
    path: %s
    exists: true
  dependent:
    path: %s
    exists: true
    depends-on:
      - file:prereq
`, missing, missing))
	r, ok := got["dependent: "+missing+" depends-on"]
	if !ok {
		t.Fatalf("no depends-on result; got keys %v", keysOf(got))
	}
	want := `dependency "file:prereq" failed`
	if r["skip-reason"] != want {
		t.Errorf("skip-reason = %v, want %q", r["skip-reason"], want)
	}
	if r["err"] != want {
		t.Errorf("err = %v, want %q (kept alongside skip-reason)", r["err"], want)
	}
	if !strings.HasSuffix(fmt.Sprint(r["summary-line-compact"]), "skipped ("+want+")") {
		t.Errorf("summary-line-compact = %v", r["summary-line-compact"])
	}
	if _, ok := got["prereq: "+missing+" exists"]["skip-reason"]; ok {
		t.Errorf("failing prereq carries a skip-reason")
	}
}

func TestValidateSkipReasonOnPromotedAndAuthorSkips(t *testing.T) {
	missing := filepath.ToSlash(filepath.Join(t.TempDir(), "missing"))
	got := validateJSONResults(t, fmt.Sprintf(`file:
  promoted:
    path: %s
    exists: true
    mode: "0644"
  authored:
    path: %s
    exists: true
    skip: true
    skip-reason: not applicable here
  bare:
    path: %s
    exists: true
    skip: true
`, missing, missing, missing))
	checks := map[string]any{
		"promoted: " + missing + " mode":   "exists did not pass",
		"authored: " + missing + " exists": "not applicable here",
		"bare: " + missing + " exists":     nil,
		"promoted: " + missing + " exists": nil,
	}
	for key, want := range checks {
		r, ok := got[key]
		if !ok {
			t.Errorf("missing result %q; got keys %v", key, keysOf(got))
			continue
		}
		if r["skip-reason"] != want {
			t.Errorf("%s: skip-reason = %v, want %v", key, r["skip-reason"], want)
		}
	}
}

// Strict per-resource decoding must accept skip-reason on every registered
// type, including the gossfile include. Driven off the registry so a new type
// is covered without editing this test.
func TestSkipReasonAcceptedByEveryResourceType(t *testing.T) {
	keys := make([]string, 0)
	for key := range resource.Descriptors() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Fatal("no registered resource types")
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			spec := fmt.Sprintf("%s:\n  probe:\n    skip: true\n    skip-reason: because\n", key)
			cfg, err := ReadJSONData([]byte(spec), true, "goss.yaml")
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			var res any
			if key == resource.SyverFileResourceKey {
				res = cfg.Syverfiles["probe"]
			} else {
				for _, r := range cfg.Resources() {
					if r.TypeKey() == key {
						res = r
					}
				}
			}
			sr, ok := res.(interface{ GetSkipReason() string })
			if !ok {
				t.Fatalf("decoded %T does not expose GetSkipReason", res)
			}
			if got := sr.GetSkipReason(); got != "because" {
				t.Errorf("GetSkipReason() = %q, want %q", got, "because")
			}
		})
	}
}

func keysOf(m map[string]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// A multi-line skip-reason (a `|` or `>` block) must not break line-oriented
// output. Before the reason was collapsed, a `|` block put raw newlines into the
// TAP line, so text in the reason became TAP lines of its own: here an extra
// `ok 99` and a `Bail out!`, which a TAP consumer acts on.
func TestValidateSkipReasonMultiLineStaysOnOneLine(t *testing.T) {
	missing := filepath.ToSlash(filepath.Join(t.TempDir(), "missing"))
	spec := fmt.Sprintf(`file:
  literal:
    path: %s
    exists: true
    skip: true
    skip-reason: |
      first line
      ok 99 - injected
      Bail out!
  folded:
    path: %s
    exists: true
    skip: true
    skip-reason: >
      folded
      text

      after a blank line
`, missing, missing)
	path := filepath.Join(t.TempDir(), "goss.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	run := func(format string) string {
		var out bytes.Buffer
		cfg, err := util.NewConfig(util.WithSpecFile(path), util.WithOutputFormat(format),
			util.WithResultWriter(&out), util.WithNoColor())
		if err != nil {
			t.Fatalf("new config: %v", err)
		}
		if _, err := Validate(t.Context(), cfg); err != nil {
			t.Fatalf("validate: %v", err)
		}
		return out.String()
	}

	tap := strings.Split(strings.TrimRight(run("tap"), "\n"), "\n")
	if len(tap) != 3 || tap[0] != "1..2" {
		t.Fatalf("want a plan line and 2 test lines, got %d lines:\n%s", len(tap), strings.Join(tap, "\n"))
	}
	wantLines := map[string]bool{
		"ok 1 - # SKIP File: literal: " + missing + ": exists: skipped (first line ok 99 - injected Bail out!)": true,
		"ok 2 - # SKIP File: folded: " + missing + ": exists: skipped (folded text after a blank line)":         true,
	}
	for _, line := range tap[1:] {
		// TAP numbers results in output order, which follows completion order.
		trimmed := line[strings.Index(line, " - "):]
		found := false
		for want := range wantLines {
			if want[strings.Index(want, " - "):] == trimmed {
				found = true
			}
		}
		if !found {
			t.Errorf("unexpected TAP line %q", line)
		}
	}

	doc := run("documentation")
	for _, line := range strings.Split(doc, "\n") {
		if line == ")" || strings.HasPrefix(line, "ok 99") || strings.HasPrefix(line, "Bail out!") {
			t.Errorf("reason text escaped onto its own documentation line: %q\n%s", line, doc)
		}
	}

	got := validateJSONResults(t, spec)
	r := got["literal: "+missing+" exists"]
	if r["skip-reason"] != "first line ok 99 - injected Bail out!" {
		t.Errorf("json skip-reason = %q", r["skip-reason"])
	}
}

// depends-on reasons must name the prerequisite accurately and must not depend
// on the order resources reach the scheduler. With a failing, b depends-on a and
// c depends-on b, c used to get either `dependency "file:b" failed` (b was
// skipped, not failed) or `unsatisfied dependency`, depending on order.
func TestDependsOnSkipReasonsAreAccurateAndOrderIndependent(t *testing.T) {
	sys := system.New("")
	missing := filepath.Join(t.TempDir(), "missing")
	present := t.TempDir()

	newFile := func(id, path string, deps ...string) *resource.File {
		f := &resource.File{Path: path, Exists: true, DiscoveryMeta: resource.DiscoveryMeta{DependsOn: deps}}
		f.SetID(id)
		return f
	}
	want := map[string]string{
		"a":  "",
		"ok": "",
		"b":  `dependency "file:a" failed`,
		"c":  `dependency "file:b" was skipped`,
		// first prerequisite that did not pass, in declared order
		"d": `dependency "file:c" was skipped`,
		"e": `dependency "file:a" failed`,
	}
	build := func() []resource.Resource {
		return []resource.Resource{
			newFile("a", missing),
			newFile("ok", present),
			newFile("b", present, "a"),
			newFile("c", present, "b"),
			newFile("d", present, "ok", "c", "b", "a"),
			newFile("e", present, "ok", "a", "d"),
		}
	}

	// Every rotation and its reverse, a few times each: the old reasons changed
	// with the pending order, so a fixed order alone would not catch it.
	n := len(build())
	for rot := 0; rot < n; rot++ {
		for _, reverse := range []bool{false, true} {
			for rep := 0; rep < 3; rep++ {
				base := build()
				ordered := make([]resource.Resource, n)
				for i := range base {
					j := (i + rot) % n
					if reverse {
						j = n - 1 - j
					}
					ordered[j] = base[i]
				}
				out, err := validateWithDependencies(t.Context(), sys, ordered, 2)
				if err != nil {
					t.Fatalf("validate with dependencies: %v", err)
				}
				got := map[string]string{}
				var count, skipped, failed int
				for group := range out {
					for _, r := range group {
						count++
						if r.Skipped {
							skipped++
							if r.Err == nil || string(*r.Err) != r.SkipReason {
								t.Errorf("rot %d rev %v: %s: err %v != skip-reason %q", rot, reverse, r.ResourceId, r.Err, r.SkipReason)
							}
						}
						if r.Result == resource.FAIL {
							failed++
						}
						got[strings.SplitN(r.ResourceId, ":", 2)[0]] = r.SkipReason
					}
				}
				if count != 6 || skipped != 4 || failed != 1 {
					t.Errorf("rot %d rev %v: count=%d skipped=%d failed=%d, want 6/4/1", rot, reverse, count, skipped, failed)
				}
				for id, w := range want {
					if got[id] != w {
						t.Errorf("rot %d rev %v: %s: skip-reason %q, want %q", rot, reverse, id, got[id], w)
					}
				}
			}
		}
	}
}
