#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXAMPLES="${ROOT}/integration-tests/goss/examples/depends-on"
SYVER_ARGS=()

# shellcheck source=lib/syver-e2e-steps.sh
source "${ROOT}/ci/lib/syver-e2e-steps.sh"

if [[ -n "${SYVER_BINARY:-}" ]]; then
  SYVER="${SYVER_BINARY}"
elif [[ "$(uname -s)" == "Linux" ]]; then
  SYVER="${ROOT}/release/syver-linux-amd64"
  if [[ ! -x "${SYVER}" ]]; then
    make -C "${ROOT}" release/syver-linux-amd64
  fi
else
  SYVER="$(mktemp -t syver-depends-on-e2e.XXXXXX)"
  # Build the package, not the single file: syver.go alone no longer compiles
  # because env_source.go (nonEmptyEnvVars) is a sibling in the same package.
  go build -o "${SYVER}" "${ROOT}/cmd/syver"
  export GOSS_USE_ALPHA=1
  SYVER_ARGS=(--use-alpha=1)
fi

cleanup() {
  if [[ "${SYVER:-}" == /tmp/syver-depends-on-e2e.* ]] || [[ "${SYVER:-}" == *"/T/syver-depends-on-e2e."* ]]; then
    rm -f "${SYVER}"
  fi
}
trap cleanup EXIT

if [[ ! -x "${SYVER}" ]]; then
  echo "syver binary not found or not executable: ${SYVER}" >&2
  exit 1
fi

syver_runner() {
  "${SYVER}" "${SYVER_ARGS[@]}" "$@"
}

run_depends_on_e2e_steps "${EXAMPLES}" syver_runner

echo "depends-on e2e: ok"
