package resource

import (
	"strings"
	"testing"

	"github.com/krameff/syver/util"
	"gopkg.in/yaml.v2"
	"gotest.tools/v3/assert"
)

// An empty list asserts nothing -- isSet skips it (see TestIsSet). So a
// generator that writes one produces a line the reader has to read, think
// about, and dismiss, and any future "warn on an empty list matcher" check
// would fire on a file `syver add` had just written. These pin the two
// generators that used to do it: file.contents was hardcoded to []string{},
// and port.ip was assigned whatever the system returned, empty or not.
func TestGeneratorsDoNotEmitEmptyLists(t *testing.T) {
	t.Run("NewFile leaves contents unset", func(t *testing.T) {
		f, err := NewFile(conformanceFile{}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, f.Contents, nil)
	})

	t.Run("NewPort leaves ip unset when the port has no addresses", func(t *testing.T) {
		p, err := NewPort(&fakeSysPort{port: "tcp:22", listening: true, ip: []string{}}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, p.IP, nil)
	})

	t.Run("NewPort still records ip when the port has addresses", func(t *testing.T) {
		p, err := NewPort(&fakeSysPort{port: "tcp:22", listening: true, ip: []string{"0.0.0.0"}}, util.Config{})
		assert.NilError(t, err)
		assert.DeepEqual(t, p.IP, matcher([]string{"0.0.0.0"}))
	})

	t.Run("NewHTTP leaves body unset", func(t *testing.T) {
		u, err := NewHTTP(conformanceHTTP{}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, u.Body, nil)
	})

	t.Run("NewProcess leaves status and user unset when the process reports none", func(t *testing.T) {
		p, err := NewProcess(emptyProcess{}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, p.Status, nil)
		assert.Equal(t, p.User, nil)
	})

	t.Run("NewProcess still records status and user when reported", func(t *testing.T) {
		p, err := NewProcess(fullProcess{}, util.Config{})
		assert.NilError(t, err)
		assert.DeepEqual(t, p.Status, matcher([]string{"S"}))
		assert.DeepEqual(t, p.User, matcher([]string{"root"}))
	})
}

// Two process fakes rather than one configurable: what is being pinned is the
// boundary between "reports nothing" and "reports something", and naming the
// two cases reads better at the call site than a bool.
type emptyProcess struct{}

func (emptyProcess) Executable() string        { return "proc-fake" }
func (emptyProcess) Exists() (bool, error)     { return true, nil }
func (emptyProcess) Running() (bool, error)    { return true, nil }
func (emptyProcess) Status() ([]string, error) { return []string{}, nil }
func (emptyProcess) User() ([]string, error)   { return []string{}, nil }
func (emptyProcess) Pids() ([]int, error)      { return []int{}, nil }

type fullProcess struct{ emptyProcess }

func (fullProcess) Status() ([]string, error) { return []string{"S"}, nil }
func (fullProcess) User() ([]string, error)   { return []string{"root"}, nil }

// The generators above only stay clean if the marshalled output is clean too.
// `contents` was the one optional file attribute without `omitempty`, so an
// unset value rendered as `contents: null` rather than being omitted -- which
// is why `syver render` used to emit that line for every file that never
// mentioned contents. yaml.v2 is deliberate here: it is what store.go
// marshals with.
func TestUnsetOptionalAttributesAreOmitted(t *testing.T) {
	f, err := NewFile(conformanceFile{}, util.Config{})
	assert.NilError(t, err)
	out, err := yaml.Marshal(f)
	assert.NilError(t, err)
	assert.Assert(t, !strings.Contains(string(out), "contents"), "unexpected contents key in:\n%s", out)

	p, err := NewPort(&fakeSysPort{port: "tcp:22", listening: true}, util.Config{})
	assert.NilError(t, err)
	out, err = yaml.Marshal(p)
	assert.NilError(t, err)
	assert.Assert(t, !strings.Contains(string(out), "ip:"), "unexpected ip key in:\n%s", out)

	u, err := NewHTTP(conformanceHTTP{}, util.Config{})
	assert.NilError(t, err)
	out, err = yaml.Marshal(u)
	assert.NilError(t, err)
	assert.Assert(t, !strings.Contains(string(out), "body:"), "unexpected body key in:\n%s", out)

	pr, err := NewProcess(emptyProcess{}, util.Config{})
	assert.NilError(t, err)
	out, err = yaml.Marshal(pr)
	assert.NilError(t, err)
	for _, key := range []string{"status:", "user:"} {
		assert.Assert(t, !strings.Contains(string(out), key), "unexpected %s key in:\n%s", key, out)
	}
}
