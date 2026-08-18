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
# The suite is clean under it as of the serve_test.go parallelism fix -- the
# remaining package-level config globals are only ever driven concurrently by
# tests, never by the CLI.
go test -count=1 -race -coverpkg=./... ./... -coverprofile="c.out"

sed 's|github.com/krameff/syver/||' <"c.out" >"c.out.tmp"

mv "c.out.tmp" "c.out"
