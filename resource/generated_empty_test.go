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

// The five cases above were the ones found by inspecting `syver add` output.
// That was the blind spot: the add goldens cover neither http nor an interface
// with no addresses, so six more generators were emitting empty lists
// unnoticed. This table drives every remaining one through its real New*
// constructor with an empty-returning fake, so the next addition is covered by
// construction rather than by whoever remembers to look.
//
// omitempty does NOT save any of these: the fields are `matcher`, which is an
// interface, and yaml.v2's isZero for an interface field is IsNil() -- true
// only for a nil interface, never for one holding an empty or typed-nil slice.
func TestNoGeneratorEmitsAnEmptyList(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func() (any, error)
		keys []string
	}{
		{"file", func() (any, error) { return NewFile(conformanceFile{}, util.Config{}) }, []string{"contents:"}},
		{"port", func() (any, error) {
			return NewPort(&fakeSysPort{port: "tcp:22", listening: true}, util.Config{})
		}, []string{"ip:", "pid:"}},
		{"http", func() (any, error) { return NewHTTP(conformanceHTTP{}, util.Config{}) }, []string{"body:"}},
		{"process", func() (any, error) { return NewProcess(emptyProcess{}, util.Config{}) }, []string{"status:", "user:"}},
		{"interface", func() (any, error) { return NewInterface(emptyInterface{}, util.Config{}) }, []string{"addrs:"}},
		{"dns", func() (any, error) { return NewDNS(emptyDNS{}, util.Config{}) }, []string{"addrs:"}},
		{"user", func() (any, error) { return NewUser(emptyUser{}, util.Config{}) }, []string{"groups:"}},
		{"mount", func() (any, error) { return NewMount(emptyMount{}, util.Config{}) }, []string{"opts:", "vfs-opts:"}},
		{"package", func() (any, error) { return NewPackage(emptyPackage{}, util.Config{}) }, []string{"versions:"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.make()
			assert.NilError(t, err)
			out, err := yaml.Marshal(res)
			assert.NilError(t, err)
			assert.Assert(t, !strings.Contains(string(out), ": []"),
				"%s generator emitted an empty list:\n%s", tc.name, out)
			for _, k := range tc.keys {
				assert.Assert(t, !strings.Contains(string(out), k),
					"%s generator emitted %q when the system reported nothing:\n%s", tc.name, k, out)
			}
		})
	}
}

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

// Fakes that report nothing: each returns an empty (or typed-nil) slice with a
// nil error, which is exactly the shape the real system layer produces for a
// down interface, an unresolvable host, a user whose primary GID has no
// /etc/group entry, and so on. A nil error is the point -- it is what slips
// past the `err == nil` guard the generators use.
type emptyInterface struct{}

func (emptyInterface) Name() string          { return "iface-fake" }
func (emptyInterface) Exists() (bool, error) { return true, nil }
func (emptyInterface) Addrs() ([]string, error) {
	var typedNil []string // what system/interface.go's `var ret []string` yields
	return typedNil, nil
}
func (emptyInterface) MTU() (int, error) { return 1500, nil }

type emptyDNS struct{}

func (emptyDNS) Host() string              { return "dns-fake" }
func (emptyDNS) Exists() (bool, error)     { return true, nil }
func (emptyDNS) Addrs() ([]string, error)  { return []string{}, nil }
func (emptyDNS) Resolvable() (bool, error) { return false, nil }
func (emptyDNS) Server() string            { return "" }
func (emptyDNS) Qtype() string             { return "" }

type emptyUser struct{}

func (emptyUser) Username() string          { return "user-fake" }
func (emptyUser) Exists() (bool, error)     { return true, nil }
func (emptyUser) UID() (int, error)         { return 0, nil }
func (emptyUser) GID() (int, error)         { return 0, nil }
func (emptyUser) Groups() ([]string, error) { return []string{}, nil }
func (emptyUser) Home() (string, error)     { return "/root", nil }
func (emptyUser) Shell() (string, error)    { return "/bin/sh", nil }

type emptyMount struct{}

func (emptyMount) MountPoint() string          { return "/mount-fake" }
func (emptyMount) Exists() (bool, error)       { return true, nil }
func (emptyMount) Opts() ([]string, error)     { return []string{}, nil }
func (emptyMount) VfsOpts() ([]string, error)  { return []string{}, nil }
func (emptyMount) Source() (string, error)     { return "", nil }
func (emptyMount) Filesystem() (string, error) { return "", nil }
func (emptyMount) Usage() (int, error)         { return 0, nil }

type emptyPackage struct{}

func (emptyPackage) Name() string                { return "pkg-fake" }
func (emptyPackage) Exists() (bool, error)       { return true, nil }
func (emptyPackage) Installed() (bool, error)    { return true, nil }
func (emptyPackage) Versions() ([]string, error) { return []string{}, nil }
