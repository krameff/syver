#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

TRIVY_SEVERITY="${TRIVY_SEVERITY:-HIGH,CRITICAL,MEDIUM}"
TRIVY_SKIP_DIRS="${TRIVY_SKIP_DIRS:-integration-tests,release,site,.venv,.git}"

run_govulncheck() {
  echo "==> govulncheck"
  if command -v govulncheck >/dev/null 2>&1; then
    govulncheck ./...
  else
    go run golang.org/x/vuln/cmd/govulncheck@latest ./...
  fi
}

run_trivy() {
  trivy fs --scanners vuln --severity "${TRIVY_SEVERITY}" \
    --skip-dirs "${TRIVY_SKIP_DIRS}" \
    .
}

# Runs trivy through a container runtime. Accepts docker or podman: this
# project's own dev sandbox is podman-only, so a docker-only fallback meant the
# scan silently skipped on the very machine most of the development happens on.
run_trivy_container() {
  local runtime="$1"
  # :z relabels the bind mount for SELinux, which podman on RHEL-family hosts
  # needs and docker ignores harmlessly.
  "${runtime}" run --rm \
    -v "${ROOT}:/src:z" \
    -w /src \
    docker.io/aquasec/trivy:latest \
    fs --scanners vuln --severity "${TRIVY_SEVERITY}" \
    --skip-dirs "${TRIVY_SKIP_DIRS}" \
    .
}

run_trivy_fs() {
  echo "==> trivy fs (go.mod, docs/requirements.txt)"

  if command -v trivy >/dev/null 2>&1; then
    run_trivy
    return
  fi

  local runtime
  for runtime in docker podman; do
    if command -v "${runtime}" >/dev/null 2>&1; then
      echo "    (via ${runtime}, no local trivy binary)"
      run_trivy_container "${runtime}"
      return
    fi
  done

  if [[ "${SECURITY_STRICT:-}" == "1" ]]; then
    echo "ERROR: trivy not found and no container runtime available" >&2
    exit 1
  fi

  # Record that this run did NOT scan, so the summary cannot be mistaken for a
  # clean result. `make check`/`pre-push` include this script, and a silent skip
  # reads exactly like a pass.
  trivy_skipped=1
  echo "WARN: skipping trivy fs scan -- no trivy binary and no docker/podman" >&2
  echo "WARN: set SECURITY_STRICT=1 to make this a hard failure instead" >&2
}

trivy_skipped=0
run_govulncheck
run_trivy_fs

if [[ "${trivy_skipped}" == "1" ]]; then
  echo
  echo "==> SUMMARY: govulncheck ran; trivy DID NOT RUN (skipped, see WARN above)"
  echo "    This run is NOT a clean security result."
else
  echo
  echo "==> SUMMARY: govulncheck and trivy both ran"
fi
