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
git rev-parse HEAD > "$OUT/HEAD.txt" 2>/dev/null || echo "unknown" > "$OUT/HEAD.txt"
go build -o "$BIN" ./cmd/syver

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
    code=${PIPESTATUS[0]}
    set -e
    # PIPESTATUS[0], not $?: $? is the exit of `sanitize` at the end of the pipe,
    # and the old `|| true` made it 0 unconditionally -- so every golden recorded
    # exit=0, including nagios which exits 2 on failure. The gate silently
    # covered none of the exit codes it appeared to.
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
( cd "$OUT" && find validate render add -type f | sort | xargs sha256sum > MANIFEST.sha256 )
count=$(wc -l < "$OUT/MANIFEST.sha256")

if [ "$MODE" = capture ]; then
  echo "captured $count golden files -> $OUT"
  echo "HEAD: $(cat "$OUT/HEAD.txt")"
  exit 0
fi

# ---- verify ------------------------------------------------------------------
echo "baseline HEAD: $(cat "$BASE/HEAD.txt")"
echo "current  HEAD: $(cat "$OUT/HEAD.txt")"
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
