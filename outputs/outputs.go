package outputs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/fatih/color"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
	"github.com/pmezard/go-difflib/difflib"
)

type formatOption struct {
	name string
}

type Outputer interface {
	Output(io.Writer, <-chan []resource.TestResult, util.OutputConfig) int
	ValidOptions() []*formatOption
}

var (
	// outputers is fixed at compile time and only ever read, so it needs no lock.
	outputers = map[string]Outputer{
		"documentation": &Documentation{},
		"json":          &Json{},
		"junit":         &JUnit{},
		"nagios":        &Nagios{},
		"prometheus":    &Prometheus{},
		"rspecish":      &Rspecish{},
		"structured":    &Structured{},
		"tap":           &Tap{},
		"silent":        &Silent{},
	}
	foPerfData   = "perfdata"
	foVerbose    = "verbose"
	foPretty     = "pretty"
	foExcludeRaw = "exclude_raw"
	foSort       = "sort"

	// excludeRawFormats are the formats that honour exclude_raw. The option is
	// applied inside humanizeResult, which these formats share for their
	// failure text, so it is listed once here rather than in each format's own
	// ValidOptions. Formats that never call humanizeResult (prometheus, silent)
	// are absent because the option has no effect on them.
	excludeRawFormats = map[string]bool{
		"documentation": true,
		"json":          true,
		"junit":         true,
		"nagios":        true,
		"rspecish":      true,
		"structured":    true,
		"tap":           true,
	}
)

var green = color.New(color.FgGreen).SprintfFunc()
var red = color.New(color.FgRed).SprintfFunc()
var yellow = color.New(color.FgYellow).SprintfFunc()
var multiple_space = regexp.MustCompile(`\s+`)

func humanizeResult(r resource.TestResult, compact bool, includeRaw bool) string {
	sep := "\n"
	if compact {
		sep = " "
	}

	switch r.Result {
	case resource.SUCCESS:
		return green("%s: %s: %s: %s: %s", r.ResourceType, r.ResourceId, r.Property, r.MatcherResult.Message, prettyPrint(r.MatcherResult.Expected, false))
	case resource.FAIL:
		matcherResult := prettyPrintTestResult(r, compact, includeRaw)
		return red("%s: %s: %s:%s%s", r.ResourceType, r.ResourceId, r.Property, sep, matcherResult)
	case resource.SKIP:
		return yellow("%s: %s: %s: skipped", r.ResourceType, r.ResourceId, r.Property)
	default:
		panic(fmt.Sprintf("Unexpected Result Code: %v\n", r.Result))
	}
}

func prettyPrintTestResult(t resource.TestResult, compact bool, includeRaw bool) string {
	sep := "\n"
	if compact {
		sep = " "
	}
	m := t.MatcherResult
	var ss []string
	//var s string
	if t.Err != nil {
		e := fmt.Sprint(t.Err)
		if compact {
			e = multiple_space.ReplaceAllString(e, " ")
		} else {
			e = indentLines(e)
		}
		ss = append(ss, "Error")
		ss = append(ss, e)
	} else {
		ss = append(ss, "Expected")
		ss = append(ss, prettyPrint(m.Actual, !compact))
		ss = append(ss, m.Message)
		ss = append(ss, prettyPrint(m.Expected, !compact))
		ss = maybeAddDiff(ss, m.Expected, m.Actual, compact)
	}

	if reflect.ValueOf(m.MissingElements).IsValid() && !reflect.ValueOf(m.MissingElements).IsNil() {
		ss = append(ss, "the missing elements were")
		ss = append(ss, prettyPrint(m.MissingElements, !compact))
	}
	if reflect.ValueOf(m.ExtraElements).IsValid() && !reflect.ValueOf(m.ExtraElements).IsNil() {
		ss = append(ss, "the extra elements were")
		ss = append(ss, prettyPrint(m.ExtraElements, !compact))
	}
	if len(m.TransformerChain) != 0 {
		ss = append(ss, "the transform chain was")
		ss = append(ss, prettyPrint(m.TransformerChain, !compact))
		if includeRaw {
			ss = append(ss, "the raw value was")
			ss = append(ss, prettyPrint(m.UntransformedValue, !compact))
		}
	}
	return strings.Join(ss, sep)
}

func maybeAddDiff(ss []string, expected, actual any, compact bool) []string {
	if compact {
		return ss
	}
	want, ok := expected.(string)
	if !ok {
		return ss
	}
	got, ok := actual.(string)
	if !ok {
		return ss
	}
	if want == got {
		return ss
	}
	ss = append(ss, "diff")
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(want),
		B:        difflib.SplitLines(got),
		FromFile: "test",
		FromDate: "",
		ToFile:   "actual",
		ToDate:   "",
		Context:  1,
	})
	ss = append(ss, indentLines(diff))
	return ss
}

func prettyPrint(i interface{}, indent bool) string {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	var b []byte
	err := encoder.Encode(i)
	if err == nil {
		b = buffer.Bytes()
	} else {
		// FIXME: Is this the right thing to do?
		b = []byte(err.Error())
	}
	b = bytes.TrimRightFunc(b, unicode.IsSpace)
	if indent {
		return indentLines(string(b))
	} else {
		return string(b)
	}
}

// indents a block of text with an indent string
func indentLines(text string) string {
	indent := "    "
	result := ""
	for _, j := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		result += indent + j + "\n"
	}
	return result[:len(result)-1]
}

// Outputers returns a sorted list of the names of the registered outputers.
func Outputers() []string {
	var list []string
	for name := range outputers {
		list = append(list, name)
	}
	list = append(list, "discovery")
	sort.Strings(list)
	return list
}

// FormatOptions returns a sorted list of every option at least one format
// accepts.
func FormatOptions() []string {
	found := map[string]bool{}
	for _, f := range Outputers() {
		for _, name := range ValidOptionsFor(f) {
			found[name] = true
		}
	}
	list := make([]string, 0, len(found))
	for name := range found {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

// ValidOptionsFor returns the sorted options the named format honours, or nil
// if format is not a format name.
func ValidOptionsFor(format string) []string {
	var opts []*formatOption
	if format == "discovery" {
		opts = Discovery{}.ValidOptions()
	} else {
		o, ok := outputers[format]
		if !ok {
			return nil
		}
		opts = o.ValidOptions()
	}
	names := make([]string, 0, len(opts)+1)
	for _, opt := range opts {
		names = append(names, opt.name)
	}
	if excludeRawFormats[format] {
		names = append(names, foExcludeRaw)
	}
	sort.Strings(names)
	return names
}

// NormalizeFormatOptions trims surrounding whitespace from each option and
// drops any left empty, so "pretty, sort" from an environment variable means
// the same as -o pretty -o sort. The formats compare option names exactly
// apart from case, so an untrimmed " sort" would otherwise be ignored.
func NormalizeFormatOptions(opts []string) []string {
	out := make([]string, 0, len(opts))
	for _, opt := range opts {
		if opt = strings.TrimSpace(opt); opt != "" {
			out = append(out, opt)
		}
	}
	return out
}

// CheckFormatOptions sorts opts against format. Names are matched without
// regard to case, as the formats themselves match them, and after
// NormalizeFormatOptions.
//
// unknown holds options no format knows; ignored holds options some other
// format knows but this one does not honour. Neither is an error: both have
// always been accepted and had no effect, so the caller warns rather than
// refusing to run. ignored is empty for an unknown format, which GetOutputer
// reports.
func CheckFormatOptions(format string, opts []string) (unknown, ignored []string) {
	all := FormatOptions()
	own := ValidOptionsFor(format)
	for _, opt := range NormalizeFormatOptions(opts) {
		switch {
		case !util.IsValueInList(opt, all):
			unknown = append(unknown, opt)
		case own != nil && !util.IsValueInList(opt, own):
			ignored = append(ignored, opt)
		}
	}
	return unknown, ignored
}

// IsValidFormat determines if f is a valid format name based on Outputers()
func IsValidFormat(f string) bool {
	if f == "discovery" {
		return true
	}
	for _, o := range Outputers() {
		if o == f {
			return true
		}
	}

	return false
}

func GetOutputer(name string) (Outputer, error) {
	if _, ok := outputers[name]; !ok {
		return nil, fmt.Errorf("bad output format: %v", name)
	}
	return outputers[name], nil
}

func header(t resource.TestResult) string {
	var out string
	if t.Title != "" {
		out += fmt.Sprintf("Title: %s\n", t.Title)
	}
	if t.Meta != nil {
		var keys []string
		for k := range t.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		out += "Meta:\n"
		for _, k := range keys {
			out += fmt.Sprintf("    %v: %v\n", k, t.Meta[k])
		}
	}
	return out
}

func summary(startTime, endTime time.Time, count, failed, skipped int) string {
	var s string
	s += fmt.Sprintf("Total Duration: %.3fs\n", endTime.Sub(startTime).Seconds())
	f := green
	if failed > 0 {
		f = red
	}
	s += f("Count: %d, Failed: %d, Skipped: %d\n", count, failed, skipped)
	return s
}

func failedOrSkippedSummary(failedOrSkipped [][]resource.TestResult, includeRaw bool) string {
	var s string
	if len(failedOrSkipped) > 0 {
		s += "Failures/Skipped:\n\n"
		sort.Slice(failedOrSkipped, func(i, j int) bool {
			return failedOrSkipped[i][0].SortKey() < failedOrSkipped[j][0].SortKey()
		})
		for _, failedGroup := range failedOrSkipped {
			first := failedGroup[0]
			header := header(first)
			if header != "" {
				s += fmt.Sprint(header)
			}
			for _, testResult := range failedGroup {
				s += fmt.Sprintln(humanizeResult(testResult, false, includeRaw))
			}
			s += "\n"
		}
	}
	return s
}

func getResults(tr <-chan []resource.TestResult, doSort bool) <-chan []resource.TestResult {
	if !doSort {
		return tr
	}
	str := make([][]resource.TestResult, 0)
	for i := range tr {
		str = append(str, i)
	}

	sort.Slice(str, func(i, j int) bool {
		return str[i][0].SortKey() < str[j][0].SortKey()
	})

	c := make(chan []resource.TestResult)
	go func(c chan []resource.TestResult) {
		defer close(c)

		for _, i := range str {
			c <- i
		}
	}(c)

	return c
}
