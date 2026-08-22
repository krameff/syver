package resource

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/krameff/syver/matchers"
)

const (
	Value = iota
	Values
	Contains
)

const (
	SUCCESS = iota
	FAIL
	SKIP
	UNKNOWN
)

const (
	OutcomePass    = "pass"
	OutcomeFail    = "fail"
	OutcomeSkip    = "skip"
	OutcomeUnknown = "unknown"
)

var humanOutcomes map[int]string = map[int]string{
	UNKNOWN: OutcomeUnknown,
	SUCCESS: OutcomePass,
	FAIL:    OutcomeFail,
	SKIP:    OutcomeSkip,
}

func HumanOutcomes() map[int]string {
	return humanOutcomes
}

type ValidateError string

func (g ValidateError) Error() string { return string(g) }
func toValidateError(err error) *ValidateError {
	if err == nil {
		return nil
	}
	ve := ValidateError(err.Error())
	return &ve
}

type TestResult struct {
	Successful bool `json:"successful" yaml:"successful"`
	Skipped    bool `json:"skipped" yaml:"skipped"`
	// Resource data
	ResourceId   string `json:"resource-id" yaml:"resource-id"`
	ResourceType string `json:"resource-type" yaml:"resource-type"`
	Property     string `json:"property" yaml:"property"`

	// User added info
	Title string `json:"title" yaml:"title"`
	Meta  meta   `json:"meta" yaml:"meta"`

	// Result
	Result        int                    `json:"result" yaml:"result"`
	Err           *ValidateError         `json:"err" yaml:"err"`
	MatcherResult matchers.MatcherResult `json:"matcher-result" yaml:"matcher-result"`
	StartTime     time.Time              `json:"start-time" yaml:"start-time"`
	EndTime       time.Time              `json:"end-time" yaml:"end-time"`
	Duration      time.Duration          `json:"duration" yaml:"duration"`
}

// ToOutcome converts the enum to a human-friendly string.
func (tr TestResult) ToOutcome() string {
	switch tr.Result {
	case SUCCESS:
		return OutcomePass
	case FAIL:
		return OutcomeFail
	case SKIP:
		return OutcomeSkip
	default:
		return OutcomeUnknown
	}
}

func (t TestResult) SortKey() string {
	return fmt.Sprintf("%s:%s", t.ResourceType, t.ResourceId)
}

func skipResult(typeS string, id string, title string, meta meta, property string, startTime time.Time) TestResult {
	endTime := time.Now()
	return TestResult{
		Result:       SKIP,
		Skipped:      true,
		ResourceType: typeS,
		ResourceId:   id,
		Title:        title,
		Meta:         meta,
		Property:     property,
		StartTime:    startTime,
		EndTime:      endTime,
		Duration:     endTime.Sub(startTime),
	}
}

// TypedResourceRead is ResourceRead plus TypeName(). It is *not* required
// by ValidateValue/ValidateGomegaValue's exported signatures below --
// deliberately, see the comment on typeNameFor -- but every builtin
// resource type satisfies it, and any embedder's custom ResourceRead
// implementation should too, for the same reason: it's what lets those
// functions print an accurate ResourceType without reflection.
type TypedResourceRead interface {
	ResourceRead
	TypeName() string
}

// typeNameFor derives the resource type string ValidateGomegaValue prints
// in a TestResult. Prefers TypeName() (FEAT-007 §4.5: correct for any
// type, in-package or not -- resource/skip.go already did this).
//
// Falls back to the old reflect.TypeOf(res) -> split(".")[1] derivation
// for a ResourceRead that doesn't also implement TypeName() -- which,
// for every type in this codebase, is never (all 17 implement it; see
// TestTypeNameMatchesReflectType). This fallback exists purely so
// ValidateValue/ValidateGomegaValue's exported parameter type can stay
// ResourceRead, not a new interface: those two functions are the one
// place this refactor could have quietly broken a library embedder's
// custom ResourceRead-implementing type (§4.2's "preserve the public
// API" applies here every bit as much as it does to the resource maps),
// so the widened case degrades to exactly the old behaviour instead of
// failing to compile.
func typeNameFor(res ResourceRead) string {
	if tn, ok := res.(interface{ TypeName() string }); ok {
		return tn.TypeName()
	}
	typ := reflect.TypeOf(res)
	return strings.Split(typ.String(), ".")[1]
}

func ValidateValue(res ResourceRead, property string, expectedValue any, actual any, skip bool) TestResult {
	if f, ok := actual.(func() (io.Reader, error)); ok {
		if _, ok := expectedValue.([]any); !ok {
			actual = func() (string, error) {
				v, err := f()
				if err != nil {
					return "", err
				}
				i, err := matchers.ReaderToString{}.Transform(v)
				if err != nil {
					return "", err
				}
				return i.(string), nil
			}
		}
	}
	return ValidateGomegaValue(res, property, expectedValue, actual, skip)
}

func ValidateGomegaValue(res ResourceRead, property string, expectedValue any, actual any, skip bool) TestResult {
	id := res.ID()
	title := res.GetTitle()
	meta := res.GetMeta()
	typeS := typeNameFor(res)
	startTime := time.Now()
	if skip {
		return skipResult(
			typeS,
			id,
			title,
			meta,
			property,
			startTime,
		)
	}

	var foundValue any
	var gomegaMatcher matchers.SyverMatcher
	var err error
	switch f := actual.(type) {
	case func() (bool, error):
		foundValue, err = f()
	case func() (string, error):
		foundValue, err = f()
	case func() (int, error):
		foundValue, err = f()
	case func() ([]string, error):
		foundValue, err = f()
	case func() ([]int, error):
		foundValue, err = f()
	case func() (any, error):
		foundValue, err = f()
	case func() (io.Reader, error):
		var r io.Reader
		r, err = f()
		if err == nil {
			var i interface{}
			i, err = matchers.ReaderToString{}.Transform(r)
			if err == nil {
				foundValue = i.(string)
			}
		}
		gomegaMatcher = matchers.HavePatterns(expectedValue)
	default:
		err = fmt.Errorf("unknown method signature: %t", f)
	}

	var success bool
	if gomegaMatcher == nil && err == nil {
		gomegaMatcher, err = matcherToGomegaMatcher(expectedValue)
	}
	if err != nil {
		endTime := time.Now()
		return TestResult{
			Result:       FAIL,
			ResourceType: typeS,
			ResourceId:   id,
			Title:        title,
			Meta:         meta,
			Property:     property,
			Err:          toValidateError(err),
			StartTime:    startTime,
			EndTime:      endTime,
			Duration:     endTime.Sub(startTime),
		}
	}

	success, err = gomegaMatcher.Match(foundValue)

	var matcherResult matchers.MatcherResult
	result := SUCCESS
	if success {
		matcherResult = matchers.MatcherResult{
			Actual:   foundValue,
			Message:  "matches expectation",
			Expected: expectedValue,
		}
	} else {
		matcherResult = gomegaMatcher.FailureResult(foundValue)
		result = FAIL
	}

	endTime := time.Now()
	return TestResult{
		Result:        result,
		ResourceType:  typeS,
		ResourceId:    id,
		Title:         title,
		Meta:          meta,
		Property:      property,
		MatcherResult: matcherResult,
		Err:           toValidateError(err),
		StartTime:     startTime,
		EndTime:       endTime,
		Duration:      endTime.Sub(startTime),
	}
}
