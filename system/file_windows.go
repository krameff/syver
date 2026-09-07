//go:build windows
// +build windows

package system

// Mode, Owner, Uid, Group and Gid all previously returned a fabricated
// placeholder ("-1" / -1) with a nil error. NewFile's `if err == nil` guard
// (resource/file.go) then wrote that placeholder into the emitted spec, so
// `syver add` on Windows generated assertions the user never made, on a host
// syver had learned nothing about -- exactly the "silent under-testing"
// resource/file.go's own NewFile comment calls worse than a hard failure.
//
// Each accessor now returns ErrFileOwnershipUnsupported instead. See
// resource/file.go's NewFile: it distinguishes this from a lookup that RAN
// and found nothing (lookupDidNotRun), so on Windows `syver add file` still
// omits mode/owner/group/uid/gid and exits 0 -- it does not abort. Only
// `syver validate`/`serve`, which go through resource/validate.go's
// err != nil -> FAIL path, turn these into a failure. That is deliberate:
// see FEAT-010 D-4.
func (f *DefFile) Mode() (string, error) {
	return "", ErrFileOwnershipUnsupported
}

func (f *DefFile) Owner() (string, error) {
	return "", ErrFileOwnershipUnsupported
}

func (f *DefFile) Uid() (int, error) {
	return 0, ErrFileOwnershipUnsupported
}

func (f *DefFile) Group() (string, error) {
	return "", ErrFileOwnershipUnsupported
}

func (f *DefFile) Gid() (int, error) {
	return 0, ErrFileOwnershipUnsupported
}
