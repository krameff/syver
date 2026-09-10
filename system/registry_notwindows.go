//go:build !windows

package system

import (
	"context"

	"github.com/krameff/syver/util"
)

type defRegistry struct {
	key string
}

func NewDefRegistry(_ context.Context, key string, system *System, config util.Config) Registry {
	return &defRegistry{key: key}
}

func (r *defRegistry) Key() string            { return r.key }
func (r *defRegistry) Exists() (bool, error)  { return false, ErrRegistryUnsupported }
func (r *defRegistry) Value() (string, error) { return "", ErrRegistryUnsupported }
func (r *defRegistry) Type() (string, error)  { return "", ErrRegistryUnsupported }

// SetView accepts and discards the view. Every accessor on this type already
// reports ErrRegistryUnsupported, so validating the string here would replace a
// clear "not supported on this platform" with a grammar complaint about an
// attribute that could never have been honoured anyway. The grammar is still
// checked off Windows: ParseRegistryView is untagged and unit-tested on Linux.
func (r *defRegistry) SetView(view string) error { return nil }
