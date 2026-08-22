package syver

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/onsi/gomega/format"

	"github.com/krameff/syver/outputs"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

func getSyverConfig(varsFiles []string, varsInline string, specFile string, discovered map[string]bool) (cfg *SyverConfig, err error) {
	return loadSyverConfig(varsFiles, varsInline, specFile, discovered, false)
}

func getSyverConfigPeek(varsFiles []string, varsInline string, specFile string) (*SyverConfig, error) {
	return loadSyverConfig(varsFiles, varsInline, specFile, nil, true)
}

func loadSyverConfig(varsFiles []string, varsInline string, specFile string, discovered map[string]bool, peek bool) (cfg *SyverConfig, err error) {
	// quietDecode suppresses the alias-collision WARN during peek passes --
	// see the quietDecode declaration in store.go for why. Reset via defer
	// so it never leaks into a later, non-peek call.
	quietDecode = peek
	defer func() { quietDecode = false }()

	var path, source string
	var syverConfig SyverConfig

	if peek {
		currentTemplateFilter, err = NewPeekTemplateFilter(varsFiles, varsInline)
	} else {
		currentTemplateFilter, err = NewTemplateFilter(varsFiles, varsInline, discovered)
	}
	if err != nil {
		return nil, err
	}

	if specFile == "-" {
		source = "STDIN"
		// os.Stdin is a non-seekable stream -- loadSyverConfig runs twice per
		// validate invocation (peek, then the real load), so a naive
		// io.ReadAll(os.Stdin) here would exhaust the stream on the first call
		// and return 0 bytes on the second. readStdinOnce buffers it once and
		// replays the same bytes to every caller.
		data, err := readStdinOnce()
		if err != nil {
			return nil, err
		}
		outStoreFormat, err = getStoreFormatFromData(data)
		if err != nil {
			return nil, err
		}

		syverConfig, err = ReadJSONData(data, true)
		if err != nil {
			return nil, err
		}
	} else {
		source = specFile
		path = filepath.Dir(specFile)
		outStoreFormat, err = getStoreFormatFromFileName(specFile)
		if err != nil {
			return nil, err
		}

		syverConfig, err = ReadJSON(specFile)
		if err != nil {
			return nil, err
		}
	}

	syverConfig, err = mergeJSONData(syverConfig, 0, path)
	if err != nil {
		return nil, err
	}

	if len(syverConfig.Resources()) == 0 && syverConfig.Discovery.IsEmpty() {
		return nil, fmt.Errorf("found 0 tests, source: %v", source)
	}

	return &syverConfig, nil
}

func getOutputer(c *bool, format string) (outputs.Outputer, error) {
	if c != nil && *c {
		color.NoColor = true
	}
	if c != nil && !*c {
		color.NoColor = false
	}

	return outputs.GetOutputer(format)
}

// ValidateResults performs validation and provides programmatic access to validation results
// no retries or outputs are supported
func ValidateResults(ctx context.Context, c *util.Config) (results <-chan []resource.TestResult, err error) {
	syverConfig, err := loadSyverConfigWithDiscover(ctx, c)
	if err != nil {
		return nil, err
	}

	sys := system.New(c.PackageManager)

	return runValidation(ctx, sys, *syverConfig, c.DisabledResourceTypes, c.MaxConcurrent)
}

// Validate performs validation, writes formatted output to stdout by default
// and supports retries and more, this is the full featured Validate used
// by the CLI invocation and will produce output to StdOut.  Use
// ValidateResults for programmatic access
func Validate(ctx context.Context, c *util.Config) (code int, err error) {
	err = setLogLevel(c)
	if err != nil {
		return 1, err
	}
	syverConfig, err := loadSyverConfigWithDiscover(ctx, c)
	if err != nil {
		return 78, err
	}
	return ValidateConfig(ctx, c, syverConfig)
}

func ValidateConfig(ctx context.Context, c *util.Config, syverConfig *SyverConfig) (code int, err error) {
	if c.OutputFormat == "discovery" {
		return validateDiscoveryConfig(ctx, c, syverConfig)
	}

	// Needed for contains-elements
	format.UseStringerRepresentation = true
	outputConfig := util.OutputConfig{
		FormatOptions: c.FormatOptions,
	}

	sys := system.New(c.PackageManager)
	outputer, err := getOutputer(c.NoColor, c.OutputFormat)
	if err != nil {
		return 1, err
	}

	var ofh io.Writer
	ofh = os.Stdout
	if c.OutputWriter != nil {
		ofh = c.OutputWriter
	}

	sleep := c.Sleep
	retryTimeout := c.RetryTimeout
	i := 1
	startTime := time.Now()
	for {
		out, err := runValidation(ctx, sys, *syverConfig, c.DisabledResourceTypes, c.MaxConcurrent)
		if err != nil {
			return 1, err
		}
		exitCode := outputer.Output(ofh, out, outputConfig)
		if retryTimeout == 0 || exitCode == 0 {
			return exitCode, nil
		}
		elapsed := time.Since(startTime)
		if elapsed+sleep > retryTimeout {
			return 3, fmt.Errorf("timeout of %s reached before tests entered a passing state", retryTimeout)
		}
		color.Red("Retrying in %s (elapsed/timeout time: %.3fs/%s)\n\n\n", sleep, elapsed.Seconds(), retryTimeout)
		sys = system.New(c.PackageManager)
		time.Sleep(sleep)
		i++
		fmt.Printf("Attempt #%d:\n", i)
	}
}

func validateDiscoveryConfig(ctx context.Context, c *util.Config, syverConfig *SyverConfig) (code int, err error) {
	sys := system.New(c.PackageManager)
	discovered, err := validateDiscovery(ctx, sys, *syverConfig, c.MaxConcurrent)
	if err != nil {
		return 1, err
	}

	var ofh io.Writer = os.Stdout
	if c.OutputWriter != nil {
		ofh = c.OutputWriter
	}

	outputConfig := util.OutputConfig{
		FormatOptions: c.FormatOptions,
	}
	discoveryOutput := outputs.Discovery{}
	return discoveryOutput.Output(ofh, discovered, outputConfig), nil
}

func runValidation(ctx context.Context, sys *system.System, syverConfig SyverConfig, skipList []string, maxConcurrent int) (<-chan []resource.TestResult, error) {
	resources := syverConfig.Resources()
	applyDisabledTypes(resources, skipList)

	if hasDependencies(resources) {
		return validateWithDependencies(ctx, sys, resources, maxConcurrent)
	}

	return validateParallel(ctx, sys, resources, maxConcurrent), nil
}

func validateParallel(ctx context.Context, sys *system.System, resources []resource.Resource, maxConcurrent int) <-chan []resource.TestResult {
	out := make(chan []resource.TestResult)
	in := make(chan resource.Resource)

	go func() {
		for _, t := range resources {
			in <- t
		}
		close(in)
	}()

	workerCount := runtime.NumCPU() * 5
	if workerCount > maxConcurrent {
		workerCount = maxConcurrent
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range in {
				out <- f.Validate(ctx, sys)
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
