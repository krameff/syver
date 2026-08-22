package resource

import (
	"context"
	"fmt"
	"os"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type File struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Path          string  `json:"path,omitempty" yaml:"path,omitempty"`
	Exists        matcher `json:"exists" yaml:"exists"`
	Mode          matcher `json:"mode,omitempty" yaml:"mode,omitempty"`
	Size          matcher `json:"size,omitempty" yaml:"size,omitempty"`
	Owner         matcher `json:"owner,omitempty" yaml:"owner,omitempty"`
	Uid           matcher `json:"uid,omitempty" yaml:"uid,omitempty"`
	Group         matcher `json:"group,omitempty" yaml:"group,omitempty"`
	Gid           matcher `json:"gid,omitempty" yaml:"gid,omitempty"`
	LinkedTo      matcher `json:"linked-to,omitempty" yaml:"linked-to,omitempty"`
	Filetype      matcher `json:"filetype,omitempty" yaml:"filetype,omitempty"`
	Contains      matcher `json:"contains,omitempty" yaml:"contains,omitempty"`
	Contents      matcher `json:"contents,omitempty" yaml:"contents,omitempty"`
	Md5           matcher `json:"md5,omitempty" yaml:"md5,omitempty"`
	Sha256        matcher `json:"sha256,omitempty" yaml:"sha256,omitempty"`
	Sha512        matcher `json:"sha512,omitempty" yaml:"sha512,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	FileResourceKey  = "file"
	FileResourceName = "File"
)

func init() {
	Register(Descriptor{
		Key:          FileResourceKey,
		Name:         FileResourceName,
		New:          func() Resource { return &File{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &File{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		AutoAdd: &AutoAddSpec{Order: 1},
	})
}

func (f *File) ID() string {
	if f.Path != "" && f.Path != f.id {
		return fmt.Sprintf("%s: %s", f.id, f.Path)
	}
	return f.id
}
func (f *File) SetID(id string)  { f.id = id }
func (f *File) SetSkip()         { f.Skip = true }
func (f *File) TypeKey() string  { return FileResourceKey }
func (f *File) TypeName() string { return FileResourceName }

func (f *File) GetTitle() string { return f.Title }
func (f *File) GetMeta() meta    { return f.Meta }
func (f *File) GetPath() string {
	if f.Path != "" {
		return f.Path
	}
	return f.id
}

func (f *File) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = context.WithValue(ctx, idKey{}, f.ID())
	skip := f.Skip
	sysFile := sys.NewFile(ctx, f.GetPath(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(f, "exists", f.Exists, sysFile.Exists, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSet(f.Mode) {
		results = append(results, ValidateValue(f, "mode", f.Mode, sysFile.Mode, skip))
	}
	if isSet(f.Owner) {
		results = append(results, ValidateValue(f, "owner", f.Owner, sysFile.Owner, skip))
	}
	if isSet(f.Uid) {
		results = append(results, ValidateValue(f, "uid", f.Uid, sysFile.Uid, skip))
	}
	if isSet(f.Group) {
		results = append(results, ValidateValue(f, "group", f.Group, sysFile.Group, skip))
	}
	if isSet(f.Gid) {
		results = append(results, ValidateValue(f, "gid", f.Gid, sysFile.Gid, skip))
	}
	if isSet(f.LinkedTo) {
		results = append(results, ValidateValue(f, "linkedto", f.LinkedTo, sysFile.LinkedTo, skip))
	}
	if isSet(f.Filetype) {
		results = append(results, ValidateValue(f, "filetype", f.Filetype, sysFile.Filetype, skip))
	}
	if isSetWarnEmpty(f.Contains, fmt.Sprintf("%s: file.contains", f.ID())) {
		fmt.Fprintf(os.Stderr, "DEPRECATION WARNING: file.contains has been renamed to file.contents\n")
		results = append(results, ValidateValue(f, "contains", f.Contains, sysFile.Contents, skip))
	}
	if isSetWarnEmpty(f.Contents, fmt.Sprintf("%s: file.contents", f.ID())) {
		results = append(results, ValidateValue(f, "contents", f.Contents, sysFile.Contents, skip))
	}
	if isSet(f.Size) {
		results = append(results, ValidateValue(f, "size", f.Size, sysFile.Size, skip))
	}
	if isSet(f.Md5) {
		results = append(results, ValidateValue(f, "md5", f.Md5, sysFile.Md5, skip))
	}
	if isSet(f.Sha256) {
		results = append(results, ValidateValue(f, "sha256", f.Sha256, sysFile.Sha256, skip))
	}
	if isSet(f.Sha512) {
		results = append(results, ValidateValue(f, "sha512", f.Sha512, sysFile.Sha512, skip))
	}
	return results
}

func NewFile(sysFile system.File, config util.Config) (*File, error) {
	path := sysFile.Path()
	exists, err := sysFile.Exists()
	if err != nil {
		return nil, err
	}
	// Contents is deliberately left unset. `[]` asserts nothing (isSet skips
	// an empty list), so emitting it only writes a line the reader has to
	// think about and dismiss.
	f := &File{
		id:     path,
		Exists: exists,
	}
	if !contains(config.IgnoreList, "mode") {
		if mode, err := sysFile.Mode(); err == nil {
			f.Mode = mode
		}
	}
	if !contains(config.IgnoreList, "owner") {
		if owner, err := sysFile.Owner(); err == nil {
			f.Owner = owner
		}
	}
	if !contains(config.IgnoreList, "group") {
		if group, err := sysFile.Group(); err == nil {
			f.Group = group
		}
	}
	if !contains(config.IgnoreList, "linked-to") {
		if linkedTo, err := sysFile.LinkedTo(); err == nil {
			f.LinkedTo = linkedTo
		}
	}
	if !contains(config.IgnoreList, "filetype") {
		if filetype, err := sysFile.Filetype(); err == nil {
			f.Filetype = filetype
		}
	}
	return f, nil
}

// fromSystem builds a fresh File from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewFile" off
// *system.System from a type parameter alone.
func (f *File) fromSystem(sys *system.System, key string, config util.Config) (system.File, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewFile(ctx, key, sys, config)
	n, err := NewFile(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*f = *n
	return sysRes, nil
}
