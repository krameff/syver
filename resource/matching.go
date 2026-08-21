package resource

import (
	"fmt"
	"io"
	"strings"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Matching struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	Content       any     `json:"content,omitempty" yaml:"content,omitempty"`
	AsReader      bool    `json:"as-reader,omitempty" yaml:"as-reader,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Matches       matcher `json:"matches" yaml:"matches"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	MatchingResourceKey  = "matching"
	MatchingResourceName = "Matching"
)

// MatchingMap's hand-written duplicate of the genny-generated map (this file
// used to define its own UnmarshalJSON/UnmarshalYAML here, byte-identical to
// every other type's) collapses into the shared generic -- see the
// MatchingMap alias in resource_map.go.

// matching gains a registration for the first time here (FEAT-007 S4.1).
// It had none before this -- no init() at all in this file -- because
// goss-modular couldn't register it without colliding with the
// MatchingResourceKey = "mount" copy-paste bug it never fixed. Syver fixed
// that bug in 802454a, so it can register cleanly.
//
// InValidation: true, matching SyverConfig.Resources()'s existing generic
// map list (which already includes c.Matchings). InDiscovery: true, matching
// DiscoveryConfig's existing Matchings field. AppendSys: nil -- matching has
// no `syver add` subcommand and never has; there's nothing to build from
// live system state (see fromSystem's stub below). AutoAdd: nil, for the
// same reason.
func init() {
	Register(Descriptor{
		Key:          MatchingResourceKey,
		Name:         MatchingResourceName,
		New:          func() Resource { return &Matching{} },
		InValidation: true,
		InDiscovery:  true,
	})
}

func (a *Matching) ID() string      { return a.id }
func (a *Matching) SetID(id string) { a.id = id }

// SetSkip is the one deliberate behaviour change in FEAT-007 (G2, AC-7):
// this used to be a no-op, which is a live bug, not a latent one --
// `matching` IS in SyverConfig.Resources() (unlike `gossfile`), so it does
// reach applyDisabledTypes (dependency_scheduler.go), whose only action is
// calling SetSkip(). Before this fix,
// util.WithDisabledResourceTypes("matching") produced total=1 skipped=0
// failed=1 instead of the expected total=1 skipped=1 failed=0 -- disabling
// the type didn't disable it. No golden exercises DisabledResourceTypes (it
// has no CLI flag, only util.WithDisabledResourceTypes, a library-only
// option), so this changes no golden and AC-1 stays valid. See
// TestMatchingSetSkipDisablesValidation in matching_test.go for the
// regression test.
func (a *Matching) SetSkip()         { a.Skip = true }
func (a *Matching) TypeKey() string  { return MatchingResourceKey }
func (a *Matching) TypeName() string { return MatchingResourceName }

// FIXME: Can this be refactored?
func (r *Matching) GetTitle() string { return r.Title }
func (r *Matching) GetMeta() meta    { return r.Meta }

func (a *Matching) Validate(sys *system.System) []TestResult {
	skip := a.Skip

	var stub interface{}
	if a.AsReader {
		s := fmt.Sprintf("%v", a.Content)
		// ValidateValue expects a function
		stub = func() (io.Reader, error) {
			return strings.NewReader(s), nil
		}
	} else {
		// ValidateValue expects a function
		stub = func() (any, error) {
			return a.Content, nil
		}
	}

	var results []TestResult
	results = append(results, ValidateValue(a, "matches", a.Matches, stub, skip))
	return results
}

// fromSystem is a stub: matching resources have no system backing (they
// validate a literal `content:` value, not anything read off the host), so
// Descriptor.AppendSys is nil for matching and this is never actually
// called. It exists only to satisfy ResourceMap's PT constraint so
// MatchingMap can still share the generic AppendSysResource/
// AppendSysResourceIfExists/UnmarshalJSON/UnmarshalYAML implementation for
// its Unmarshal path.
func (a *Matching) fromSystem(sys *system.System, key string, config util.Config) (any, error) {
	return nil, fmt.Errorf("matching resources do not support `syver add` (no system backing)")
}
