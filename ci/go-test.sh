#!/usr/bin/env bash
set -euo pipefail

command -v go

# -count=1 disables the test result cache. Without it a run whose source did
# not change replays a previous PASS, which has already hidden a genuinely
# broken test in this repo. A gate that can report a stale green is worse
# than no gate.
# -race is affordable here (the suite runs in seconds) and this repo has already
# paid for its absence: a write-write race on fatih/color's NoColor global, hit
# by every concurrent `serve` response rendering JSON or JUnit, survived the
# project's entire history undetected because nothing ever ran the detector.
# The suite PASSES under it as of the serve_test.go parallelism fix. That is not
# the same as the code being race-free, and the difference has bitten twice.
# -race is purely dynamic: it reports only on memory an executing test actually
# touches concurrently. A data race in the command timeout path
# (system/command.go reading cmd.Stdout while the child's goroutine still wrote
# to it) sat in v0.8.0 and v0.9.0 under a green run of this gate, simply because
# no test had ever timed a command out. The gap was coverage, not the gate.
# Read a green result as "the paths these tests exercise are clean", never as
# "the code is clean".
go test -count=1 -race -coverpkg=./... ./... -coverprofile="c.out"

sed 's|github.com/krameff/syver/||' <"c.out" >"c.out.tmp"

mv "c.out.tmp" "c.out"
