package syver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"dario.cat/mergo"
	yamlv2 "go.yaml.in/yaml/v2"
	"go.yaml.in/yaml/v3"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
)

// isRootedIncludePath reports whether a `gossfile:` include should be used as
// given, rather than resolved relative to the file that included it.
//
// This used to be a bare strings.HasPrefix(path, "/"), which no `C:\...` path
// satisfies, so a genuinely absolute Windows include was joined onto the
// including file's directory and silently resolved somewhere else entirely.
// See FEAT-013 / FEAT-011 W2-9 (SW-12).
//
// BOTH tests are kept, deliberately. On Unix they are the same test --
// filepath.IsAbs is exactly this prefix check -- so Linux and macOS behaviour
// is provably unchanged. On Windows they differ: filepath.IsAbs("/shared/x")
// is false there, because a drive-relative path is not absolute. Dropping the
// prefix test would have changed how an existing Windows gossfile resolves,
// which is a silent behaviour change rather than a fix.
func isRootedIncludePath(p string) bool {
	return filepath.IsAbs(p) || strings.HasPrefix(p, "/")
}

const (
	UNSET = iota
	JSON
	YAML
)

var outStoreFormat = UNSET
var currentTemplateFilter TemplateFilter
var debug = false

// quietDecode suppresses non-essential decode-time logging (currently: the
// gossfile:/syverfile: alias-collision WARN in ReadJSONData below) for a
// single loadSyverConfig call. loadSyverConfigWithDiscover (discovery_load.go)
// decodes every spec at least twice per validate invocation -- once as a
// "peek" purely to inspect Discovery, once for the real, used result. The
// peek's SyverConfig result (including any
// alias-collision resolution) is never observed by the caller, so it is
// correct, not just convenient, for the peek pass to resolve collisions
// silently and let the real load's WARN be the only one that surfaces.
// Set at the top of loadSyverConfig based on its own peek parameter, and
// reset via defer -- see loadSyverConfig for where this is set. Like
// outStoreFormat and debug, this assumes loadSyverConfig is never called
// concurrently from multiple goroutines within one process, which matches
// how this CLI actually uses it today (one validate invocation, one
// sequential peek-then-load per spec, no goroutines in the loading phase).
var quietDecode = false

var (
	stdinData []byte
	stdinErr  error
	stdinOnce sync.Once
)

// readStdinOnce reads os.Stdin exactly once, no matter how many times it is
// called, and returns the same bytes (or the same error) on every call.
// os.Stdin is a non-seekable stream -- a naive io.ReadAll(os.Stdin) on every
// "-" load returns the real data on the first call and 0 bytes on every call
// after, because the stream is already exhausted. validate's load path reads
// the spec twice per invocation (once via getSyverConfigPeek to check for a
// discovery: section, once via the real load). Buffering here, once, fixes
// both callers without restructuring the
// peek/load sequencing in discovery_load.go.
func readStdinOnce() ([]byte, error) {
	stdinOnce.Do(func() {
		stdinData, stdinErr = io.ReadAll(os.Stdin)
	})
	return stdinData, stdinErr
}

var (
	errCannotDetermineFormat = errors.New("unable to determine format from content")
	errMaxDepth              = errors.New("max depth of 50 reached, possibly due to dependency loop in goss file")
	errStoreFormatUnset      = errors.New("StoreFormat unset")
)

func getStoreFormatFromFileName(f string) (int, error) {
	ext := filepath.Ext(f)
	switch ext {
	case ".json":
		return JSON, nil
	case ".yaml", ".yml":
		return YAML, nil
	default:
		return 0, fmt.Errorf("unknown file extension: %v", ext)
	}
}

func getStoreFormatFromData(data []byte) (int, error) {
	var v any
	if err := unmarshalJSON(data, &v); err == nil {
		return JSON, nil
	}
	if err := unmarshalYAML(data, &v); err == nil {
		return YAML, nil
	}

	return 0, errCannotDetermineFormat
}

// ReadJSON Reads json file returning SyverConfig
func ReadJSON(filePath string) (SyverConfig, error) {
	cfg, _, err := readJSONFile(filePath, "")
	return cfg, err
}

// readJSONFile is ReadJSON with the strict origin threaded through; see
// readJSONData.
func readJSONFile(filePath, strictOrigin string) (SyverConfig, string, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return SyverConfig{}, "", fmt.Errorf("file error: %w", err)
	}

	return readJSONData(file, false, filePath, strictOrigin)
}

type TmplVars struct {
	Vars       map[string]any
	Discovered map[string]any
}

func (t *TmplVars) Env() map[string]string {
	env := make(map[string]string)
	for _, i := range os.Environ() {
		sep := strings.Index(i, "=")
		env[i[0:sep]] = i[sep+1:]
	}
	return env
}

func loadVars(varsFiles []string, varsInline string) (map[string]any, error) {
	mergedVars := map[string]any{}

	for _, varsFile := range varsFiles {
		vars, err := varsFromFile(varsFile)
		if err != nil {
			return nil, fmt.Errorf("loading vars file '%s'\n%w", varsFile, err)
		}
		if err := mergo.Merge(&mergedVars, vars, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("merging vars file '%s'\n%w", varsFile, err)
		}
	}

	varsExtra, err := varsFromString(varsInline)
	if err != nil {
		return nil, fmt.Errorf("loading inline vars\n%w", err)
	}

	for k, v := range varsExtra {
		mergedVars[k] = v
	}

	return mergedVars, nil
}

func loadVarsForTemplates(varsFiles []string, varsInline string, discovered map[string]bool) (map[string]any, error) {
	mergedVars, err := loadVars(varsFiles, "")
	if err != nil {
		return nil, err
	}

	if len(discovered) > 0 {
		disc := discoveredFromVars(mergedVars)
		for k, v := range discovered {
			disc[k] = v
		}
		mergedVars["Discovered"] = disc
	}

	varsExtra, err := varsFromString(varsInline)
	if err != nil {
		return nil, fmt.Errorf("loading inline vars\n%w", err)
	}

	for k, v := range varsExtra {
		mergedVars[k] = v
	}

	return mergedVars, nil
}

func discoveredFromVars(vars map[string]any) map[string]any {
	discovered := map[string]any{}
	if raw, ok := vars["Discovered"]; ok {
		if typed, ok := raw.(map[string]any); ok {
			for k, v := range typed {
				discovered[k] = v
			}
		}
	}
	return discovered
}

type varsFileEntry struct {
	data []byte
	err  error
}

var (
	varsFileCacheMu sync.Mutex
	varsFileCache   = map[string]varsFileEntry{}
)

// readVarsFile reads a --vars file. A regular file is read fresh on every
// call. Anything else (a pipe, a FIFO, the /dev/fd/N a shell passes for
// `--vars <(command)`, a character device) is read at most once per process,
// keyed by the path as given, and the same bytes (or the same error) are
// returned on every later call. validate loads the spec more than once per run
// (peek, then the real load, plus the discover spec with --discover), and each
// load renders templates with the vars; a pipe returns its data to the first
// read only. Same idea as readStdinOnce.
//
// If Stat fails the path is read as before, uncached, so a missing or
// unreadable file reports the same error it always did.
func readVarsFile(path string) ([]byte, error) {
	varsFileCacheMu.Lock()
	defer varsFileCacheMu.Unlock()
	if e, ok := varsFileCache[path]; ok {
		return e.data, e.err
	}
	if fi, statErr := os.Stat(path); statErr != nil || fi.Mode().IsRegular() {
		return os.ReadFile(path)
	}
	data, err := os.ReadFile(path)
	varsFileCache[path] = varsFileEntry{data: data, err: err}
	return data, err
}

func varsFromFile(varsFile string) (map[string]any, error) {
	vars := make(map[string]any)
	if varsFile == "" {
		return vars, nil
	}
	data, err := readVarsFile(varsFile)
	if err != nil {
		return vars, err
	}
	format, err := getStoreFormatFromData(data)
	if err != nil {
		return nil, err
	}
	if err := unmarshal(data, &vars, format); err != nil {
		return vars, err
	}
	return vars, nil
}

// ValidateVarsInline reports whether s is usable as the value of
// --vars-inline, without keeping the parsed result.
//
// It exists so the CLI can reject a bad value AT FLAG-PARSE TIME rather than
// several steps later while loading vars. The difference is not cosmetic. A
// shell that mangles the argument -- cmd.exe does, because it does not treat
// `'` as a quote character -- leaves syver holding a fragment plus a stray
// argument, and the stray one is then read as a subcommand, so the user gets a
// "No help topic" error naming that fragment, pointing nowhere near the flag.
// Validating here
// makes the message name --vars-inline and quote the value it actually
// received, which is what tells someone their shell split the command line.
func ValidateVarsInline(s string) error {
	_, err := varsFromString(s)
	return err
}

func varsFromString(varsString string) (map[string]any, error) {
	vars := make(map[string]any)
	if varsString == "" {
		return vars, nil
	}
	data := []byte(varsString)
	format, err := getStoreFormatFromData(data)
	if err != nil {
		return nil, err
	}

	if err := unmarshal(data, &vars, format); err != nil {
		return vars, err
	}
	return vars, nil
}

// ReadJSONData Reads json byte array returning SyverConfig. path names the
// spec this data came from and is used only in the top-level key guard's
// warnings and errors below, so an included gossfile's message names that
// file rather than its parent -- pass "" if there is none (e.g. content
// assembled in memory rather than read from a file).
func ReadJSONData(data []byte, detectFormat bool, path string) (SyverConfig, error) {
	cfg, _, err := readJSONData(data, detectFormat, path, "")
	return cfg, err
}

// readJSONData is ReadJSONData for a file that may be strict because of an
// ancestor. strictOrigin is the path of the nearest ancestor that declared
// syver-version:, or "" when none did. A file that declares the key itself
// becomes its own origin. The origin in effect for this file is returned so
// the caller can pass it on to this file's gossfile: includes.
//
// Strictness, and the minimum-version check, apply on the real load only. A
// peek pass (quietDecode) renders templates differently and can produce keys
// the real load does not, and its result is never used beyond discovery:.
func readJSONData(data []byte, detectFormat bool, path, strictOrigin string) (SyverConfig, string, error) {
	var err error
	if currentTemplateFilter != nil {
		data, err = currentTemplateFilter(data)
		if err != nil {
			return SyverConfig{}, "", err
		}
		// Logged, not printed: stdout carries the rendered spec, and a dump
		// written there would corrupt it for anything parsing the output. INFO
		// rather than DEBUG because --debug is an explicit request for it: at
		// DEBUG the default log level would hide what the flag asked for, while
		// -L WARN or -L ERROR still silences it.
		if debug {
			log.Printf("[INFO] file after text/template render:\n%s", data)
		}
	}

	format := outStoreFormat
	if detectFormat {
		format, err = getStoreFormatFromData(data)
		if err != nil {
			return SyverConfig{}, "", err
		}
	}

	// Top-level key guard: find any top-level key the real decode below is
	// about to silently drop, and whether this file declares syver-version:.
	// Skipped entirely on a peek pass, for the same reason the
	// gossfile:/syverfile: collision WARN a few lines down is gated -- every
	// spec is decoded twice per validate invocation (peek, then the real
	// load), and an ungated warning here would fire twice per unknown key.
	var scan topLevelScan
	if !quietDecode {
		switch format {
		case YAML:
			scan = scanTopLevelYAML(data, path)
		case JSON:
			scan = scanTopLevelJSON(data, path)
		}
	}
	// On a peek pass the scan is empty, so no file there is strict.
	origin := strictOrigin
	if scan.marker {
		origin = specLabel(path)
	}
	strict := origin != ""

	// Not strict: each unknown key is a WARN. YAML only; an unmarked JSON
	// spec is not checked (see docs/gossfile.md).
	if !strict && format == YAML {
		for _, w := range scan.unknown {
			log.Printf("[WARN] %s", w)
		}
	}

	syverConfig := NewSyverConfig()
	// Horrible, but will do for now
	if err := unmarshal(data, syverConfig, format); err != nil {
		return *syverConfig, "", err
	}

	// Strict: the value problems and the unknown keys come back as one error,
	// returned rather than logged so it can only surface once.
	if strict {
		var errs []error
		if err := checkDeclaredVersion(path, scan, util.Version); err != nil {
			errs = append(errs, err)
		}
		if len(scan.unknown) > 0 {
			errs = append(errs, strictKeysError(path, origin, scan.unknown))
		}
		if len(errs) > 0 {
			return SyverConfig{}, "", errors.Join(errs...)
		}
	}

	// Fold the syverfile: input alias into the canonical gossfile: map.
	// gossfile:-tagged entries are already populated in Syverfiles by the
	// unmarshal above, so on a key collision the gossfile: value wins --
	// the loop below only ever adds a syverfile: entry when that key isn't
	// already present.
	for k, v := range syverConfig.SyverfileAlias {
		if _, dup := syverConfig.Syverfiles[k]; dup {
			if !quietDecode {
				log.Printf("[WARN] %q declared under both gossfile: and syverfile:", k)
			}
			continue
		}
		syverConfig.Syverfiles[k] = v
	}
	syverConfig.SyverfileAlias = nil

	return *syverConfig, origin, nil
}

// RenderJSON reads json file recursively returning string
func RenderJSON(c *util.Config) (string, error) {
	err := setLogLevel(c)
	if err != nil {
		return "", err
	}
	debug = c.Debug
	currentTemplateFilter, err = NewTemplateFilter(c.VarsFiles, c.VarsInline, nil)
	if err != nil {
		return "", err
	}

	outStoreFormat, err = getStoreFormatFromFileName(c.Spec)
	if err != nil {
		return "", err
	}

	j, origin, err := readJSONFile(c.Spec, "")
	if err != nil {
		return "", err
	}

	syverConfig, err := mergeJSONData(j, 0, filepath.Dir(c.Spec), origin)
	if err != nil {
		return "", err
	}

	b, err := marshal(syverConfig)
	if err != nil {
		return "", fmt.Errorf("rendering failed: %w", err)
	}

	return string(b), nil
}

// mergeJSONData resolves syverConfig's gossfile: includes, found relative to
// path, and merges them in. strictOrigin is the file whose syver-version:
// makes these includes strict, or "" (see readJSONData). The result carries
// the highest syver-version: among the files read; a skipped include is not
// read and does not count.
func mergeJSONData(syverConfig SyverConfig, depth int, path, strictOrigin string) (SyverConfig, error) {
	depth++
	if depth >= 50 {
		return SyverConfig{}, errMaxDepth
	}
	// Our return syverConfig
	ret := *NewSyverConfig()
	ret = mergeSyver(ret, syverConfig)
	// Merge only touches the resource maps and Discovery.
	ret.SyverVersion = higherSyverVersion(syverConfig.SyverVersion, "")

	// Sort the gossfiles to ensure consistent ordering
	var keys []string
	for k := range syverConfig.Syverfiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Merge gossfiles in sorted order
	for _, k := range keys {
		g := syverConfig.Syverfiles[k]
		var fpath string
		if isRootedIncludePath(g.GetSyverfile()) {
			fpath = g.GetSyverfile()
		} else {
			fpath = filepath.Join(path, g.GetSyverfile())
		}
		if g.GetSkip() {
			// Do not process gossfiles with the skip attribute
			continue
		}
		matches, err := filepath.Glob(fpath)
		if err != nil {
			return ret, fmt.Errorf("error in expanding glob pattern: %w", err)
		}
		if matches == nil {
			return ret, fmt.Errorf("no matched files were found: %q", fpath)
		}
		for _, match := range matches {
			fdir := filepath.Dir(match)
			j, origin, err := readJSONFile(match, strictOrigin)
			if err != nil {
				return SyverConfig{}, fmt.Errorf("could not read json data in %s: %w", match, err)
			}
			j, err = mergeJSONData(j, depth, fdir, origin)
			if err != nil {
				return ret, fmt.Errorf("could not write json data: %w", err)
			}
			ret = mergeSyver(ret, j)
			ret.SyverVersion = higherSyverVersion(ret.SyverVersion, j.SyverVersion)
		}
	}
	return ret, nil
}

func WriteJSON(filePath string, syverConfig SyverConfig) error {
	jsonData, err := marshal(syverConfig)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	// check if the auto added json data is empty before writing to file.
	emptyConfig := *NewSyverConfig()
	emptyData, err := marshal(emptyConfig)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	if string(emptyData) == string(jsonData) {
		return fmt.Errorf("can't write empty configuration file %s, please check resource name(s)", filePath)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	return nil
}

func resourcePrint(fileName string, res resource.ResourceRead, announce bool) {
	typ := reflect.TypeOf(res)
	typs := strings.Split(typ.String(), ".")[1]

	if announce {
		fmt.Printf("Adding %s to '%s': %s\n", typs, fileName, res.ID())
	}
}

func marshal(syverConfig any) ([]byte, error) {
	switch outStoreFormat {
	case JSON:
		return marshalJSON(syverConfig)
	case YAML:
		return marshalYAML(syverConfig)
	default:
		return nil, errStoreFormatUnset
	}
}

func unmarshal(data []byte, v any, storeFormat int) error {
	switch storeFormat {
	case JSON:
		return unmarshalJSON(data, v)
	case YAML:
		return unmarshalYAML(data, v)
	default:
		return errStoreFormatUnset
	}
}

func marshalJSON(syverConfig any) ([]byte, error) {
	return json.MarshalIndent(syverConfig, "", "    ")
}

func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func marshalYAML(syverConfig any) ([]byte, error) {
	// yaml.v3 always indents block sequences under their parent key; yaml.v2 uses
	// indentless sequences, matching the format `goss add`-generated gossfiles have
	// always had. Kept on v2 for writes only -- unmarshalYAML below still uses v3.
	//
	// v3 CAN now match the sequence indentation -- Encoder.CompactSeqIndent(),
	// added after v3.0.1, does exactly that -- but it still does not reproduce
	// v2's output. v2 folds long scalars at ~80 columns and v3 emits them on one
	// line, so consolidating onto v3 was measured to reflow 7 of the 204 goldens
	// (long `exec:` commands in the windows and linux fixtures). That would
	// silently rewrite those lines in any gossfile the next `syver add` touched,
	// for no functional gain, so the marshal side stays on v2. (The seven are
	// long `exec:` commands in the per-platform integration fixtures: 3 windows,
	// 2 darwin, 2 linux.)
	//
	// Both are on go.yaml.in/yaml, the maintained fork: gopkg.in/yaml.v2 AND v3
	// were archived together, so v3 was never the "supported" option, the fork is.
	return yamlv2.Marshal(syverConfig)
}

func unmarshalYAML(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}
