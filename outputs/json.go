package outputs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
)

type Json struct{}

func (r Json) ValidOptions() []*formatOption {
	return []*formatOption{
		{name: foPretty},
		{name: foSort},
	}
}

func (r Json) Output(w io.Writer, results <-chan []resource.TestResult,
	outConfig util.OutputConfig) (exitCode int) {

	pretty := util.IsValueInList(foPretty, outConfig.FormatOptions)
	includeRaw := !util.IsValueInList(foExcludeRaw, outConfig.FormatOptions)

	sort := util.IsValueInList(foSort, outConfig.FormatOptions)
	results = getResults(results, sort)

	var startTime time.Time
	var endTime time.Time
	forceNoColor()
	testCount := 0
	failed := 0
	skipped := 0
	renderFailed := false
	var resultsOut []map[string]any
	for resultGroup := range results {
		for _, testResult := range resultGroup {
			if startTime.IsZero() || testResult.StartTime.Before(startTime) {
				startTime = testResult.StartTime
			}
			if endTime.IsZero() || testResult.EndTime.After(endTime) {
				endTime = testResult.EndTime
			}
			if testResult.Result == resource.FAIL {
				failed++
				logTrace("TRACE", "FAIL", testResult, true)
			} else {
				logTrace("TRACE", "SUCCESS", testResult, true)
			}
			if testResult.Skipped {
				skipped++
			}
			m, err := struct2map(testResult)
			if err != nil {
				log.Printf("[ERROR] json output: cannot render result %s %s: %v", testResult.ResourceType, testResult.ResourceId, err)
				renderFailed = true
				continue
			}
			m["successful"] = testResult.Result != resource.FAIL
			m["summary-line"] = humanizeResult(testResult, false, includeRaw)
			m["summary-line-compact"] = humanizeResult(testResult, true, includeRaw)
			m["duration"] = testResult.Duration.Nanoseconds()
			resultsOut = append(resultsOut, m)
			testCount++
		}
	}

	summary := make(map[string]any)
	duration := endTime.Sub(startTime)
	summary["test-count"] = testCount
	summary["failed-count"] = failed
	summary["skipped-count"] = skipped
	summary["total-duration"] = duration
	summary["summary-line"] = fmt.Sprintf("Count: %d, Failed: %d, Skipped: %d, Duration: %.3fs", testCount, failed, skipped, duration.Seconds())

	out := make(map[string]any)
	out["results"] = resultsOut
	out["summary"] = summary

	var j []byte
	var err error
	if pretty {
		j, err = json.MarshalIndent(out, "", "    ")
	} else {
		j, err = json.Marshal(out)
	}
	if err != nil {
		log.Printf("[ERROR] json output: %v", err)
		return 1
	}

	resstr := string(j)
	fmt.Fprintln(w, resstr)

	if renderFailed {
		return 1
	}
	if failed > 0 {
		log.Printf("[DEBUG] FAIL SUMMARY: %s", resstr)
		return 1
	}

	log.Printf("[DEBUG] OK SUMMARY: %s", resstr)
	return 0
}

func struct2map(tr resource.TestResult) (map[string]any, error) {
	out := make(map[string]any)
	j, err := marshalJSONSafe(json.Marshal, tr, func() any { return jsonSafeResult(tr) })
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(j, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// marshalJSONSafe marshals v, and if that fails on a value JSON cannot
// represent (NaN or an infinity, which a numeric matcher or a YAML `.nan` /
// `.inf` can put in a result), marshals what safe returns instead. The retry
// only runs on that failure, so ordinary results render exactly as before.
func marshalJSONSafe(marshal func(any) ([]byte, error), v any, safe func() any) ([]byte, error) {
	j, err := marshal(v)
	var unsupported *json.UnsupportedValueError
	if err == nil || !errors.As(err, &unsupported) {
		return j, err
	}
	return marshal(safe())
}

// jsonSafeResult returns tr with every free-form matcher value passed through
// jsonSafe.
func jsonSafeResult(tr resource.TestResult) resource.TestResult {
	mr := tr.MatcherResult
	mr.Actual = jsonSafe(mr.Actual)
	mr.Expected = jsonSafe(mr.Expected)
	mr.MissingElements = jsonSafe(mr.MissingElements)
	mr.FoundElements = jsonSafe(mr.FoundElements)
	mr.ExtraElements = jsonSafe(mr.ExtraElements)
	mr.UntransformedValue = jsonSafe(mr.UntransformedValue)
	tr.MatcherResult = mr
	return tr
}

// jsonSafe copies v, replacing NaN and infinite floats with their string form
// ("NaN", "+Inf", "-Inf") anywhere inside slices, arrays and string-keyed maps.
func jsonSafe(v any) any {
	if v == nil {
		return nil
	}
	return jsonSafeValue(reflect.ValueOf(v))
}

func jsonSafeValue(rv reflect.Value) any {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		if f := rv.Float(); math.IsNaN(f) || math.IsInf(f, 0) {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
	case reflect.Interface, reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return jsonSafeValue(rv.Elem())
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return nil
		}
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			break
		}
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = jsonSafeValue(rv.Index(i))
		}
		return out
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			break
		}
		if rv.IsNil() {
			return nil
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = jsonSafeValue(iter.Value())
		}
		return out
	}
	return rv.Interface()
}
