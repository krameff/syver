#!/usr/bin/env bash
#
# golden-baseline.sh -- capture, or verify against, the FEAT-007 golden baseline.
#
# A rolling regression gate: it records the CLI's observable output and exit
# codes, so any later change that alters them shows up as a diff instead of a
# surprise. It began life as the proof that FEAT-007 (registry-driven dispatch)
# was a verified no-op, and it still serves that purpose for any change claiming
# to be one -- but the baseline is NOT frozen at that point and no longer means
# "FEAT-007 changed nothing". It means "nothing changes from here on".
#
# So the baseline moves when a behaviour change is deliberately accepted. When
# you re-capture, say why in the commit message: an un-narrated re-capture makes
# a real regression indistinguishable from an intended change.
#
# Two ordering rules, both easy to get wrong:
#   * `capture` must run on a tree you have decided is CORRECT, not merely one
#     that builds. A baseline taken mid-change still diffs clean against itself
#     and proves nothing, silently.
#   * Re-capture only AFTER reviewing the diff the old baseline produces. That
#     diff is the evidence; overwriting it first throws the evidence away.
#
# Known limits, so nobody over-reads a PASS:
#   * The add/ goldens capture THIS HOST (its interfaces, its listening ports,
#     its shells), so they are a determinism check, not a portability one. This
#     does not run in CI, and would fail wholesale on a different machine.
#   * It is a manual gate -- nothing in the Makefile or .github/workflows runs
#     it -- so "204/204" is only as current as the last hand-run.
#
#   ./ci/golden-baseline.sh capture [outdir]   # run first, on the pre-refactor tree
#   ./ci/golden-baseline.sh verify  [outdir]   # run after; non-zero exit == behaviour changed
#
# Default outdir is $SYVER_GOLDEN_DIR, else ../.golden-baseline (outside the git
# repo on purpose -- it is ~24MB and must never be committed).
#
# What is covered, and why each one:
#   render/    every spec in the tree round-tripped unmarshal -> marshal. This is
#              the highest-value check: `syver add` writes YAML, and marshal field
#              order comes from SyverConfig's *struct declaration order*. Collapsing
#              those typed fields into a map would silently reorder every generated
#              gossfile. Empirically confirmed: `autoadd root` emits service/user/group,
#              which is struct field order (5,6,7), not alphabetical and not map order.
#   validate/  both fixtures x all 9 output formats, INCLUDING the exit code.
#              The exit code is the primary contract of a validation CLI -- it is
#              what CI steps and `dgoss run` actually consume -- and for a long
#              time this harness recorded a constant 0 for every case, so it was
#              blind to exit-code-only changes. See the capture loop below.
#              Catches result-shape and
#              formatter regressions, including the resource-type string that
#              validate.go derives (see the TypeName note in the FEAT-007 spec).
#   add/       one `syver add` per addable type + `autoadd`. Directly exercises the
#              per-type AppendSysResource path the descriptor table replaces.
#
# Durations are normalised to <DUR> -- same idea as sanitizeOutput in matcher_test.go.

set -euo pipefail

MODE="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_OUT="$(cd "$REPO_ROOT/.." && pwd)/.golden-baseline"
OUT="${2:-${SYVER_GOLDEN_DIR:-$DEFAULT_OUT}}"

case "$MODE" in
  capture|verify) ;;
  *) echo "usage: $0 {capture|verify} [outdir]" >&2; exit 2 ;;
esac

# verify captures into a sibling dir, then diffs the two trees
if [ "$MODE" = verify ]; then
  BASE="$OUT"
  OUT="${OUT}.actual"
  if [ ! -f "$BASE/MANIFEST.sha256" ]; then
    echo "FATAL: no baseline at $BASE -- run 'capture' on the pre-refactor tree first." >&2
    exit 1
  fi
fi

mkdir -p "$OUT"
BIN="$OUT/syver-bin"

cd "$REPO_ROOT"
go build -o "$BIN" ./cmd/syver

# Wipe the output trees before generating into them, and do it AFTER the build
# so a compile failure cannot destroy a baseline it was never going to replace.
#
# This is not tidiness. The harness generates a golden per spec it harvests and
# then lists what it finds on disk, so anything left over from a previous run is
# indistinguishable from a golden produced by this one. DELETING A FIXTURE WAS
# THEREFORE INVISIBLE TO THE GATE: the removed spec is no longer harvested, its
# stale golden is still on disk, `find` lists it, the checksum still matches the
# baseline, and verify reports PASS with the count unchanged. That happened on
# 2026-09-07, when dropping the bullseye fixtures produced a clean pass and the
# corrected count had to be worked out by hand.
#
# A gate that cannot see a deletion is worse than no gate, because it is
# believed. Keep this list in step with the `find` in the manifest step below.
for d in validate render add render-vars work; do
  rm -rf "${OUT:?}/$d"
done
rm -f "$OUT/MANIFEST.sha256"

git rev-parse HEAD > "$OUT/HEAD.txt" 2>/dev/null || echo "unknown" > "$OUT/HEAD.txt"

# Normalise everything wall-clock. The json/structured formatters embed absolute
# RFC3339 start-time/end-time and raw nanosecond durations, so stripping only the
# human-readable "1.2ms" spellings is not enough -- a self-verify against an
# unmodified tree still failed until these were added. Keep this list exhaustive;
# a sanitizer that under-matches produces false failures and destroys trust in
# the gate, which is worse than having no gate.
sanitize() {
  sed -E \
    -e 's/[0-9]+(\.[0-9]+)?(ms|µs|us|ns|s)\b/<DUR>/g' \
    -e 's/"(start-time|end-time)":"[^"]*"/"\1":"<TS>"/g' \
    -e 's/"(duration|total-duration)":[0-9]+/"\1":<DUR>/g' \
    -e 's/time="[^"]*"/time="<TS>"/g' \
    -e 's/timestamp="[^"]*"/timestamp="<TS>"/g' \
    -e 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?([+-][0-9]{2}:[0-9]{2}|Z)/<TS>/g' \
    -e 's/(duration_milliseconds\{[^}]*\}) [0-9]+/\1 <DUR>/g'
}

# ---- validate: both fixtures x every registered output format -----------------
mkdir -p "$OUT/validate"
for spec in passing failing; do
  for fmt in documentation json junit nagios prometheus rspecish structured tap silent; do
    f="$OUT/validate/${spec}.${fmt}"
    set +e
    "$BIN" -g "testdata/${spec}.goss.yaml" validate --no-color --format "$fmt" 2>&1 | sanitize > "$f"
    # MUST stay the very next command: PIPESTATUS is clobbered by the next
    # pipeline or simple command, so inserting anything above this line silently
    # reverts this harness to recording a constant again.
    #
    # The whole array is copied in one assignment rather than read element by
    # element, for the same reason: a `${PIPESTATUS[1]}` on the following line
    # would be reading the array left by this assignment, not by the pipeline.
    codes=("${PIPESTATUS[@]}")
    set -e
    # codes[0], not $?: $? is the exit of `sanitize` at the end of the pipe,
    # and the old `|| true` made it 0 unconditionally -- so every golden recorded
    # exit=0, including nagios which exits 2 on failure. The gate silently
    # covered none of the exit codes it appeared to.
    code=${codes[0]}
    # codes[1] is sanitize, which writes the golden. A sed that fails leaves the
    # file truncated or empty, and that damage is indistinguishable from a real
    # output change -- or, if it fails the same way during `capture`, from no
    # change at all. Neither is something to record. `set -e` cannot see this:
    # the pipeline ran under `set +e` above, and `pipefail` only sets $?, which
    # this loop no longer reads.
    if [ "${codes[1]}" -ne 0 ]; then
      echo "FATAL: sanitize failed (exit ${codes[1]}) while writing $f" >&2
      exit 1
    fi
    echo "exit=$code" >> "$f"
  done
done

# ---- render: every spec in the tree ------------------------------------------
mkdir -p "$OUT/render"
while IFS= read -r f; do
  safe="${f#./}"; safe="${safe//\//__}"
  if ! "$BIN" -g "$f" render > "$OUT/render/$safe.out" 2> "$OUT/render/$safe.err"; then
    mv "$OUT/render/$safe.out" "$OUT/render/$safe.out.failed"
  else
    rm -f "$OUT/render/$safe.err"
  fi
done < <(find ./testdata ./integration-tests ./examples ./docs \
           \( -name '*.yaml' -o -name '*.yml' \) 2>/dev/null \
         | grep -viE 'compose|expected|generated|\.pages' | sort)

# ---- render WITH --vars: the only case that exercises the template engine ----
# The loop above renders every harvested spec with NO vars and NO env. Every
# templated spec in the tree needs one or the other, so all of them die at
# variable lookup ("<.Env.OS>: map has no entry for key \"OS\"") and their
# goldens record that error string rather than any rendered output. The string
# is identical either side of a template-engine change, so those goldens are
# blind to the whole template layer.
#
# Measured 2026-09-01 during the sprig -> sprout migration: 8 templated
# fixtures, 8 zero-byte .out.failed files, and a green 205/205 that proved
# nothing whatever about the swap. This case renders one spec WITH its vars so
# the golden holds real output, and a change in the engine shows up as a diff.
#
# ci/fixtures/templated.vars.yaml uses a double-space value on purpose: it is the
# input where sprig and sprout disagree. Do not "tidy" it to a single space.
mkdir -p "$OUT/render-vars"
if ! "$BIN" -g testdata/templated.goss.yaml --vars ci/fixtures/templated.vars.yaml render \
       > "$OUT/render-vars/templated.out" 2> "$OUT/render-vars/templated.err"; then
  mv "$OUT/render-vars/templated.out" "$OUT/render-vars/templated.out.failed"
else
  rm -f "$OUT/render-vars/templated.err"
fi

# ---- add / autoadd: the marshal path -----------------------------------------
mkdir -p "$OUT/add" "$OUT/work"
_add() {
  local t="$1" k="$2" f="$OUT/work/$1.yaml"
  rm -f "$f"
  "$BIN" -g "$f" add "$t" "$k" > "$OUT/add/$t.stdout" 2> "$OUT/add/$t.stderr" || true
  cp "$f" "$OUT/add/$t.yaml" 2>/dev/null || echo "(no file written)" > "$OUT/add/$t.yaml"
}
_add file /etc/hosts
_add command "echo hi"
_add group root
_add user root
_add process systemd
_add port tcp:22
_add kernel-param kernel.ostype
_add mount /
_add interface lo
_add addr tcp://127.0.0.1:22
_add dns localhost
rm -f "$OUT/work/aa.yaml"
"$BIN" -g "$OUT/work/aa.yaml" autoadd root > "$OUT/add/autoadd.stdout" 2> "$OUT/add/autoadd.stderr" || true
cp "$OUT/work/aa.yaml" "$OUT/add/autoadd.yaml" 2>/dev/null || true

# ---- normalise -------------------------------------------------------------
# Two things leak the capture location and the clock into the goldens, and both
# are artefacts of this harness rather than of syver:
#   * `syver add` echoes the absolute path it wrote to, which differs between the
#     baseline dir and the .actual dir used by verify;
#   * render's stderr carries Go's "2006/01/02 15:04:05" log prefix.
# Normalise the whole tree, including the .err files that never went through
# sanitize() above.
# ORDER IS LOAD-BEARING: BASE is a strict prefix of OUT (.golden-baseline vs
# .golden-baseline.actual), so OUT must be substituted first to consume the
# .actual paths whole. Reverse these two and every .actual path normalises to
# "<OUT>.actual", making every verify fail with a diff that looks like a real
# behaviour change. Do not sort these -e expressions.
find "$OUT/validate" "$OUT/render" "$OUT/add" -type f -print0 | xargs -0 sed -i -E \
  -e "s#${OUT}#<OUT>#g" \
  -e "s#${BASE:-__nomatch__}#<OUT>#g" \
  -e 's#[0-9]{4}/[0-9]{2}/[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}#<TS>#g'

# ---- manifest ----------------------------------------------------------------
# render-vars is in this list deliberately. It was added 2026-09-01 and omitting
# it would have made the whole point of that case moot: the file is written on
# every run, but a golden outside the manifest is never compared to anything, so
# it would have sat there looking like coverage while proving nothing. If you add
# another output directory, add it here in the same commit or it is decoration.
# `work/` is deliberately absent -- it is the scratch tree `add` writes into.
( cd "$OUT" && find validate render add render-vars -type f | sort | xargs sha256sum > MANIFEST.sha256 )
count=$(wc -l < "$OUT/MANIFEST.sha256")

if [ "$MODE" = capture ]; then
  echo "captured $count golden files -> $OUT"
  echo "HEAD: $(cat "$OUT/HEAD.txt")"
  exit 0
fi

# ---- verify ------------------------------------------------------------------
base_head="$(cat "$BASE/HEAD.txt")"
this_head="$(cat "$OUT/HEAD.txt")"
echo "baseline HEAD: $base_head"
echo "current  HEAD: $this_head"

# Say it, rather than printing two shas and leaving the reader to compare them.
# The baseline lives OUTSIDE the repository, so it does not switch with the
# branch: capture on one branch, check out another, and every golden that
# differs between the two branches is reported as a failure. Nothing is wrong.
# Diagnosed 2026-09-04 in a minute, but only because the cause happened to be
# fresh; the failure output on its own points at the code, not at the baseline.
if [ "$base_head" != "$this_head" ]; then
  echo
  echo "NOTE: the baseline was captured on a DIFFERENT commit to the one being" >&2
  echo "      verified. If files differ below, check whether they differ between" >&2
  echo "      those two commits before reading it as a regression. Do NOT" >&2
  echo "      re-capture to make it pass: that adopts the other tree's output as" >&2
  echo "      the baseline and destroys the comparison you wanted." >&2
  echo
fi
if diff -u "$BASE/MANIFEST.sha256" "$OUT/MANIFEST.sha256" > "$OUT/manifest.diff"; then
  echo "PASS: all $count golden files byte-identical -- no observable change."
  exit 0
fi
echo "FAIL: golden output changed. Differing files:" >&2
awk '/^[-+][^-+]/ {print $2}' "$OUT/manifest.diff" | sort -u | while read -r p; do
  [ -n "$p" ] || continue
  echo "  --- $p" >&2
  diff -u "$BASE/$p" "$OUT/$p" 2>/dev/null | head -30 >&2 || true
done
exit 1
