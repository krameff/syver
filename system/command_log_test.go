package system

import (
	"bytes"
	"context"
	"log"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// The DEBUG lines for a command's output carry one line per line of output,
// labelled with a run number rather than the command text, which can hold
// credentials. They used to read "[Command][%!s(<nil>)][stdout]" and end with
// an empty line for the output's trailing newline.
func TestCommandOutputDebugLogOmitsTheCommandText(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	c := NewDefCommand(context.Background(), "echo hi secret-arg", &System{}, util.Config{Timeout: 10 * time.Second})
	if err := c.(*DefCommand).setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := logOutput.String()
	if strings.Contains(got, "%!") {
		t.Errorf("log line has a formatting error:\n%s", got)
	}
	if !regexp.MustCompile(`\[DEBUG\]\[Command\]\[#\d+\]\[stdout\] hi secret-arg`).MatchString(got) {
		t.Errorf("expected the stdout line labelled with a run number, got:\n%s", got)
	}
	if strings.Contains(got, "echo") || strings.Count(got, "secret-arg") != 1 {
		t.Errorf("the command text must not appear in the label, only the output may:\n%s", got)
	}
	if n := strings.Count(got, "[stdout]"); n != 1 {
		t.Errorf("one line of output should log one line, got %d:\n%s", n, got)
	}
}

// A multi-line `exec: |` script must not leak into the label either: each
// log record is one prefixed line of output, never a fragment of the script.
func TestMultiLineCommandDebugLogIsOneRecordPerOutputLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cmd.exe /c does not run a newline-separated script")
	}
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })

	script := "echo first\necho second # secret-in-script\n"
	c := NewDefCommand(context.Background(), script, &System{}, util.Config{Timeout: 10 * time.Second})
	if err := c.(*DefCommand).setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := strings.TrimSuffix(logOutput.String(), "\n")
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("two lines of output should log two records, got %d:\n%s", len(lines), got)
	}
	label := regexp.MustCompile(`^\[DEBUG\]\[Command\]\[#\d+\]\[stdout\] (first|second)$`)
	for _, l := range lines {
		if !label.MatchString(l) {
			t.Errorf("record is not one labelled line of output: %q", l)
		}
	}
	if strings.Contains(got, "secret-in-script") || strings.Contains(got, "echo") {
		t.Errorf("script text leaked into the log:\n%s", got)
	}
}

func TestForLogNormalisesLineEndings(t *testing.T) {
	for in, want := range map[string]string{
		"hi\n":         "hi",
		"hi\r\n":       "hi",
		"a\r\nb\r\n":   "a\nb",
		"a\nb":         "a\nb",
		"":             "",
		"keep\n\n":     "keep\n",
		"bare\rcr\r\n": "bare\rcr",
	} {
		if got := string(forLog([]byte(in))); got != want {
			t.Errorf("forLog(%q) = %q, want %q", in, got, want)
		}
	}
}
