package syver

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

func TestValidateDiscoveryFormat(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "discovery.yaml")
	content := []byte(`discovery:
  file:
    /etc/hosts:
      register: hosts_exists
      exists: true
`)
	if err := os.WriteFile(spec, content, 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	cfg, err := util.NewConfig(
		util.WithSpecFile(spec),
		util.WithOutputFormat("discovery"),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	code, err := Validate(cfg)
	if err != nil {
		t.Fatalf("validate discovery: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestValidateDependsOnSkipsDependent(t *testing.T) {
	sys := system.New("")
	base := &resource.File{
		Exists: true,
		Path:   "/nonexistent/goss-dependency-base",
	}
	dependent := &resource.File{
		Exists: true,
		DiscoveryMeta: resource.DiscoveryMeta{
			DependsOn: []string{"base"},
		},
	}
	base.SetID("base")
	dependent.SetID("dependent")
	resources := []resource.Resource{base, dependent}

	out, err := validateWithDependencies(sys, resources, 1)
	if err != nil {
		t.Fatalf("validate with dependencies: %v", err)
	}

	var skipped bool
	var failed bool
	for group := range out {
		for _, result := range group {
			if result.Skipped && result.Property == "depends-on" {
				skipped = true
			}
			if result.ResourceType == "File" && result.Property == "exists" && result.Result == resource.FAIL {
				failed = true
			}
		}
	}

	if !failed {
		t.Fatal("expected base resource to fail")
	}
	if !skipped {
		t.Fatal("expected dependent resource to be skipped")
	}
}

func TestTemplateDiscoveredVars(t *testing.T) {
	filter, err := NewTemplateFilter([]string{}, `{"Discovered":{"enabled":true}}`, nil)
	if err != nil {
		t.Fatalf("template filter: %v", err)
	}

	out, err := filter([]byte(`enabled: {{ .Discovered.enabled }}`))
	if err != nil {
		t.Fatalf("render template: %v", err)
	}

	if string(out) != "enabled: true" {
		t.Fatalf("unexpected rendered output: %q", string(out))
	}
}

func TestValidateWithoutDiscover(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.ToSlash(filepath.Join(dir, "sentinel"))
	if err := os.WriteFile(filepath.FromSlash(sentinel), []byte("present"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	spec := filepath.Join(dir, "goss.yml")
	if err := os.WriteFile(spec, []byte(fmt.Sprintf(`file:
  sentinel:
    path: %s
    exists: true
`, sentinel)), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	cfg, err := util.NewConfig(
		util.WithSpecFile(spec),
		util.WithOutputFormat("documentation"),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	if cfg.DiscoverSpec != "" {
		t.Fatal("expected empty DiscoverSpec for plain validate")
	}

	code, err := Validate(cfg)
	if err != nil {
		t.Fatalf("validate without discover: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestValidateWithoutDiscoverErrorsOnMissingTemplateVar(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "goss.yml")
	if err := os.WriteFile(spec, []byte(`file:
  {{ .Vars.missing_key }}/hosts:
    exists: true
`), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	cfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithNoColor())
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	_, err = Validate(cfg)
	if err == nil {
		t.Fatal("expected template error for missing .Vars key without discover")
	}
}

func discoveryExamplesDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("integration-tests", "syver", "examples", "discovery")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("discovery examples missing: %v", err)
	}
	return dir
}

func TestValidateWithDiscoverFlag(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("discovery example fixture asserts on /etc/hosts, linux only")
	}
	dir := discoveryExamplesDir(t)
	cfg, err := util.NewConfig(
		util.WithSpecFile(filepath.Join(dir, "goss.yml")),
		util.WithDiscoverFile(filepath.Join(dir, "discovery.yaml")),
		util.WithOutputFormat("documentation"),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	code, err := Validate(cfg)
	if err != nil {
		t.Fatalf("validate with discover: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestValidateInlineDiscovery(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("discovery example fixture asserts on /etc/hosts, linux only")
	}
	dir := discoveryExamplesDir(t)
	cfg, err := util.NewConfig(
		util.WithSpecFile(filepath.Join(dir, "goss-inline.yml")),
		util.WithOutputFormat("documentation"),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	code, err := Validate(cfg)
	if err != nil {
		t.Fatalf("validate inline discovery: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestDiscoverFlagOverridesInline(t *testing.T) {
	dir := t.TempDir()
	inline := filepath.Join(dir, "inline.yml")
	discover := filepath.Join(dir, "discover.yaml")
	main := filepath.Join(dir, "main.yml")

	// Use a file we create ourselves rather than a Unix-specific path like
	// /etc/hosts so this test is portable to Windows/macOS runners.
	sentinel := filepath.ToSlash(filepath.Join(dir, "sentinel"))
	if err := os.WriteFile(filepath.FromSlash(sentinel), []byte("present"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	inlineContent := []byte(`discovery:
  file:
    /etc/hosts:
      register: from_inline
      exists: true
file:
  /tmp/inline-should-not-run:
    exists: false
`)
	discoverContent := []byte(fmt.Sprintf(`discovery:
  file:
    sentinel:
      path: %s
      register: from_flag
      exists: true
`, sentinel))
	mainContent := []byte(fmt.Sprintf(`{{ if .Discovered.from_flag }}
file:
  sentinel:
    path: %s
    exists: true
{{ end }}
`, sentinel))

	for path, content := range map[string][]byte{
		inline: inlineContent, discover: discoverContent, main: mainContent,
	} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	cfg, err := util.NewConfig(
		util.WithSpecFile(main),
		util.WithDiscoverFile(discover),
		util.WithOutputFormat("documentation"),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	// Peek: inline file has discovery but we use --discover pointing elsewhere
	peekCfg, err := util.NewConfig(util.WithSpecFile(inline), util.WithNoColor())
	if err != nil {
		t.Fatalf("peek config: %v", err)
	}
	peek, err := getSyverConfigPeek(peekCfg.VarsFiles, peekCfg.VarsInline, peekCfg.Spec)
	if err != nil {
		t.Fatalf("peek load: %v", err)
	}
	if peek.Discovery.IsEmpty() {
		t.Fatal("expected inline discovery in peek file")
	}

	code, err := Validate(cfg)
	if err != nil {
		t.Fatalf("validate override: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0 when --discover supplies from_flag, got %d", code)
	}
}

// TestValidateStdinMatchesFile covers the first stdin symptom: this is the actual
// reported bug. `validate -g -` (stdin) must produce the same result count
// as `validate -g <file>` on the equivalent file. Before the fix, stdin
// always failed with "found 0 tests, source: STDIN" because
// loadSyverConfigWithDiscover decodes the spec twice (peek, then the real
// load) and a naive double-read of os.Stdin returns the real bytes once and
// 0 bytes on the second call.
func TestValidateStdinMatchesFile(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.ToSlash(filepath.Join(dir, "sentinel"))
	if err := os.WriteFile(filepath.FromSlash(sentinel), []byte("present"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	content := []byte(fmt.Sprintf(`file:
  sentinel:
    path: %s
    exists: true
`, sentinel))

	spec := filepath.Join(dir, "goss.yml")
	if err := os.WriteFile(spec, content, 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	fileCfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithNoColor())
	if err != nil {
		t.Fatalf("new config (file): %v", err)
	}
	fileResults, err := ValidateResults(fileCfg)
	if err != nil {
		t.Fatalf("validate (file): %v", err)
	}
	fileCount := 0
	for group := range fileResults {
		fileCount += len(group)
	}
	if fileCount == 0 {
		t.Fatal("expected at least one result from the file-based baseline")
	}

	resetStdinOnce(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		_, _ = w.Write(content)
		_ = w.Close()
	}()

	stdinCfg, err := util.NewConfig(util.WithSpecFile("-"), util.WithNoColor())
	if err != nil {
		t.Fatalf("new config (stdin): %v", err)
	}
	stdinResults, err := ValidateResults(stdinCfg)
	if err != nil {
		t.Fatalf("validate (stdin): %v", err)
	}
	stdinCount := 0
	for group := range stdinResults {
		stdinCount += len(group)
	}

	if stdinCount != fileCount {
		t.Fatalf("stdin validate produced %d results, expected %d to match the file-based run (this is the original stdin double-read symptom -- 'found 0 tests, source: STDIN')", stdinCount, fileCount)
	}
}

// TestValidateStdinWithDiscoverySection covers Acceptance Criterion 2: the
// same stdin fix must also hold for a spec with a non-empty discovery:
// section, which exercises the runDiscoveryPhase branch in
// loadSyverConfigWithDiscover -- a different code path that also peeks
// before the final load.
func TestValidateStdinWithDiscoverySection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("discovery fixture asserts on /etc/hosts, linux only")
	}

	// Deliberately plain YAML with no {{ }} templating: for a "-" spec,
	// outStoreFormat is detected from the RAW bytes (getStoreFormatFromData,
	// before any template rendering happens), so templated content like the
	// goss-inline.yml example fixture uses would fail format detection here
	// with "unable to determine format from content" -- a real, separate,
	// pre-existing limitation of stdin format-detection, not part of
	// the double-decode fix. Keeping this fixture template-free
	// isolates the test to the thing that fix actually addresses.
	content := []byte(`discovery:
  file:
    /etc/hosts:
      register: hosts_exists
      exists: true
file:
  /etc/hosts:
    exists: true
`)

	resetStdinOnce(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		_, _ = w.Write(content)
		_ = w.Close()
	}()

	cfg, err := util.NewConfig(
		util.WithSpecFile("-"),
		util.WithOutputFormat("documentation"),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	code, err := Validate(cfg)
	if err != nil {
		t.Fatalf("validate inline discovery via stdin: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

// TestValidateStdinEmptySpecStillErrors is the regression check for
// Acceptance Criterion 3: a genuinely empty spec piped via stdin must still
// correctly error with "found 0 tests, source: STDIN" -- the fix must not
// suppress this real error case.
func TestValidateStdinEmptySpecStillErrors(t *testing.T) {
	resetStdinOnce(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte("file: {}\n"))
		_ = w.Close()
	}()

	cfg, err := util.NewConfig(util.WithSpecFile("-"), util.WithNoColor())
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	_, err = Validate(cfg)
	if err == nil {
		t.Fatal("expected 'found 0 tests' error for a genuinely empty stdin spec")
	}
	if !strings.Contains(err.Error(), "found 0 tests, source: STDIN") {
		t.Fatalf("expected 'found 0 tests, source: STDIN' error, got: %v", err)
	}
}

// TestValidateCollisionWarnLogsOnce covers the second symptom of that same
// double-decode root cause: a real
// gossfile:/syverfile: alias collision must log its WARN exactly once, not
// once per decode. loadSyverConfigWithDiscover decodes every spec at least
// twice (peek, then the real load); quietDecode (set from loadSyverConfig's
// own peek parameter) suppresses ReadJSONData's alias-collision WARN during
// the peek pass, since the peek's SyverConfig result is never observed by
// the caller -- only the real load's WARN should reach the user. Unlike
// Test_syverfileAlias_CollisionLogsWarnAndGossfileWins in store_test.go
// (which calls ReadJSONData directly, once, and so never exercises the
// double-decode path at all), this test goes through the real
// loadSyverConfigWithDiscover call path where the duplication would occur
// without quietDecode.
func TestValidateCollisionWarnLogsOnce(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.ToSlash(filepath.Join(dir, "sentinel"))
	if err := os.WriteFile(filepath.FromSlash(sentinel), []byte("present"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	imported := filepath.Join(dir, "imported.yaml")
	if err := os.WriteFile(imported, []byte(fmt.Sprintf(`file:
  sentinel:
    path: %s
    exists: true
`, sentinel)), 0o644); err != nil {
		t.Fatalf("write imported spec: %v", err)
	}

	spec := filepath.Join(dir, "same.yaml")
	content := fmt.Sprintf("gossfile:\n  dup:\n    file: %s\nsyverfile:\n  dup:\n    file: %s\n", filepath.Base(imported), filepath.Base(imported))
	if err := os.WriteFile(spec, []byte(content), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	cfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithNoColor())
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	// Call loadSyverConfigWithDiscover directly, not Validate(): Validate()
	// calls setLogLevel() first, which unconditionally re-points the log
	// package's output at os.Stderr (via logutils), clobbering any
	// log.SetOutput a test installs beforehand. loadSyverConfigWithDiscover
	// is the actual site of the peek-then-load double decode this test
	// targets, so calling it directly is both necessary to observe the log
	// output and a faithful exercise of the real bug path.
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	syverConfig, err := loadSyverConfigWithDiscover(cfg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(syverConfig.Resources()) == 0 {
		t.Fatal("expected at least one resource from the imported gossfile")
	}
	// gossfile: still wins the collision (unchanged) -- see
	// Test_syverfileAlias_CollisionLogsWarnAndGossfileWins in store_test.go
	// for the direct ReadJSONData-level assertion of this. Here we only need
	// to confirm the *count* of WARN lines through the real double-decode path.

	warnCount := strings.Count(logOutput.String(), "[WARN]")
	if warnCount != 1 {
		t.Fatalf("expected exactly 1 [WARN] line for the collision, got %d:\n%s", warnCount, logOutput.String())
	}
}

func TestValidateDiscoverWithDependsOn(t *testing.T) {
	// Self-contained variant of integration-tests/syver/examples/discovery/
	// goss-with-deps.yml using a sentinel file instead of /etc/hosts, so the
	// depends-on/skip semantics are also exercised on Windows/macOS runners.
	dir := t.TempDir()
	sentinel := filepath.ToSlash(filepath.Join(dir, "sentinel"))
	if err := os.WriteFile(filepath.FromSlash(sentinel), []byte("present"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	missing := filepath.ToSlash(filepath.Join(dir, "does-not-exist"))

	discover := filepath.Join(dir, "discovery.yaml")
	main := filepath.Join(dir, "goss-with-deps.yml")

	discoverContent := []byte(fmt.Sprintf(`discovery:
  file:
    sentinel:
      path: %s
      register: hosts_exists
      exists: true
`, sentinel))
	mainContent := []byte(fmt.Sprintf(`{{ if .Discovered.hosts_exists }}
file:
  prereq:
    path: %s
    exists: true

command:
  dependent:
    depends-on:
      - prereq
    exec: true
    exit-status: 0
{{ end }}
`, missing))

	for path, content := range map[string][]byte{discover: discoverContent, main: mainContent} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	cfg, err := util.NewConfig(
		util.WithSpecFile(main),
		util.WithDiscoverFile(discover),
		util.WithOutputFormat("documentation"),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	code, err := Validate(cfg)
	if err != nil {
		t.Fatalf("validate discover with depends-on: %v", err)
	}
	if code == 0 {
		t.Fatal("expected non-zero exit when prerequisite fails")
	}
}
