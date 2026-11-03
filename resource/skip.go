package resource

import (
	"strings"
	"time"
)

// oneLineSkipReason collapses every run of whitespace, newlines included, to a
// single space. A skip reason is printed inside one line in several formats:
// documentation and TAP print it on the result's own line, so a raw newline
// from a `|` or `>` block would end that line early and let the rest of the
// reason read as further TAP output (`ok 99`, `not ok`, `Bail out!`). Every
// reason passes through here once, where it is stored on the result, so all
// formats carry the same text.
func oneLineSkipReason(reason string) string {
	return strings.Join(strings.Fields(reason), " ")
}

// SkipResourceResults emits skip results for every validated property on a resource.
// reason is set as both SkipReason and Err; Err is kept so consumers that
// read it on depends-on skips continue to work.
func SkipResourceResults(res Resource, reason string) []TestResult {
	reason = oneLineSkipReason(reason)
	startTime := time.Now()
	validateErr := ValidateError(reason)

	if rr, ok := res.(ResourceRead); ok {
		return []TestResult{
			{
				Result:       SKIP,
				Skipped:      true,
				SkipReason:   reason,
				ResourceType: res.TypeName(),
				ResourceId:   rr.ID(),
				Title:        rr.GetTitle(),
				Meta:         rr.GetMeta(),
				Property:     "depends-on",
				Err:          &validateErr,
				StartTime:    startTime,
				EndTime:      startTime,
			},
		}
	}

	return []TestResult{
		{
			Result:       SKIP,
			Skipped:      true,
			SkipReason:   reason,
			ResourceType: res.TypeName(),
			ResourceId:   res.TypeKey(),
			Property:     "depends-on",
			Err:          &validateErr,
			StartTime:    startTime,
			EndTime:      startTime,
		},
	}
}
