package resource

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

type Command struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Exec          string  `json:"exec,omitempty" yaml:"exec,omitempty"`
	ExitStatus    matcher `json:"exit-status" yaml:"exit-status"`
	Stdout        matcher `json:"stdout" yaml:"stdout"`
	Stderr        matcher `json:"stderr" yaml:"stderr"`
	Timeout       int     `json:"timeout" yaml:"timeout"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	CommandResourceKey  = "command"
	CommandResourceName = "Command"
)

func init() {
	Register(Descriptor{
		Key:          CommandResourceKey,
		Name:         CommandResourceName,
		New:          func() Resource { return &Command{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Command{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		CLIFlags: func() []cli.Flag {
			return []cli.Flag{&cli.DurationFlag{Name: "timeout", Value: 10 * time.Second}}
		},
	})
}

func (c *Command) ID() string       { return c.id }
func (c *Command) SetID(id string)  { c.id = id }
func (c *Command) SetSkip()         { c.Skip = true }
func (c *Command) TypeKey() string  { return CommandResourceKey }
func (c *Command) TypeName() string { return CommandResourceName }

func (c *Command) GetTitle() string { return c.Title }
func (c *Command) GetMeta() meta    { return c.Meta }
func (c *Command) GetExec() string {
	if c.Exec != "" {
		return c.Exec
	}
	return c.id
}

func (c *Command) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = withID(ctx, c.ID())
	skip := c.Skip

	if c.Timeout == 0 {
		c.Timeout = 10000
	}

	var results []TestResult
	sysCommand := sys.NewCommand(ctx, c.GetExec(), sys, util.Config{Timeout: time.Duration(c.Timeout) * time.Millisecond})

	cExitStatus := deprecateAtoI(c.ExitStatus, fmt.Sprintf("%s: command.exit-status", c.ID()))
	results = append(results, ValidateValue(c, "exit-status", cExitStatus, sysCommand.ExitStatus, skip))
	if isSetWarnEmpty(c.Stdout, fmt.Sprintf("%s: command.stdout", c.ID()), c.Skip) {
		results = append(results, ValidateValue(c, "stdout", c.Stdout, sysCommand.Stdout, skip))
	}
	if isSetWarnEmpty(c.Stderr, fmt.Sprintf("%s: command.stderr", c.ID()), c.Skip) {
		results = append(results, ValidateValue(c, "stderr", c.Stderr, sysCommand.Stderr, skip))
	}
	return results
}

func NewCommand(sysCommand system.Command, config util.Config) (*Command, error) {
	command := sysCommand.Command()
	exitStatus, err := sysCommand.ExitStatus()
	c := &Command{
		id:         command,
		ExitStatus: exitStatus,
		Stdout:     "",
		Stderr:     "",
		Timeout:    config.TimeOutMilliSeconds(),
	}

	if !contains(config.IgnoreList, "stdout") {
		stdout, _ := sysCommand.Stdout()
		outSlice := readerToSlice(stdout)
		if len(outSlice) != 0 {
			c.Stdout = outSlice
		}
	}
	if !contains(config.IgnoreList, "stderr") {
		stderr, _ := sysCommand.Stderr()
		errSlice := readerToSlice(stderr)
		if len(errSlice) != 0 {
			c.Stderr = errSlice
		}
	}

	return c, err
}

func escapePattern(s string) string {
	if strings.HasPrefix(s, "!") || strings.HasPrefix(s, "/") {
		return "\\" + s
	}
	return s
}

func readerToSlice(reader io.Reader) []string {
	scanner := bufio.NewScanner(reader)
	slice := []string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = escapePattern(line)
		if line != "" {
			slice = append(slice, line)
		}
	}

	return slice
}

// fromSystem builds a fresh Command from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewCommand" off
// *system.System from a type parameter alone.
func (c *Command) fromSystem(sys *system.System, key string, config util.Config) (system.Command, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewCommand(ctx, key, sys, config)
	n, err := NewCommand(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*c = *n
	return sysRes, nil
}
