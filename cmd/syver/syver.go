package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/krameff/syver"
	"github.com/krameff/syver/outputs"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

// specFilenameCandidates is the probe order used by resolveSpecPath when
// the user didn't explicitly pass --syverfile/--gossfile/-g.
var specFilenameCandidates = []string{"syver.yaml", "syver.yml", "goss.yaml", "goss.yml"}

// resolveSpecPath returns the effective config file path for this
// invocation: whatever the user explicitly passed via --syverfile/
// --gossfile/-g, or (if unset) the first of specFilenameCandidates that
// exists in the current directory, or (if none exist) "./syver.yaml" as
// the default target for a brand-new file. Warns if more than one
// candidate exists simultaneously, since that's an ambiguous state the
// user should resolve themselves.
//
// Every one of the ~18 c.String("gossfile") read call sites in this file
// was replaced with resolveSpecPath(c), and it's also the fileName
// argument passed to AddResources/AutoAddResources (the write path,
// add.go:14-42) -- so read and write are structurally guaranteed to
// resolve to the same file, with no separate write-target decision
// needed.
func resolveSpecPath(c *cli.Command) string {
	if c.IsSet("syverfile") {
		return c.String("syverfile")
	}
	var found []string
	for _, candidate := range specFilenameCandidates {
		if _, err := os.Stat(candidate); err == nil {
			found = append(found, candidate)
		}
	}
	if len(found) == 0 {
		return "./syver.yaml"
	}
	if len(found) > 1 {
		log.Printf("[WARN] multiple config files present (%s) — using %q, remove the others to avoid ambiguity", strings.Join(found, ", "), found[0])
	}
	return found[0]
}

// converts a cli context into a goss Config
func newRuntimeConfigFromCLI(c *cli.Command) *util.Config {
	cfg := &util.Config{
		AllowInsecure:     c.Bool("insecure"),
		AnnounceToCLI:     true,
		Cache:             c.Duration("cache"),
		Debug:             c.Bool("debug"),
		LogLevel:          c.String("log-level"),
		Endpoint:          c.String("endpoint"),
		FormatOptions:     c.StringSlice("format-options"),
		IgnoreList:        c.StringSlice("exclude-attr"),
		ListenAddress:     c.String("listen-addr"),
		MaxConcurrent:     c.Int("max-concurrent"),
		NoFollowRedirects: c.Bool("no-follow-redirects"),
		OutputFormat:      c.String("format"),
		PackageManager:    c.String("package"),
		Password:          c.String("password"),
		Proxy:             c.String("proxy"),
		RetryTimeout:      c.Duration("retry-timeout"),
		Server:            c.String("server"),
		Sleep:             c.Duration("sleep"),
		Spec:              resolveSpecPath(c),
		Timeout:           c.Duration("timeout"),
		Username:          c.String("username"),
		VarsFiles:         c.StringSlice("vars"),
		VarsInline:        c.String("vars-inline"),
		DiscoverSpec:      c.String("discover"),
	}

	if c.Bool("no-color") {
		util.WithNoColor()(cfg)
	}

	if c.Bool("color") {
		util.WithColor()(cfg)
	}

	return cfg
}

// addSubcommandOrder preserves the exact original CLI listing order for
// `syver add` subcommands. Unrelated to fieldOrder/resourceOrder in the
// root package (those drive struct iteration and marshalling; this is
// purely a CLI presentation concern), so it gets its own list rather than
// reusing either.
var addSubcommandOrder = []string{
	"package", "file", "addr", "port", "service", "user", "group",
	"command", "dns", "process", "http", "gossfile", "kernel-param",
	"mount", "interface", "registry",
}

// addSubcommandUsage holds the per-type `syver add <type>` help text.
// Not part of resource.Descriptor: it's presentation-only, CLI-specific,
// and every other Descriptor field is meaningful outside cmd/syver too
// (validation, discovery, dispatch) where a help string wouldn't be.
var addSubcommandUsage = map[string]string{
	"package":      "add new package",
	"file":         "add new file",
	"addr":         "add new remote address:port - ex: google.com:80",
	"port":         "add new listening [protocol]:port - ex: 80 or udp:123",
	"service":      "add new service",
	"user":         "add new user",
	"group":        "add new group",
	"command":      "add new command",
	"dns":          "add new dns",
	"process":      "add new process name",
	"http":         "add new http",
	"gossfile":     "add new syver file, it will be imported from this one",
	"kernel-param": "add new goss kernel param",
	"mount":        "add new mount",
	"interface":    "add new interface",
	"registry":     "add new registry key",
}

// addSubcommands builds the `syver add` subcommand tree from
// resource.Descriptors() (FEAT-007 task 8) instead of the ~180-line
// hand-written list this used to be -- one *cli.Command per addable type
// (AppendSys != nil; today that's every type except matching).
//
// G4: the gossfile subcommand is named neither its Key ("gossfile") nor
// its Name ("Gossfile") -- it's "syver", aliased to "goss". A naive
// Key/Name-driven generator would rename it and break AC-1's `add`
// goldens, so CLIName/CLIAliases drive Name/Aliases here instead,
// defaulting to Key/nil when unset (every type but gossfile).
func addSubcommands() []*cli.Command {
	var cmds []*cli.Command
	for _, key := range addSubcommandOrder {
		desc, ok := resource.DescriptorByKey(key)
		if !ok || desc.AppendSys == nil {
			continue
		}

		name := desc.CLIName
		if name == "" {
			name = desc.Key
		}
		var flags []cli.Flag
		if desc.CLIFlags != nil {
			flags = desc.CLIFlags()
		}
		resourceName := desc.Name

		cmds = append(cmds, &cli.Command{
			Name:    name,
			Aliases: desc.CLIAliases,
			Usage:   addSubcommandUsage[key],
			Flags:   flags,
			Action: func(ctx context.Context, c *cli.Command) error {
				fatalAlphaIfNeeded(c)
				return syver.AddResources(resolveSpecPath(c), resourceName, c.Args().Slice(), newRuntimeConfigFromCLI(c))
			},
		})
	}
	return cmds
}

// newApp builds the syver CLI command tree. Split out from main() so
// tests can inspect the command tree (Name, flags, subcommand aliases)
// without invoking app.Run / os.Exit.
func newApp() *cli.Command {
	return &cli.Command{
		EnableShellCompletion: true,
		Version:               util.Version,
		Name:                  "syver",
		Usage:                 "Quick and Easy server validation",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Aliases: []string{"loglevel", "L", "l"},
				Value:   "INFO",
				Usage:   "Goss log verbosity level",
				Sources: nonEmptyEnvVars("SYVER_LOGLEVEL", "GOSS_LOGLEVEL"),
			},
			&cli.StringFlag{
				Name:    "syverfile",
				Aliases: []string{"gossfile", "g"},
				// Value intentionally omitted -- see resolveSpecPath, which
				// distinguishes "not set" from "set to a real value" via
				// c.IsSet, so the default can be resolved post-parse.
				Usage:   "Syver file to read from / write to",
				Sources: nonEmptyEnvVars("SYVER_FILE", "GOSS_FILE"),
			},
			&cli.StringSliceFlag{
				Name:    "vars",
				Usage:   "json/yaml file containing variables for template. Can be specified multiple times. Later files override overlapping keys.",
				Sources: nonEmptyEnvVars("SYVER_VARS", "GOSS_VARS"),
			},
			&cli.StringFlag{
				Name:    "vars-inline",
				Usage:   "json/yaml string containing variables for template (overwrites vars)",
				Sources: nonEmptyEnvVars("SYVER_VARS_INLINE", "GOSS_VARS_INLINE"),
			},
			&cli.StringFlag{
				Name:  "package",
				Usage: fmt.Sprintf("Package type to use [%s]", strings.Join(system.SupportedPackageManagers(), ", ")),
			},
		},
		Commands: []*cli.Command{
			{
				Name:    "validate",
				Aliases: []string{"v"},
				Usage:   "Validate system",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   "rspecish",
						Usage:   fmt.Sprintf("Format to output in, valid options: %s", outputs.Outputers()),
						Sources: nonEmptyEnvVars("SYVER_FMT", "GOSS_FMT"),
					},
					&cli.StringSliceFlag{
						Name:    "format-options",
						Aliases: []string{"o"},
						Usage:   fmt.Sprintf("Extra options passed to the formatter, valid options: %s", outputs.FormatOptions()),
						Sources: nonEmptyEnvVars("SYVER_FMT_OPTIONS", "GOSS_FMT_OPTIONS"),
					},
					&cli.BoolFlag{
						Name:    "color",
						Usage:   "Force color on",
						Sources: nonEmptyEnvVars("SYVER_COLOR", "GOSS_COLOR"),
					},
					&cli.BoolFlag{
						Name:    "no-color",
						Usage:   "Force color off",
						Sources: nonEmptyEnvVars("SYVER_NOCOLOR", "GOSS_NOCOLOR"),
					},
					&cli.DurationFlag{
						Name:    "sleep",
						Aliases: []string{"s"},
						Usage:   "Time to sleep between retries, only active when -r is set",
						Value:   1 * time.Second,
						Sources: nonEmptyEnvVars("SYVER_SLEEP", "GOSS_SLEEP"),
					},
					&cli.DurationFlag{
						Name:    "retry-timeout",
						Aliases: []string{"r"},
						Usage:   "Retry on failure so long as elapsed + sleep time is less than this",
						Value:   0,
						Sources: nonEmptyEnvVars("SYVER_RETRY_TIMEOUT", "GOSS_RETRY_TIMEOUT"),
					},
					&cli.IntFlag{
						Name:    "max-concurrent",
						Usage:   "Max number of tests to run concurrently",
						Value:   50,
						Sources: nonEmptyEnvVars("SYVER_MAX_CONCURRENT", "GOSS_MAX_CONCURRENT"),
					},
					&cli.StringFlag{
						Name:    "discover",
						Usage:   "Syverfile with discovery: tests to run before the main -g gossfile",
						Sources: nonEmptyEnvVars("SYVER_DISCOVER", "GOSS_DISCOVER"),
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					fatalAlphaIfNeeded(c)
					code, err := syver.Validate(newRuntimeConfigFromCLI(c))
					if err != nil {
						color.Red(fmt.Sprintf("Error: %v\n", err))
					}
					os.Exit(code)

					return nil
				},
			},
			{
				Name:    "serve",
				Aliases: []string{"s"},
				Usage:   "Serve a health endpoint",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   "rspecish",
						Usage:   fmt.Sprintf("Format to output in, valid options: %s", outputs.Outputers()),
						Sources: nonEmptyEnvVars("SYVER_FMT", "GOSS_FMT"),
					},
					&cli.StringSliceFlag{
						Name:    "format-options",
						Aliases: []string{"o"},
						Usage:   fmt.Sprintf("Extra options passed to the formatter, valid options: %s", outputs.FormatOptions()),
						Sources: nonEmptyEnvVars("SYVER_FMT_OPTIONS", "GOSS_FMT_OPTIONS"),
					},
					&cli.DurationFlag{
						Name:    "cache",
						Aliases: []string{"c"},
						Usage:   "Time to cache the results",
						Value:   5 * time.Second,
						Sources: nonEmptyEnvVars("SYVER_CACHE", "GOSS_CACHE"),
					},
					&cli.StringFlag{
						Name:    "listen-addr",
						Aliases: []string{"l"},
						Value:   ":8080",
						Usage:   "Address to listen on [ip]:port",
						Sources: nonEmptyEnvVars("SYVER_LISTEN", "GOSS_LISTEN"),
					},
					&cli.StringFlag{
						Name:    "endpoint",
						Aliases: []string{"e"},
						Value:   "/healthz",
						Usage:   "Endpoint to expose",
						Sources: nonEmptyEnvVars("SYVER_ENDPOINT", "GOSS_ENDPOINT"),
					},
					&cli.IntFlag{
						Name:    "max-concurrent",
						Usage:   "Max number of tests to run concurrently",
						Value:   50,
						Sources: nonEmptyEnvVars("SYVER_MAX_CONCURRENT", "GOSS_MAX_CONCURRENT"),
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					fatalAlphaIfNeeded(c)
					return syver.Serve(newRuntimeConfigFromCLI(c))
				},
			},
			{
				Name:    "render",
				Aliases: []string{"r"},
				Usage:   "render gossfile after imports",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "debug",
						Aliases: []string{"d"},
						Usage:   "Print debugging info when rendering",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					fatalAlphaIfNeeded(c)
					j, err := syver.RenderJSON(newRuntimeConfigFromCLI(c))
					if err != nil {
						return err
					}

					fmt.Print(j)

					return nil
				},
			},
			{
				Name:    "autoadd",
				Aliases: []string{"aa"},
				Usage:   "automatically add all matching resource to the test suite",
				Action: func(ctx context.Context, c *cli.Command) error {
					fatalAlphaIfNeeded(c)
					return syver.AutoAddResources(resolveSpecPath(c), c.Args().Slice(), newRuntimeConfigFromCLI(c))
				},
			},
			{
				Name:    "add",
				Aliases: []string{"a"},
				Usage:   "add a resource to the test suite",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:  "exclude-attr",
						Usage: "Exclude the following attributes when adding a new resource",
					},
				},
				Commands: addSubcommands(),
			},
		},
	}
}

func main() {
	cli.VersionPrinter = func(cmd *cli.Command) {
		fmt.Fprintf(cmd.Root().Writer, "%v version %v\nKrameff Solutions Ltd\n", cmd.Name, cmd.Version)
	}

	app := newApp()

	addAlphaFlagIfNeeded(app)
	err := app.Run(context.Background(), os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func addAlphaFlagIfNeeded(cmd *cli.Command) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		cmd.Flags = append(cmd.Flags, &cli.StringFlag{
			Name:    "use-alpha",
			Usage:   "goss on macOS/Windows is alpha-quality. Set to 1 to use anyway.",
			Sources: nonEmptyEnvVars("SYVER_USE_ALPHA", "GOSS_USE_ALPHA"),
			Value:   "0",
		})
	}
}

func fatalAlphaIfNeeded(c *cli.Command) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if c.String("use-alpha") != "1" {
			// Advertise the SYVER_ name: it is what the docs tell people to
			// set, and it is the one this flag reads first. GOSS_USE_ALPHA
			// keeps working -- see the Sources chain above -- it just is not
			// what a new user should be told to type.
			howto := map[string]string{
				"darwin":  "export SYVER_USE_ALPHA=1",
				"windows": "In cmd:        set SYVER_USE_ALPHA=1\nIn powershell: $env:SYVER_USE_ALPHA=1\nIn bash:       export SYVER_USE_ALPHA=1",
			}
			log.Printf(`Terminating.

To bypass this and use the binary anyway:

%s`, howto[runtime.GOOS])
			os.Exit(1)
		}
	}
}
