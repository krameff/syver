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
// Each accessor returned ErrFileOwnershipUnsupported instead. See
// resource/file.go's NewFile: it distinguishes this from a lookup that RAN
// and found nothing (lookupDidNotRun), so on Windows `syver add file` still
// omits the unsupported attributes and exits 0 -- it does not abort. Only
// `syver validate`/`serve`, which go through resource/validate.go's
// err != nil -> FAIL path, turn these into a failure. That is deliberate:
// see FEAT-010 D-4.
//
// FEAT-016 then replaced the honest error with a real answer for TWO of the
// five. Owner and Group now read the security descriptor and live in
// file_acl_windows.go, alongside the new Acl and AclSid. The three below stay
// sentinels, and two of them PERMANENTLY:
//
//   - Uid and Gid are a category mismatch, not missing work. Windows identifies
//     accounts by SID; syver parses these attributes as integers. There is no
//     value an implementation could return. Use owner: or acl:.
//   - Mode stays unsupported by decision, not by difficulty. A POSIX mode could
//     be derived from an ACL only lossily, and a plausible-looking 0644 computed
//     from a DACL is exactly the confident wrong answer FEAT-010 removed: a
//     cross-platform gossfile would then pass on Windows for the wrong reason.
//     acl: is the Windows answer to the question mode: asks.
func (f *DefFile) Mode() (string, error) {
	return "", ErrFileOwnershipUnsupported
}

func (f *DefFile) Uid() (int, error) {
	return 0, ErrFileOwnershipUnsupported
}

func (f *DefFile) Gid() (int, error) {
	return 0, ErrFileOwnershipUnsupported
}
