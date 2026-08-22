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
}

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
}
