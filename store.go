package syver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"dario.cat/mergo"
	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"

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

	return ReadJSONData(file, false)
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

// ReadJSONData Reads json byte array returning SyverConfig
func ReadJSONData(data []byte, detectFormat bool) (SyverConfig, error) {
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

	syverConfig := NewSyverConfig()
	// Horrible, but will do for now
	if err := unmarshal(data, syverConfig, format); err != nil {
		return *syverConfig, err
	}

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
	return yamlv2.Marshal(syverConfig)
}

func unmarshalYAML(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}
