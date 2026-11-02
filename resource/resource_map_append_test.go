package resource

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

// fakeSysGroup is a system.Group whose answers are fixed by the test.
type fakeSysGroup struct {
	name      string
	exists    bool
	existsErr error
	gid       int
}

func (f fakeSysGroup) Groupname() string     { return f.name }
func (f fakeSysGroup) Exists() (bool, error) { return f.exists, f.existsErr }
func (f fakeSysGroup) GID() (int, error)     { return f.gid, nil }
func groupSys(groups ...fakeSysGroup) *system.System {
	return &system.System{
		NewGroup: func(_ context.Context, name string, _ *system.System, _ util.Config) system.Group {
			for _, g := range groups {
				if g.name == name {
					return g
				}
			}
			return fakeSysGroup{name: name}
		},
	}
}

func TestAppendSysResource(t *testing.T) {
	sys := groupSys(fakeSysGroup{name: "wheel", exists: true, gid: 10})

	t.Run("stores a fresh resource under its ID", func(t *testing.T) {
		m := GroupMap{}
		res, err := m.AppendSysResource("wheel", sys, util.Config{})
		if err != nil {
			t.Fatal(err)
		}
		if m["wheel"] != res {
			t.Fatalf("map[wheel] = %v, want the returned resource %v", m["wheel"], res)
		}
		if res.Exists != true || res.GID != 10 {
			t.Errorf("resource = {Exists: %v, GID: %v}, want {true, 10}", res.Exists, res.GID)
		}
	})

	t.Run("re-adding keeps the existing Title and Meta", func(t *testing.T) {
		m := GroupMap{"wheel": {Title: "admins", Meta: meta{"owner": "ops"}, GID: 99}}
		res, err := m.AppendSysResource("wheel", sys, util.Config{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Title != "admins" || res.Meta["owner"] != "ops" {
			t.Errorf("Title/Meta = %q/%v, want admins/map[owner:ops]", res.Title, res.Meta)
		}
		if res.GID != 10 {
			t.Errorf("GID = %v, want the live value 10, not the old 99", res.GID)
		}
		if m["wheel"] != res {
			t.Error("existing entry was not replaced")
		}
	})

	t.Run("a lookup error stores nothing", func(t *testing.T) {
		bad := groupSys(fakeSysGroup{name: "broken", existsErr: errors.New("ldap unreachable")})
		m := GroupMap{}
		res, err := m.AppendSysResource("broken", bad, util.Config{})
		if err == nil || res != nil {
			t.Fatalf("got (%v, %v), want (nil, error)", res, err)
		}
		if len(m) != 0 {
			t.Errorf("map = %v, want empty", m)
		}
	})
}

func TestAppendSysResourceIfExists(t *testing.T) {
	sys := groupSys(
		fakeSysGroup{name: "wheel", exists: true, gid: 10},
		fakeSysGroup{name: "nobody-here", exists: false},
	)

	t.Run("present resource is stored", func(t *testing.T) {
		m := GroupMap{}
		res, sysRes, ok, err := m.AppendSysResourceIfExists("wheel", sys)
		if err != nil || !ok {
			t.Fatalf("got ok=%v err=%v, want ok=true err=nil", ok, err)
		}
		if m["wheel"] != res {
			t.Error("resource not stored under its ID")
		}
		if sysRes.Groupname() != "wheel" {
			t.Errorf("system resource = %q, want wheel", sysRes.Groupname())
		}
	})

	t.Run("absent resource is returned but not stored", func(t *testing.T) {
		m := GroupMap{}
		res, _, ok, err := m.AppendSysResourceIfExists("nobody-here", sys)
		if err != nil || ok {
			t.Fatalf("got ok=%v err=%v, want ok=false err=nil", ok, err)
		}
		if res == nil {
			t.Error("resource should still be returned")
		}
		if len(m) != 0 {
			t.Errorf("map = %v, want empty", m)
		}
	})

	t.Run("re-adding keeps the existing Title and Meta", func(t *testing.T) {
		m := GroupMap{"wheel": {Title: "admins", Meta: meta{"k": "v"}}}
		res, _, ok, err := m.AppendSysResourceIfExists("wheel", sys)
		if err != nil || !ok {
			t.Fatalf("got ok=%v err=%v", ok, err)
		}
		if res.Title != "admins" || res.Meta["k"] != "v" {
			t.Errorf("Title/Meta = %q/%v, want admins/map[k:v]", res.Title, res.Meta)
		}
	})

	t.Run("a lookup error is returned", func(t *testing.T) {
		bad := groupSys(fakeSysGroup{name: "broken", existsErr: errors.New("ldap unreachable")})
		m := GroupMap{}
		res, _, ok, err := m.AppendSysResourceIfExists("broken", bad)
		if err == nil || ok || res != nil {
			t.Fatalf("got (%v, ok=%v, %v), want (nil, false, error)", res, ok, err)
		}
	})
}

// probeRes reaches the Exists()-error branch of AppendSysResourceIfExists,
// which Group cannot: NewGroup already propagates an Exists error from
// fromSystem. It reuses Group for everything except fromSystem, whose system
// value reports a fixed Exists error.
type probeRes struct {
	Group
}

type probeSys struct{ err error }

func (p probeSys) Exists() (bool, error) { return false, p.err }

var probeErr error

func (p *probeRes) fromSystem(_ *system.System, key string, _ util.Config) (probeSys, error) {
	p.SetID(key)
	return probeSys{err: probeErr}, nil
}

func TestAppendSysResourceIfExistsExistsError(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	tests := []struct {
		name     string
		err      error
		wantWarn bool
	}{
		{"lookup that could not run warns", errors.New("permission denied"), true},
		{"unsupported platform warns", system.ErrMountUnsupported, true},
		{"mount not found is an answer, not a failure", system.ErrMountpointNotFound, false},
		{"service not found is an answer, not a failure", system.ErrServiceNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			probeErr = tt.err
			m := ResourceMap[probeRes, probeSys, *probeRes]{}
			_, _, ok, err := m.AppendSysResourceIfExists("thing", nil)
			if err != nil || ok {
				t.Fatalf("got ok=%v err=%v, want ok=false err=nil (skipped, run continues)", ok, err)
			}
			if len(m) != 0 {
				t.Errorf("map = %v, want empty", m)
			}
			warned := strings.Contains(buf.String(), `[WARN] autoadd: skipping "thing": `+tt.err.Error())
			if warned != tt.wantWarn {
				t.Errorf("warned = %v, want %v; log was %q", warned, tt.wantWarn, buf.String())
			}
		})
	}
}

func TestCopyTitleMeta(t *testing.T) {
	t.Run("pointer to pointer", func(t *testing.T) {
		dst := &Group{GID: 1}
		copyTitleMeta(dst, &Group{Title: "t", Meta: meta{"a": 1}, GID: 2})
		if dst.Title != "t" || dst.Meta["a"] != 1 || dst.GID != 1 {
			t.Errorf("dst = %+v, want Title t, Meta a=1, GID untouched", dst)
		}
	})

	t.Run("value source", func(t *testing.T) {
		dst := &Group{}
		copyTitleMeta(dst, Group{Title: "t"})
		if dst.Title != "t" {
			t.Errorf("Title = %q, want t", dst.Title)
		}
	})

	t.Run("empty source clears the destination", func(t *testing.T) {
		dst := &Group{Title: "fresh", Meta: meta{"x": 1}}
		copyTitleMeta(dst, &Group{})
		if dst.Title != "" || dst.Meta != nil {
			t.Errorf("dst = %+v, want Title and Meta copied as empty", dst)
		}
	})

	t.Run("non-pointer destination is left alone", func(t *testing.T) {
		dst := Group{Title: "keep"}
		copyTitleMeta(dst, &Group{Title: "other"})
		if dst.Title != "keep" {
			t.Errorf("Title = %q, want keep", dst.Title)
		}
	})

	t.Run("types without the fields do not panic", func(t *testing.T) {
		type bare struct{ Name string }
		dst := &bare{Name: "n"}
		copyTitleMeta(dst, &Group{Title: "t"})
		copyTitleMeta(&Group{}, &bare{})
		if dst.Name != "n" {
			t.Errorf("Name = %q, want n", dst.Name)
		}
	})
}
