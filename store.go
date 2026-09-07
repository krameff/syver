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
	file, err := os.ReadFile(filePath)
	if err != nil {
		return SyverConfig{}, fmt.Errorf("file error: %w", err)
	}

	return ReadJSONData(file, false, filePath)
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

func varsFromFile(varsFile string) (map[string]any, error) {
	vars := make(map[string]any)
	if varsFile == "" {
		return vars, nil
	}
	data, err := os.ReadFile(varsFile)
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
// spec this data came from and is used only for the top-level key guard's
// warnings below (D7 in PLAN_toplevel_key_guard.md) -- pass "" if there is
// none (e.g. content assembled in memory rather than read from a file).
func ReadJSONData(data []byte, detectFormat bool, path string) (SyverConfig, error) {
	var err error
	if currentTemplateFilter != nil {
		data, err = currentTemplateFilter(data)
		if err != nil {
			return SyverConfig{}, err
		}
		if debug {
			fmt.Println("DEBUG: file after text/template render")
			fmt.Println(string(data))
		}
	}

	format := outStoreFormat
	if detectFormat {
		format, err = getStoreFormatFromData(data)
		if err != nil {
			return SyverConfig{}, err
		}
	}

	// Top-level key guard (D1/D5): report any top-level key the real decode
	// below is about to silently drop. YAML only (D4 -- JSON is a known,
	// documented gap, see docs/gossfile.md) and gated on quietDecode for the
	// same reason the gossfile:/syverfile: collision WARN a few lines down
	// is -- every spec is decoded twice per validate invocation (peek, then
	// the real load), and an ungated warning here would fire twice per
	// unknown key, which is the exact defect BUG-001 already fixed once.
	if format == YAML && !quietDecode {
		for _, w := range checkTopLevelKeys(data, path) {
			log.Printf("[WARN] %s", w)
		}
	}

	syverConfig := NewSyverConfig()
	// Horrible, but will do for now
	if err := unmarshal(data, syverConfig, format); err != nil {
		return *syverConfig, err
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

	return *syverConfig, nil
}

// RenderJSON reads json file recursively returning string
func RenderJSON(c *util.Config) (string, error) {
	var err error
	debug = c.Debug
	currentTemplateFilter, err = NewTemplateFilter(c.VarsFiles, c.VarsInline, nil)
	if err != nil {
		return "", err
	}

	outStoreFormat, err = getStoreFormatFromFileName(c.Spec)
	if err != nil {
		return "", err
	}

	j, err := ReadJSON(c.Spec)
	if err != nil {
		return "", err
	}

	syverConfig, err := mergeJSONData(j, 0, filepath.Dir(c.Spec))
	if err != nil {
		return "", err
	}

	b, err := marshal(syverConfig)
	if err != nil {
		return "", fmt.Errorf("rendering failed: %w", err)
	}

	return string(b), nil
}

func mergeJSONData(syverConfig SyverConfig, depth int, path string) (SyverConfig, error) {
	depth++
	if depth >= 50 {
		return SyverConfig{}, errMaxDepth
	}
	// Our return syverConfig
	ret := *NewSyverConfig()
	ret = mergeSyver(ret, syverConfig)

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
		if strings.HasPrefix(g.GetSyverfile(), "/") {
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
			j, err := ReadJSON(match)
			if err != nil {
				return SyverConfig{}, fmt.Errorf("could not read json data in %s: %w", match, err)
			}
			j, err = mergeJSONData(j, depth, fdir)
			if err != nil {
				return ret, fmt.Errorf("could not write json data: %w", err)
			}
			ret = mergeSyver(ret, j)
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
		log.Printf("Can't write empty configuration file. Please check resource name(s).")
		return nil
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
