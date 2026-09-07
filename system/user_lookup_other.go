//go:build !windows

package system

// lookupAbsenceIsPlatformSpecific is the non-Windows half of the pair. On Unix,
// os/user returns UnknownUserError / UnknownGroupError for an account that does
// not exist, which the shared classifiers already handle, so there is nothing
// platform-specific to add here. See user_lookup_windows.go for why Windows
// needs its own answer.
func lookupAbsenceIsPlatformSpecific(err error) bool {
	return false
}
