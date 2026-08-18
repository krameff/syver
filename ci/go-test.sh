#!/usr/bin/env bash
set -euo pipefail

command -v go

# -count=1 disables the test result cache. Without it a run whose source did
# not change replays a previous PASS, which has already hidden a genuinely
# broken test in this repo. A gate that can report a stale green is worse
# than no gate.
go test -count=1 -coverpkg=./... ./... -coverprofile="c.out"

sed 's|github.com/krameff/syver/||' <"c.out" >"c.out.tmp"

mv "c.out.tmp" "c.out"
