package outputs

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// meta: is free-form, so JUnit carries it as testcase-level <properties>,
// flattened to one <property> per leaf. These tests pin the flattening rule,
// the ordering, the escaping and the fact that a result with no meta renders
// exactly as it did before properties existed.

func TestFlattenMeta(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want []metaProperty
	}{
		{"nil map", nil, nil},
		{"empty map", map[string]any{}, nil},
		{
			"scalars of each type",
			map[string]any{"s": "str", "i": 3, "f": 1.5, "b": true, "n": nil},
			[]metaProperty{{"b", "true"}, {"f", "1.5"}, {"i", "3"}, {"n", ""}, {"s", "str"}},
		},
		{
			"nested map, keys sorted at every level",
			map[string]any{"owner": map[string]any{"team": "sec", "contact": "x@y"}, "a": 1},
			[]metaProperty{{"a", "1"}, {"owner.contact", "x@y"}, {"owner.team", "sec"}},
		},
		{
			"slice is indexed",
			map[string]any{"tags": []any{"pci", "cis"}},
			[]metaProperty{{"tags[0]", "pci"}, {"tags[1]", "cis"}},
		},
		{
			"slice of maps",
			map[string]any{"refs": []any{map[string]any{"id": "AC-2"}, map[string]any{"id": "AC-3", "rev": 5}}},
			[]metaProperty{{"refs[0].id", "AC-2"}, {"refs[1].id", "AC-3"}, {"refs[1].rev", "5"}},
		},
		{
			"nested slice",
			map[string]any{"m": []any{[]any{"a", "b"}}},
			[]metaProperty{{"m[0][0]", "a"}, {"m[0][1]", "b"}},
		},
		{
			"empty nested map and slice keep their path",
			map[string]any{"e": map[string]any{}, "l": []any{}},
			[]metaProperty{{"e", ""}, {"l", ""}},
		},
		{
			"non-string map keys",
			map[string]any{"m": map[any]any{2: "two", 1: "one"}},
			[]metaProperty{{"m.1", "one"}, {"m.2", "two"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, flattenMeta(tc.in))
		})
	}
}

// A cyclic structure must not hang the run: past the depth cap the path is
// emitted once with a marker and recursion stops.
func TestFlattenMetaDepthCap(t *testing.T) {
	cyclic := map[string]any{}
	cyclic["self"] = cyclic

	done := make(chan []metaProperty, 1)
	go func() { done <- flattenMeta(cyclic) }()
	select {
	case got := <-done:
		require.Len(t, got, 1)
		assert.Equal(t, strings.Repeat("self.", metaMaxDepth-1)+"self", got[0].Name)
		assert.Equal(t, metaTruncated, got[0].Value)
	case <-time.After(5 * time.Second):
		t.Fatal("flattenMeta did not terminate on a cyclic map")
	}
}

func metaResult(m map[string]any) resource.TestResult {
	now := time.Now()
	return resource.TestResult{
		Successful:   true,
		Result:       resource.SUCCESS,
		ResourceType: "File",
		ResourceId:   "/etc/probe",
		Property:     "mode",
		Meta:         m,
		StartTime:    now,
		EndTime:      now,
	}
}

func renderJUnit(t *testing.T, trs ...resource.TestResult) string {
	t.Helper()
	ch := make(chan []resource.TestResult, 1)
	ch <- trs
	close(ch)
	var buf bytes.Buffer
	JUnit{}.Output(&buf, ch, util.OutputConfig{})
	return buf.String()
}

func TestJUnitMetaProperties(t *testing.T) {
	forceNoColor()
	got := renderJUnit(t, metaResult(map[string]any{
		"framework": "800-53r5",
		"control":   "SC-12",
		"tags":      []any{"a"},
	}))
	want := "<testcase name=\"File /etc/probe mode\" time=\"0.000\">\n" +
		"<properties>\n" +
		"<property name=\"control\" value=\"SC-12\"/>\n" +
		"<property name=\"framework\" value=\"800-53r5\"/>\n" +
		"<property name=\"tags[0]\" value=\"a\"/>\n" +
		"</properties>\n" +
		"<system-out>"
	assert.Contains(t, got, want)
}

// No meta, nil meta and an empty meta: map all render with no <properties>
// element at all, so specs that do not use meta: produce byte-identical output.
func TestJUnitNoMetaNoProperties(t *testing.T) {
	forceNoColor()
	for name, m := range map[string]map[string]any{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			got := renderJUnit(t, metaResult(m))
			assert.NotContains(t, got, "properties")
			assert.Contains(t, got, "time=\"0.000\">\n<system-out>")
		})
	}
}

// Names and values are both user-authored, so both are escaped, and the result
// must still parse. A control byte that XML 1.0 cannot carry becomes U+FFFD.
func TestJUnitMetaEscapedAndWellFormed(t *testing.T) {
	forceNoColor()
	got := renderJUnit(t, metaResult(map[string]any{
		`k<&"'` + "\x01": `v<&"'` + "\x00\t\n",
	}))
	assert.Contains(t, got,
		`<property name="k&lt;&amp;&#34;&#39;`+"�"+`" value="v&lt;&amp;&#34;&#39;`+"�"+`&#x9;&#xA;"/>`)

	var doc struct {
		Cases []struct {
			Props []struct {
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"properties>property"`
		} `xml:"testcase"`
	}
	require.NoError(t, xml.Unmarshal([]byte(got), &doc))
	require.Len(t, doc.Cases, 1)
	require.Len(t, doc.Cases[0].Props, 1)
	assert.Equal(t, `k<&"'`+"�", doc.Cases[0].Props[0].Name)
	assert.Equal(t, `v<&"'`+"�\t\n", doc.Cases[0].Props[0].Value)
}

// Go randomises map iteration, so an unsorted implementation passes a single
// render by luck. Rendering many times and requiring identical bytes catches it.
func TestJUnitMetaDeterministic(t *testing.T) {
	forceNoColor()
	m := map[string]any{}
	for _, k := range strings.Split("q w e r t y u i o p a s d f g h j k l z", " ") {
		m[k] = map[string]any{"x": k, "y": k, "z": k}
	}
	tr := metaResult(m)
	first := renderJUnit(t, tr)
	for i := 0; i < 20; i++ {
		got := renderJUnit(t, tr)
		// strip the run timestamp, which is the only legitimately varying part
		require.Equal(t, stripTimestamp(first), stripTimestamp(got))
	}
}

func stripTimestamp(s string) string {
	i := strings.Index(s, "timestamp=\"")
	if i < 0 {
		return s
	}
	j := strings.Index(s[i+len("timestamp=\""):], "\"")
	return s[:i] + s[i+len("timestamp=\"")+j:]
}
