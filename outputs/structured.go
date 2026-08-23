package outputs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
)

// Structured is a output formatter that logs into a StructuredOutput structure
type Structured struct{}

func (r Structured) ValidOptions() []*formatOption {
	return []*formatOption{
		{name: foPretty},
		{name: foSort},
	}
}

// StructuredTestResult is an individual test result with additional human friendly summary
type StructuredTestResult struct {
	resource.TestResult
	SummaryLine        string `json:"summary-line"`
	SummaryLineCompact string `json:"summary-line-compact"`
}

// StructureTestSummary holds summary information about a test run
type StructureTestSummary struct {
	TestCount     int           `json:"test-count"`
	Failed        int           `json:"failed-count"`
	TotalDuration time.Duration `json:"total-duration"`
}

// StructuredOutput is the full output structure for the structured output format
type StructuredOutput struct {
	Results     []StructuredTestResult `json:"results"`
	Summary     StructureTestSummary   `json:"summary"`
	SummaryLine string                 `json:"summary-line"`
}

// String represents human friendly representation of the test summary
func (s *StructureTestSummary) String() string {
	return fmt.Sprintf("Count: %d, Failed: %d, Duration: %.3fs", s.TestCount, s.Failed, s.TotalDuration.Seconds())
}

// Output processes output from tests into StructuredOutput written to w as a string
func (r Structured) Output(w io.Writer, results <-chan []resource.TestResult, outConfig util.OutputConfig) (exitCode int) {
	includeRaw := !util.IsValueInList(foExcludeRaw, outConfig.FormatOptions)

	sort := util.IsValueInList(foSort, outConfig.FormatOptions)
	results = getResults(results, sort)

	result := &StructuredOutput{
		Results: []StructuredTestResult{},
		Summary: StructureTestSummary{},
	}

	var startTime time.Time
	var endTime time.Time
	for resultGroup := range results {
		for _, testResult := range resultGroup {
			if startTime.IsZero() || testResult.StartTime.Before(startTime) {
				startTime = testResult.StartTime
			}
			if endTime.IsZero() || testResult.EndTime.After(endTime) {
				endTime = testResult.EndTime
			}
			r := StructuredTestResult{
				TestResult:         testResult,
				SummaryLine:        humanizeResult(testResult, false, includeRaw),
				SummaryLineCompact: humanizeResult(testResult, true, includeRaw),
			}

			if testResult.Result == resource.FAIL {
				result.Summary.Failed++
			}

			result.Summary.TestCount++

			result.Results = append(result.Results, r)
		}
	}

	result.Summary.TotalDuration = endTime.Sub(startTime)
	result.SummaryLine = result.Summary.String()

	var j []byte

	if util.IsValueInList(foPretty, outConfig.FormatOptions) {
		j, _ = json.MarshalIndent(result, "", "  ")
	} else {
		j, _ = json.Marshal(result)
	}

	fmt.Fprintln(w, string(j))

	// Every other formatter in this package reports failure through the exit
	// code; structured was the sole outlier, ending with a bare `return 0` since
	// the file was created (upstream a17ce0f, 2020). The count was being
	// computed above and then discarded, which made `validate --format
	// structured` exit 0 on a failing spec, and -- because serve negotiates the
	// format from the request's Accept header -- let a client flip /healthz to
	// 200 on a host where every check was failing.
	if result.Summary.Failed > 0 {
		log.Printf("[DEBUG] FAIL SUMMARY: %s", result.SummaryLine)
		return 1
	}

	log.Printf("[DEBUG] OK SUMMARY: %s", result.SummaryLine)
	return 0
}
