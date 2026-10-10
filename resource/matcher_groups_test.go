package resource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// An `and` or `or` group whose children never ran used to panic while its
// failure was being rendered, because each read the child it recorded during
// Match without checking it was set. Two ways to get there: the enclosing
// transform errors first (a gjson path that does not exist), or a child errors
// (`have-key` against a plain string). ValidateSafe recovered the panic, so
// the run failed, but with a stack trace in place of the failure. Each case
// must fail normally and report the group itself.
//
// ValidateGomegaValue is called directly, with no recover, so a missing guard
// panics this test rather than being absorbed.
func TestMatcherGroupWhoseChildrenNeverRanReportsTheGroup(t *testing.T) {
	const doc = `{"this": {"is": {"just": {"a": "test"}}}}`
	cases := []struct {
		name    string
		actual  any
		matcher string
		message string
	}{
		{
			name:    "not or, gjson path does not exist",
			actual:  doc,
			matcher: `{"gjson": {"this.is.typo": {"not": {"or": [{"have-key": "a"}, {"have-key": "b"}]}}}}`,
			message: "not to satisfy any of these matchers",
		},
		{
			name:    "not or, child errors",
			actual:  "plain string",
			matcher: `{"not": {"or": [{"have-key": "a"}]}}`,
			message: "not to satisfy any of these matchers",
		},
		{
			name:    "and, gjson path does not exist",
			actual:  doc,
			matcher: `{"gjson": {"this.is.typo": {"and": [{"have-key": "a"}, {"have-key": "b"}]}}}`,
			message: "to satisfy all of these matchers",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var expected any
			if err := json.Unmarshal([]byte(c.matcher), &expected); err != nil {
				t.Fatal(err)
			}
			actual := func() (any, error) { return c.actual, nil }
			got := ValidateGomegaValue(&FakeResource{"example"}, "matches", expected, actual, false)
			assert.Equal(t, FAIL, got.Result)
			// The transform or child error is real and is reported as well.
			assert.NotNil(t, got.Err)
			assert.Equal(t, c.message, got.MatcherResult.Message)
		})
	}
}

// A named matcher group with nothing in it asserts nothing. `and`,
// `contain-elements` and `gjson` were vacuously true and `or` vacuously false,
// which `not:` inverts, so each could report a pass without the value ever
// being looked at. They are syntax errors now, at any depth.
func TestEmptyMatcherGroupIsASyntaxError(t *testing.T) {
	for _, in := range []string{
		`{"and": []}`,
		`{"or": []}`,
		`{"contain-elements": []}`,
		`{"gjson": {}}`,
		`{"not": {"or": []}}`,
		`{"not": {"and": []}}`,
		`{"gjson": {"a": {"and": []}}}`,
	} {
		t.Run(in, func(t *testing.T) {
			var dat any
			if err := json.Unmarshal([]byte(in), &dat); err != nil {
				t.Fatal(err)
			}
			got, err := matcherToGomegaMatcher(dat)
			assert.Nil(t, got)
			if assert.Error(t, err) {
				assert.ErrorIs(t, err, errEmptyMatcherGroup)
			}
		})
	}
}

// The empty forms that still mean something stay accepted. `consist-of: []`
// asserts the value is empty. A bare `[]` is what `syver add` writes for
// `stderr:` and `contents:`, and existing specs are full of it.
func TestEmptyFormsThatAssertSomethingAreAccepted(t *testing.T) {
	for _, in := range []string{
		`{"consist-of": []}`,
		`[]`,
	} {
		t.Run(in, func(t *testing.T) {
			var dat any
			if err := json.Unmarshal([]byte(in), &dat); err != nil {
				t.Fatal(err)
			}
			_, err := matcherToGomegaMatcher(dat)
			assert.NoError(t, err)
		})
	}
}
