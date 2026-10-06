package matchers

import (
	"encoding/json"
)

type AndMatcher struct {
	fakeOmegaMatcher
	Matchers []SyverMatcher

	// state
	firstFailedMatcher SyverMatcher
}

func And(ms ...SyverMatcher) SyverMatcher {
	return &AndMatcher{Matchers: ms}
}

func (m *AndMatcher) Match(actual interface{}) (success bool, err error) {
	m.firstFailedMatcher = nil
	for _, matcher := range m.Matchers {
		success, err := matcher.Match(actual)
		if !success || err != nil {
			m.firstFailedMatcher = matcher
			return false, err
		}
	}
	return true, nil
}

func (m *AndMatcher) FailureResult(actual interface{}) MatcherResult {
	// Unset when Match never ran a child: the enclosing transform errored
	// first, as with a gjson path that does not exist. Report the group itself
	// rather than dereference nil.
	if m.firstFailedMatcher == nil {
		return MatcherResult{
			Actual:   actual,
			Message:  "to satisfy all of these matchers",
			Expected: m.Matchers,
		}
	}
	return m.firstFailedMatcher.FailureResult(actual)
}

func (m *AndMatcher) NegatedFailureResult(actual interface{}) MatcherResult {
	return MatcherResult{
		Actual:   actual,
		Message:  "not to satisfy all of these matchers",
		Expected: m.Matchers,
	}
}

func (m *AndMatcher) MarshalJSON() ([]byte, error) {
	if len(m.Matchers) == 1 {
		return json.Marshal(m.Matchers[0])
	}
	j := make(map[string]interface{})
	j["and"] = m.Matchers
	return json.Marshal(j)
}
