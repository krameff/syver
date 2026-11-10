#!/usr/bin/env bash
# Run two syver binaries against the same spec and report every check whose
# result differs: pass, fail or skip.
#
# Usage: ci/compare-binaries.sh <old-binary> <new-binary> [syver global flags...]
#   e.g. ci/compare-binaries.sh ./syver-0.16.1 ./release/syver-linux-amd64 \
#          -g audit/goss.yml --vars audit/vars.yml
#
# The flags are passed to both binaries ahead of `validate --format json`. Run it
# on one host so both binaries see the same system state; a check that depends
# on timing or on something changing between the runs can still differ.
#
# Checks are matched by resource type, resource id and property. A check
# present in only one run is reported as missing on the other side.
#
# Exit: 0 when every check has the same result, 1 when any differs, 2 when a
# binary produced no usable JSON or the arguments are wrong.
set -euo pipefail

if [ "$#" -lt 2 ]; then
  sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
fi
OLD="$1"; NEW="$2"; shift 2

command -v jq >/dev/null 2>&1 || { echo "compare-binaries: jq is required" >&2; exit 2; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# One line per check, "type|id|property#n<TAB>status", sorted; n numbers
# repeats of the same key so join pairs them one to one. A non-zero exit is
# normal (failing checks), so only the JSON decides whether the run was usable.
results() {
  local bin="$1" out="$2"
  set +e
  "$bin" "${@:3}" validate --format json --no-color >"$out.json" 2>"$out.err"
  set -e
  if ! jq -e '.results | type == "array"' "$out.json" >/dev/null 2>&1; then
    echo "compare-binaries: $bin produced no results JSON; its output:" >&2
    cat "$out.json" "$out.err" | head -20 | sed 's/^/  /' >&2
    exit 2
  fi
  jq -r '.results[]
    | [(.["resource-type"] + "|" + .["resource-id"] + "|" + .property),
       (if .skipped then "skip" elif .successful then "pass" else "fail" end)]
    | @tsv' "$out.json" | LC_ALL=C sort \
    | awk -F '\t' '{ n[$1]++; printf "%s#%d\t%s\n", $1, n[$1], $2 }' >"$out.tsv"
}

results "$OLD" "$TMP/old" "$@"
results "$NEW" "$TMP/new" "$@"

LC_ALL=C join -t $'\t' -a 1 -a 2 -e missing -o 0,1.2,2.2 \
  "$TMP/old.tsv" "$TMP/new.tsv" >"$TMP/joined"
awk -F '\t' '$2 != $3 { k = $1; sub(/#1$/, "", k); printf "%s: %s -> %s\n", k, $2, $3 }' "$TMP/joined" >"$TMP/diff"

# Arithmetic expansion drops the leading spaces BSD wc pads its count with.
total=$(( $(wc -l <"$TMP/joined") ))
changed=$(( $(wc -l <"$TMP/diff") ))
cat "$TMP/diff"
echo "compared $total checks: $changed changed"
[ "$changed" -eq 0 ]
