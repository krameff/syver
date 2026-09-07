//go:build windows

package system

import (
	"errors"

	"golang.org/x/sys/windows"
)

// lookupAbsenceIsPlatformSpecific reports whether err is Windows' own way of
// saying "that account name does not resolve".
//
// WHY THIS FILE EXISTS. os/user's UnknownUserError / UnknownGroupError are the
// documented "looked it up, it is not there" signal, and on Unix that is what
// Lookup returns. On WINDOWS it is not: the lookup goes through
// LookupAccountName, and a name that does not resolve comes back as the raw
// ERROR_NONE_MAPPED ("No mapping between account names and security IDs was
// done"), which is not wrapped into UnknownUserError. So errors.As misses it.
//
// The consequence, which FEAT-010 shipped until the Windows container run on
// 2026-09-02 caught it: a genuinely absent user was classified as "the lookup
// could not run", so `user: absent-account: {exists: false}` FAILED. That is a
// false failure -- the exact mirror of the false pass FEAT-010 exists to
// remove, and invisible to a Linux-only gate because on Linux the
// UnknownUserError path is correct and the tests pass.
//
// MATCH THE ERRNO, NOT THE MESSAGE. "No mapping between account names and
// security IDs was done." is localised by Windows, so a string comparison is
// wrong on any non-English host -- the same trap D-7 records for the service
// probe, which is why that one uses a structural check too.
func lookupAbsenceIsPlatformSpecific(err error) bool {
	return errors.Is(err, windows.ERROR_NONE_MAPPED)
}
