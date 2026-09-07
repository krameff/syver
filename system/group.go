package system

import (
	"context"
	"errors"
	"os/user"
	"strconv"

	"github.com/krameff/syver/util"
)

type Group interface {
	Groupname() string
	Exists() (bool, error)
	GID() (int, error)
}

type DefGroup struct {
	groupname string
}

func NewDefGroup(_ context.Context, groupname string, system *System, config util.Config) Group {
	return &DefGroup{groupname: groupname}
}

func (u *DefGroup) Groupname() string {
	return u.groupname
}

// Exists distinguishes "user.LookupGroup ran and genuinely found no such
// group" (groupLookupFoundNothing -- false, nil, unchanged) from every other
// failure (false, err), for the same reason as DefUser.Exists. See FEAT-010
// SW-9 / Trap 1.
func (u *DefGroup) Exists() (bool, error) {
	_, err := user.LookupGroup(u.groupname)
	if err == nil {
		return true, nil
	}
	if groupLookupFoundNothing(err) {
		return false, nil
	}
	return false, err
}

// groupLookupFoundNothing is userLookupFoundNothing's counterpart for
// user.UnknownGroupError. Kept as a pure function for the same testability
// reason.
func groupLookupFoundNothing(err error) bool {
	var unknown user.UnknownGroupError
	if errors.As(err, &unknown) {
		return true
	}
	// Windows does not wrap a non-resolving account name into
	// user.UnknownGroupError -- see user_lookup_windows.go.
	return lookupAbsenceIsPlatformSpecific(err)
}

func (u *DefGroup) GID() (int, error) {
	group, err := user.LookupGroup(u.groupname)
	if err != nil {
		return 0, err
	}

	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, err
	}

	return gid, nil
}
