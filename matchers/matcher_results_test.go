package matchers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMatcherResults pins, for each matcher, the failure and negated-failure
// results the output formatters render, and the JSON the matcher marshals to
// (the "expected" side shown in json output). Match runs first because some
// matchers build their failure result from state Match leaves behind.
func TestMatcherResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		matcher     SyverMatcher
		actual      interface{}
		wantMatch   bool
		wantFailure MatcherResult
		wantNegated MatcherResult
		wantJSON    string
	}{
		{
			name:        "equal",
			matcher:     Equal("a"),
			actual:      "b",
			wantFailure: MatcherResult{Actual: "b", Message: "to equal", Expected: "a"},
			wantNegated: MatcherResult{Actual: "b", Message: "not to equal", Expected: "a"},
			wantJSON:    `"a"`,
		},
		{
			name:        "be-numerically",
			matcher:     BeNumerically("gt", 5),
			actual:      3,
			wantFailure: MatcherResult{Actual: 3, Message: "to be numerically gt", Expected: 5},
			wantNegated: MatcherResult{Actual: 3, Message: "not to be numerically gt", Expected: 5},
			wantJSON:    `{"gt":5}`,
		},
		{
			name:        "contain-element",
			matcher:     ContainElement("x"),
			actual:      []string{"a"},
			wantFailure: MatcherResult{Actual: []string{"a"}, Message: "to contain element matching", Expected: "x"},
			wantNegated: MatcherResult{Actual: []string{"a"}, Message: "not to contain element matching", Expected: "x"},
			wantJSON:    `{"contain-element":"x"}`,
		},
		{
			name:    "contain-elements",
			matcher: ContainElements("a", "x"),
			actual:  []string{"a", "b"},
			wantFailure: MatcherResult{
				Actual: []string{"a", "b"}, Message: "to contain elements matching",
				Expected:        []interface{}{"a", "x"},
				MissingElements: []interface{}{"x"},
				FoundElements:   []interface{}{"a"},
			},
			wantNegated: MatcherResult{Actual: []string{"a", "b"}, Message: "not to contain elements matching", Expected: []interface{}{"a", "x"}},
			wantJSON:    `{"contain-elements":["a","x"]}`,
		},
		{
			name:    "consist-of",
			matcher: ConsistOf("a", "x"),
			actual:  []string{"a", "b"},
			wantFailure: MatcherResult{
				Actual: []string{"a", "b"}, Message: "to consist of",
				Expected:        []interface{}{"a", "x"},
				MissingElements: []interface{}{"x"},
				ExtraElements:   []interface{}{"b"},
				FoundElements:   []interface{}{"a"},
			},
			wantNegated: MatcherResult{Actual: []string{"a", "b"}, Message: "not to consist of", Expected: []interface{}{"a", "x"}},
			wantJSON:    `{"consist-of":["a","x"]}`,
		},
		{
			name:        "contain-substring",
			matcher:     ContainSubstring("zz"),
			actual:      "abc",
			wantFailure: MatcherResult{Actual: "abc", Message: "to contain substring", Expected: "zz"},
			wantNegated: MatcherResult{Actual: "abc", Message: "not to contain substring", Expected: "zz"},
			wantJSON:    `{"contain-substring":"zz"}`,
		},
		{
			name:        "have-key",
			matcher:     HaveKey("k"),
			actual:      map[string]int{"j": 1},
			wantFailure: MatcherResult{Actual: map[string]int{"j": 1}, Message: "to have key matching", Expected: "k"},
			wantNegated: MatcherResult{Actual: map[string]int{"j": 1}, Message: "not to have key matching", Expected: "k"},
			wantJSON:    `{"have-key":"k"}`,
		},
		{
			name:        "have-len",
			matcher:     HaveLen(2),
			actual:      []int{1},
			wantFailure: MatcherResult{Actual: []int{1}, Message: "to have length", Expected: 2},
			wantNegated: MatcherResult{Actual: []int{1}, Message: "not to have length", Expected: 2},
			wantJSON:    `{"have-len":2}`,
		},
		{
			name:        "have-prefix",
			matcher:     HavePrefix("x"),
			actual:      "abc",
			wantFailure: MatcherResult{Actual: "abc", Message: "to have prefix", Expected: "x"},
			wantNegated: MatcherResult{Actual: "abc", Message: "not to have prefix", Expected: "x"},
			wantJSON:    `{"have-prefix":"x"}`,
		},
		{
			name:        "have-suffix",
			matcher:     HaveSuffix("x"),
			actual:      "abc",
			wantFailure: MatcherResult{Actual: "abc", Message: "to have suffix", Expected: "x"},
			wantNegated: MatcherResult{Actual: "abc", Message: "not to have suffix", Expected: "x"},
			wantJSON:    `{"have-suffix":"x"}`,
		},
		{
			name:        "match-regexp",
			matcher:     MatchRegexp("^z"),
			actual:      "abc",
			wantFailure: MatcherResult{Actual: "abc", Message: "to match regular expression", Expected: "^z"},
			wantNegated: MatcherResult{Actual: "abc", Message: "not to match regular expression", Expected: "^z"},
			wantJSON:    `{"match-regexp":"^z"}`,
		},
		{
			// Not swaps the inner matcher's two results.
			name:        "not",
			matcher:     Not(Equal("b")),
			actual:      "b",
			wantFailure: MatcherResult{Actual: "b", Message: "not to equal", Expected: "b"},
			wantNegated: MatcherResult{Actual: "b", Message: "to equal", Expected: "b"},
			wantJSON:    `{"not":"b"}`,
		},
		{
			// And reports the first child that failed, not the group.
			name:        "and",
			matcher:     And(HavePrefix("a"), HaveSuffix("x")),
			actual:      "abc",
			wantFailure: MatcherResult{Actual: "abc", Message: "to have suffix", Expected: "x"},
			wantNegated: MatcherResult{
				Actual: "abc", Message: "not to satisfy all of these matchers",
				Expected: []SyverMatcher{HavePrefix("a"), HaveSuffix("x")},
			},
			wantJSON: `{"and":[{"have-prefix":"a"},{"have-suffix":"x"}]}`,
		},
		{
			name:        "or",
			matcher:     Or(Equal("x"), Equal("y")),
			actual:      "abc",
			wantFailure: MatcherResult{Actual: "abc", Message: "to satisfy at least one of these matchers", Expected: []SyverMatcher{Equal("x"), Equal("y")}},
			wantNegated: MatcherResult{Actual: "abc", Message: "not to satisfy any of these matchers", Expected: []SyverMatcher{Equal("x"), Equal("y")}},
			wantJSON:    `{"or":["x","y"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := tt.matcher.Match(tt.actual)
			require.NoError(t, err)
			require.Equal(t, tt.wantMatch, ok)

			assert.Equal(t, tt.wantFailure, tt.matcher.FailureResult(tt.actual), "FailureResult")
			assert.Equal(t, tt.wantNegated, tt.matcher.NegatedFailureResult(tt.actual), "NegatedFailureResult")

			b, err := json.Marshal(tt.matcher)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(b), "MarshalJSON")
		})
	}
}

// TestAndSingleChildMarshalsAsChild pins that an and-group of one marshals as
// that child alone, which is how a bare matcher in a spec round-trips.
func TestAndSingleChildMarshalsAsChild(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(And(Equal(1)))
	require.NoError(t, err)
	assert.JSONEq(t, `1`, string(b))
}

// TestOrNegatedReportsMatchingChild pins that a negated or-group reports the
// child that matched, not the group.
func TestOrNegatedReportsMatchingChild(t *testing.T) {
	t.Parallel()
	m := Or(Equal("x"), Equal("abc"))
	ok, err := m.Match("abc")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, MatcherResult{Actual: "abc", Message: "not to equal", Expected: "abc"}, m.NegatedFailureResult("abc"))
}

// TestWithSafeTransformResult pins that a transformed failure reports the
// inner matcher's message against the transformed value, carries the chain of
// transforms that changed the value, keeps the original value, and marshals
// as the inner matcher.
func TestWithSafeTransformResult(t *testing.T) {
	t.Parallel()

	m := WithSafeTransform(ToNumeric{}, BeNumerically("eq", 2))
	ok, err := m.Match("1\n")
	require.NoError(t, err)
	require.False(t, ok)

	got := m.FailureResult("1\n")
	assert.Equal(t, MatcherResult{
		Actual:             float64(1),
		Message:            "to be numerically eq",
		Expected:           2,
		TransformerChain:   []Transformer{ToNumeric{}},
		UntransformedValue: "1\n",
	}, got)

	b, err := json.Marshal(m)
	require.NoError(t, err)
	assert.JSONEq(t, `{"eq":2}`, string(b))

	// A transform that errors is reported with the transformer named.
	_, err = WithSafeTransform(Gjson{Path: "a"}, Equal(1)).Match("not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid json")
}

func TestBeNumericallyUnknownComparator(t *testing.T) {
	t.Parallel()
	_, err := BeNumerically("approx", 1).Match(1)
	require.Error(t, err)
	assert.Equal(t, "unknown comparator: approx", err.Error())
}

// TestBeNumericallyComparators pins each comparator at its boundary: below,
// equal to and above the expected value. Only the word forms are accepted;
// "ne" and the symbolic forms are rejected rather than guessed at.
func TestBeNumericallyComparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		comparator          string
		below, equal, above bool
	}{
		{"eq", false, true, false},
		{"lt", true, false, false},
		{"le", true, true, false},
		{"gt", false, false, true},
		{"ge", false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.comparator, func(t *testing.T) {
			for _, c := range []struct {
				actual interface{}
				want   bool
			}{{4, tt.below}, {5, tt.equal}, {6, tt.above}, {4.999, tt.below}, {5.0, tt.equal}, {5.001, tt.above}} {
				got, err := BeNumerically(tt.comparator, 5).Match(c.actual)
				require.NoError(t, err)
				assert.Equal(t, c.want, got, "%v %s 5", c.actual, tt.comparator)
			}
		})
	}

	for _, bad := range []string{"ne", "==", "<", "<=", ">", ">=", "EQ"} {
		_, err := BeNumerically(bad, 5).Match(5)
		require.Error(t, err, "comparator %q", bad)
		assert.Equal(t, "unknown comparator: "+bad, err.Error())
	}
}

// TestGroupResultsWhenNoChildRan covers and/or reporting when the enclosing
// transform errored before any child ran (here a gjson path on invalid JSON),
// so there is no failed or matching child to report. The group itself is
// reported instead of dereferencing nil.
func TestGroupResultsWhenNoChildRan(t *testing.T) {
	t.Parallel()

	and := And(Equal("a"), Equal("b"))
	m := WithSafeTransform(Gjson{Path: "x"}, and)
	_, err := m.Match("not json")
	require.Error(t, err)
	assert.Equal(t, MatcherResult{
		Actual:             nil,
		Message:            "to satisfy all of these matchers",
		Expected:           []SyverMatcher{Equal("a"), Equal("b")},
		TransformerChain:   []Transformer{Gjson{Path: "x"}},
		UntransformedValue: "not json",
	}, m.FailureResult("not json"))

	or := Or(Equal("a"), Equal("b"))
	m = WithSafeTransform(Gjson{Path: "x"}, or)
	_, err = m.Match("not json")
	require.Error(t, err)
	assert.Equal(t, MatcherResult{
		Actual:             nil,
		Message:            "not to satisfy any of these matchers",
		Expected:           []SyverMatcher{Equal("a"), Equal("b")},
		TransformerChain:   []Transformer{Gjson{Path: "x"}},
		UntransformedValue: "not json",
	}, m.NegatedFailureResult("not json"))
}
