#!/usr/bin/env bash
# End-to-end test for the compose wrappers (dcsyver, and dcgoss as its shim).
#
# Nothing else in the suite covers extras/dcsyver at all: the docker matrix
# exercises the bare binary in a container, and the discovery/depends-on e2e
# scripts exercise it on the host. The compose path had no automated coverage
# before this script, which is how the stdin defect noted at the bottom of this
# file survived unnoticed.
#
# Requires a working `docker compose` (or legacy `docker-compose`). Skips
# cleanly when no compose provider is present, so it is safe to wire into
# targets that run on machines without one.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURE="${ROOT}/integration-tests/dcsyver"
SERVICE="syver-target"

: "${CONTAINER_RUNTIME:=docker}"

log() { echo "dcsyver-e2e: $*" >&2; }

if ! command -v "${CONTAINER_RUNTIME}" >/dev/null 2>&1; then
  log "SKIP: no ${CONTAINER_RUNTIME} on PATH"
  exit 0
fi
if ! "${CONTAINER_RUNTIME}" compose version >/dev/null 2>&1 \
  && ! command -v docker-compose >/dev/null 2>&1; then
  log "SKIP: no compose provider (neither '${CONTAINER_RUNTIME} compose' nor 'docker-compose')"
  exit 0
fi

# Build the binary the same way the release does. A bare `go build` leaves the
# version string empty, which changes the outbound User-Agent and has already
# produced one false failure in this suite.
BIN="${ROOT}/release/syver-linux-amd64"
if [[ ! -x "${BIN}" ]]; then
  log "building ${BIN}"
  make -C "${ROOT}" release/syver-linux-amd64
fi

# dcsyver resolves its own binary via SYVER_PATH, falling back to PATH lookup.
export SYVER_PATH="${BIN}"

cleanup() {
  local ret=$?
  # dcsyver's own cleanup issues `compose stop`, which stops but does not
  # remove. Reap explicitly so a failing run cannot leave a container behind on
  # a shared test host.
  ( cd "${FIXTURE}" && "${CONTAINER_RUNTIME}" compose down -v --remove-orphans >/dev/null 2>&1 ) || true
  exit "${ret}"
}
trap cleanup EXIT

log "running dcsyver against ${FIXTURE}/compose.yaml (service: ${SERVICE})"
cd "${FIXTURE}"

out="$("${ROOT}/extras/dcsyver/dcsyver" run "${SERVICE}" 2>&1)" && rc=0 || rc=$?
echo "${out}"

if [[ "${rc}" -ne 0 ]]; then
  log "FAIL: dcsyver exited ${rc}"
  # This specific error meant the stdin double-read bug, fixed
  # 2026-08-17. If it reappears, that fix has regressed -- check readStdinOnce()
  # in store.go before looking anywhere else.
  if grep -q "found 0 tests, source: STDIN" <<<"${out}"; then
    log "cause: REGRESSION of the stdin double-read fix. dcsyver's test"
    log "       path is 'render | validate -g -'; reading a spec from a pipe is"
    log "       returning zero tests again. See readStdinOnce() in store.go."
  fi
  exit "${rc}"
fi

# Assert on outcome, not on an exact assertion count. An earlier version of this
# script hardcoded "Count: 2" while the fixture defined 4, so editing the fixture
# broke the script in a way that looked like a product failure. What actually
# matters is that the spec was found, ran inside the container, and everything
# passed -- a zero Count is the real failure mode, since that is what a spec that
# never loaded looks like.
if ! grep -qE "Count: [1-9][0-9]*, Failed: 0, Skipped: 0" <<<"${out}"; then
  log "FAIL: expected a non-zero assertion count with no failures or skips, got:"
  grep -E "^Count:" <<<"${out}" >&2 || log "  (no Count line at all -- the spec never ran)"
  exit 1
fi

log "ok"

# HISTORY
#
# This script was written while `syver -g - validate` was broken: reading a spec
# from a pipe returned zero tests, so dcsyver -- whose only test path is
# `render | validate -g -` -- could not execute any test at all. That defect
# predated the rename (the v0.6.0 seed binary failed identically) and was fixed
# on 2026-08-17: `loadSyverConfigWithDiscover` decodes each spec twice
# (a peek pass, then the real load), and `os.Stdin` is not re-readable, so the
# second decode saw nothing. `readStdinOnce()` in store.go now buffers it.
#
# This script is the regression test for that fix, verified passing end-to-end
# against a real Docker Compose v2 provider.
#
# It is deliberately NOT in `make check` or `make pre-push`, because it needs a
# compose provider that not every dev machine has. It skips cleanly without one --
# note that a skip exits 0 and is NOT a pass.
