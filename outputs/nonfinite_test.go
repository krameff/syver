package outputs

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/krameff/syver/matchers"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A result whose matcher values hold NaN or an infinity cannot be encoded by
// encoding/json. json used to drop the whole result and structured printed an
// empty line, both with the error discarded.
func nonFiniteResult() resource.TestResult {
	return resource.TestResult{
		ResourceType: "Command",
		ResourceId:   "nan-check",
		Property:     "stdout",
		Result:       resource.FAIL,
		MatcherResult: matchers.MatcherResult{
			Actual:        math.NaN(),
			Expected:      math.Inf(1),
			FoundElements: []any{1.5, math.Inf(-1)},
			Message:       "to equal",
		},
	}
}

func TestJSONOutputsSurviveNonFiniteValues(t *testing.T) {
	firstResult := func(t *testing.T, doc map[string]any) map[string]any {
		results, ok := doc["results"].([]any)
		require.True(t, ok, "results missing or not a list: %v", doc["results"])
		require.Len(t, results, 1)
		return results[0].(map[string]any)
	}

	tests := []struct {
		name     string
		outputer Outputer
		options  []string
		result   func(t *testing.T, doc map[string]any) map[string]any
	}{
		{"json", Json{}, nil, firstResult},
		{"json pretty", Json{}, []string{foPretty}, firstResult},
		{"structured", Structured{}, nil, firstResult},
		{"structured pretty", Structured{}, []string{foPretty}, firstResult},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			code := tt.outputer.Output(buf, makeResults(nonFiniteResult()), util.OutputConfig{FormatOptions: tt.options})
			assert.Equal(t, 1, code, "a failing result still exits 1")

			var doc map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &doc), "output is not JSON: %q", buf.String())
			r := tt.result(t, doc)
			assert.Equal(t, "nan-check", r["resource-id"])
			mr, ok := r["matcher-result"].(map[string]any)
			require.True(t, ok, "matcher-result missing: %v", r)
			assert.Equal(t, "NaN", mr["actual"])
			assert.Equal(t, "+Inf", mr["expected"])
			assert.Equal(t, []any{1.5, "-Inf"}, mr["found-elements"])
		})
	}
}

func TestJSONSafe(t *testing.T) {
	type named map[string]float64
	in := map[string]any{
		"nan":    math.NaN(),
		"f32":    float32(math.Inf(1)),
		"finite": 2.5,
		"nested": named{"x": math.Inf(-1)},
		"ptr":    func() *float64 { f := math.NaN(); return &f }(),
		"bytes":  []byte("hi"),
		"str":    "s",
	}
	got := jsonSafe(in).(map[string]any)
	assert.Equal(t, "NaN", got["nan"])
	assert.Equal(t, "+Inf", got["f32"])
	assert.Equal(t, 2.5, got["finite"])
	assert.Equal(t, map[string]any{"x": "-Inf"}, got["nested"])
	assert.Equal(t, "NaN", got["ptr"])
	assert.Equal(t, []byte("hi"), got["bytes"])
	assert.Equal(t, "s", got["str"])
	assert.Nil(t, jsonSafe(nil))
}
