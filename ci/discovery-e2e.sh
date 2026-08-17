#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXAMPLES="${ROOT}/integration-tests/goss/examples/discovery"
GOSS_ARGS=()

# shellcheck source=lib/syver-e2e-steps.sh
source "${ROOT}/ci/lib/syver-e2e-steps.sh"

if [[ -n "${GOSS_BINARY:-}" ]]; then
  GOSS="${GOSS_BINARY}"
elif [[ "$(uname -s)" == "Linux" ]]; then
  GOSS="${ROOT}/release/syver-linux-amd64"
  if [[ ! -x "${GOSS}" ]]; then
    make -C "${ROOT}" release/syver-linux-amd64
  fi
else
  GOSS="$(mktemp -t goss-discovery-e2e.XXXXXX)"
  # Build the package, not the single file: syver.go alone no longer compiles
  # because env_source.go (nonEmptyEnvVars) is a sibling in the same package.
  go build -o "${GOSS}" "${ROOT}/cmd/syver"
  export GOSS_USE_ALPHA=1
  GOSS_ARGS=(--use-alpha=1)
fi

cleanup() {
  if [[ "${GOSS:-}" == /tmp/goss-discovery-e2e.* ]] || [[ "${GOSS:-}" == *"/T/goss-discovery-e2e."* ]]; then
    rm -f "${GOSS}"
  fi
}
trap cleanup EXIT

if [[ ! -x "${GOSS}" ]]; then
  echo "goss binary not found or not executable: ${GOSS}" >&2
  exit 1
fi

goss_runner() {
  "${GOSS}" "${GOSS_ARGS[@]}" "$@"
}

run_discovery_e2e_steps "${EXAMPLES}" goss_runner

echo "discovery e2e: ok"
