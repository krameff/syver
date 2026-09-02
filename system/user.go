package system

import (
	"context"
	"errors"
	"os/user"
	"strconv"

	"github.com/krameff/syver/util"
)

type User interface {
	Username() string
	Exists() (bool, error)
	UID() (int, error)
	GID() (int, error)
	Groups() ([]string, error)
	Home() (string, error)
	Shell() (string, error)
}

type DefUser struct {
	username string
}

func NewDefUser(_ context.Context, username string, system *System, config util.Config) User {
	return &DefUser{username: username}
}

func (u *DefUser) Username() string {
	return u.username
}

// Exists distinguishes "user.Lookup ran and genuinely found no such user"
// (userLookupFoundNothing -- false, nil, unchanged) from every other failure
// (false, err). The distinction matters most on a domain-joined host with
// an unreachable domain controller: previously EVERY error, including a
// transport failure that learned nothing, was folded into "does not exist".
// See FEAT-010 SW-9 / Trap 1.
func (u *DefUser) Exists() (bool, error) {
	_, err := user.Lookup(u.username)
	if err == nil {
		return true, nil
	}
	if userLookupFoundNothing(err) {
		return false, nil
	}
	return false, err
}

// userLookupFoundNothing reports whether err is user.UnknownUserError --
// the one error os/user's own doc comment guarantees means "the user was
// looked up and does not exist", as opposed to a lookup that could not
// complete at all. Split out as a pure function so it is unit-testable
// against a synthetic error, without needing an actual unreachable domain
// controller to provoke the "could not run" case from user.Lookup itself.
func userLookupFoundNothing(err error) bool {
	var unknown user.UnknownUserError
	return errors.As(err, &unknown)
}

func (u *DefUser) UID() (int, error) {
	user, err := user.Lookup(u.username)
	if err != nil {
		return 0, err
	}

	uid, err := strconv.Atoi(user.Uid)
	if err != nil {
		return 0, err
	}

	return uid, nil
}

func (u *DefUser) GID() (int, error) {
	user, err := user.Lookup(u.username)
	if err != nil {
		return 0, err
	}

	gid, err := strconv.Atoi(user.Gid)
	if err != nil {
		return 0, err
	}

	return gid, nil
}

func (u *DefUser) Home() (string, error) {
	user, err := user.Lookup(u.username)
	if err != nil {
		return "", err
	}

	return user.HomeDir, nil
}
