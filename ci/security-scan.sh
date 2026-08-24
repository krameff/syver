#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# UNKNOWN is included deliberately. Trivy reports Go vulndb GO-IDs with no CVSS
# score as UNKNOWN, and GO-2026-5932 -- the sole entry in this repo's
# .trivyignore.yaml -- is exactly that. Without UNKNOWN the gate never looked at
# the severity class its own suppression file governs, so the suppressions and the
# validator that guards it were protecting a door the gate did not use.
TRIVY_SEVERITY="${TRIVY_SEVERITY:-HIGH,CRITICAL,MEDIUM,UNKNOWN}"
TRIVY_SKIP_DIRS="${TRIVY_SKIP_DIRS:-integration-tests,release,site,.venv,.git}"
# Passed explicitly because trivy does NOT auto-detect the YAML variant:
# --ignorefile defaults literally to ".trivyignore". Verified -- with only
# .trivyignore.yaml present and no flag, the suppression does not apply. If this
# flag is dropped the gate goes red rather than silently unsuppressing, which is
# the safe direction, but it will look like a regression rather than a config
# slip. Keep in step with ci/trivyignore-check.sh.
#
# Must be REPO-RELATIVE. The container path bind-mounts only the repo at /src,
# so an absolute path outside it does not exist inside the container and trivy
# exits 1 ("FAILED TO RUN") rather than scanning unsuppressed.
SYVER_TRIVY_IGNOREFILE="${SYVER_TRIVY_IGNOREFILE:-.trivyignore.yaml}"
# trivy's own default is [vuln,secret]. Naming just "vuln" is not a no-op, it
# NARROWS -- it turned the secret scanner off on what is now a blocking gate.
# Verified against `trivy fs --help` on 0.74.0.
SYVER_TRIVY_SCANNERS="${SYVER_TRIVY_SCANNERS:-vuln,secret,misconfig}"
# Findings must FAIL the run, not just print. Without --exit-code trivy exits 0
# with HIGH CVEs on screen, so `make check` and `make pre-push` went green while
# the summary below said "both ran" -- and .trivyignore suppressed entries in a
# report nothing acted on, which made the whole trivyignore-check.sh governance
# decorative.
#
# The value is 2, not 1, and the name is SYVER_-prefixed. Both matter:
#   * trivy exits 1 for its OWN fatal errors (DB unreachable, bad target --
#     verified). At 1 we cannot tell "found vulnerabilities" from "never ran",
#     and would confidently tell the operator to add a .trivyignore entry for a
#     scan that produced nothing. 2 is trivy's documented convention for this.
#   * trivy reads TRIVY_<FLAG> from the environment, so a variable literally
#     named TRIVY_EXIT_CODE is consumed by trivy itself -- verified: exporting
#     TRIVY_EXIT_CODE=7 makes an unrelated trivy call exit 7. Prefixing keeps
#     our knob out of trivy's namespace.
# Set to 0 to inspect findings without failing; the summary says so explicitly.
SYVER_TRIVY_EXIT_CODE="${SYVER_TRIVY_EXIT_CODE:-2}"
# The DB is ~700MB and is re-pulled into a throwaway layer on every containerised
# run without this. Honours XDG_CACHE_HOME so it can be moved off a full volume.
SYVER_TRIVY_CACHE_DIR="${SYVER_TRIVY_CACHE_DIR:-${XDG_CACHE_HOME:-${HOME:-/tmp}/.cache}/trivy}"
# Pinned, not :latest, because --exit-code above makes findings blocking and a
# floating scanner can turn a build red with no repo change.
# NOTE: this pins the CONTAINER path only. CI installs a trivy binary via
# aquasecurity/setup-trivy and therefore takes the local-binary path below, so
# CI's version is pinned by that workflow's `version:` input, not by this.
# Digest, not tag: a version tag is still re-pointable, so it floats slowly
# rather than not at all. This is the manifest LIST digest for 0.74.0, so
# multi-arch is preserved -- verified against `podman manifest inspect`, which
# shows sha256:ee940acb... as the amd64 platform manifest. Do NOT pin to that
# one, it breaks arm64. Bump tag and digest together.
SYVER_TRIVY_IMAGE="${SYVER_TRIVY_IMAGE:-docker.io/aquasec/trivy@sha256:62b1e65e8869bc4b4c6aa4fa2b21595256c7c2f6018a9d9ad61caf87187c1969}"  # 0.74.0

# Pinned for the same reason as the trivy image. CI installs trivy via
# setup-trivy but never installs govulncheck, so it falls through to this
# `go run` -- which at @latest fetches and executes whatever shipped that day,
# inside the security gate itself. Bump deliberately.
SYVER_GOVULNCHECK_VERSION="${SYVER_GOVULNCHECK_VERSION:-v1.7.0}"
# govulncheck distinguishes findings from failure by exit code, and collapsing
# the two reports a proven reachable CVE as "the scanner broke" -- the same
# mistake the trivy half of this script exists to avoid. From the tool's own
# source (x/vuln@v1.7.0 internal/scan/errors.go:17-26):
#     errVulnerabilitiesFound -> code 3
#     errUsage                -> code 2
# and main.go:42 falls back to 1 for everything else. The published prose only
# says "exits unsuccessfully", which is why this is pinned to the source.
readonly GOVULNCHECK_VULNS_FOUND=3

# Validated before use: these feed `(( ))`, where bash treats a non-numeric
# string as a variable name and `set -u` then aborts -- with no summary line at
# all, which is precisely what this script promises never to do.
if [[ ! "${SYVER_TRIVY_EXIT_CODE}" =~ ^[0-9]+$ ]]; then
  echo "ERROR: SYVER_TRIVY_EXIT_CODE must be a non-negative integer, got '${SYVER_TRIVY_EXIT_CODE}'" >&2
  exit 1
fi
if (( SYVER_TRIVY_EXIT_CODE == 1 )); then
  # Rejected rather than merely discouraged: 1 is trivy's own fatal-error code,
  # so this value silently collapses "found vulnerabilities" back into "the
  # scanner never ran" -- the precise defect the rest of this block exists to
  # prevent. Any other non-zero value is fine.
  echo "ERROR: SYVER_TRIVY_EXIT_CODE=1 collides with trivy's own error code; use 2" >&2
  exit 1
fi

run_govulncheck() {
  echo "==> govulncheck"
  if command -v govulncheck >/dev/null 2>&1; then
    govulncheck ./...
  else
    go run "golang.org/x/vuln/cmd/govulncheck@${SYVER_GOVULNCHECK_VERSION}" ./...
  fi
}

run_trivy() {
  trivy fs --scanners "${SYVER_TRIVY_SCANNERS}" --severity "${TRIVY_SEVERITY}" \
    --exit-code "${SYVER_TRIVY_EXIT_CODE}" \
    --ignorefile "${SYVER_TRIVY_IGNOREFILE}" \
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
  mkdir -p "${SYVER_TRIVY_CACHE_DIR}" || echo "WARN: cannot create ${SYVER_TRIVY_CACHE_DIR}, trivy will re-download its DB" >&2
  "${runtime}" run --rm \
    -v "${ROOT}:/src:ro,z" \
    -v "${SYVER_TRIVY_CACHE_DIR}:/root/.cache/trivy:z" \
    -w /src \
    "${SYVER_TRIVY_IMAGE}" \
    fs --scanners "${SYVER_TRIVY_SCANNERS}" --severity "${TRIVY_SEVERITY}" \
    --exit-code "${SYVER_TRIVY_EXIT_CODE}" \
    --ignorefile "${SYVER_TRIVY_IGNOREFILE}" \
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
# Captured, not left to set -e, for the same reason as trivy below: a bare
# errexit death here would skip the summary entirely.
govulncheck_status=0
run_govulncheck || govulncheck_status=$?

# Deliberately not left to `set -e`: trivy now exits non-zero ON FINDINGS, and a
# bare set -e death would kill the script before the summary, making "found
# vulnerabilities" look identical to "the scanner crashed". Capture, then say
# which it was.
trivy_status=0
run_trivy_fs || trivy_status=$?

echo
if (( govulncheck_status == GOVULNCHECK_VULNS_FOUND )); then
  echo "==> SUMMARY: govulncheck FOUND VULNERABILITIES (listed above)"
  echo "    These are reachable call paths, not just affected versions -- govulncheck"
  echo "    reports a vulnerability only when your code can actually reach it."
  exit "${govulncheck_status}"
elif (( govulncheck_status != 0 )); then
  echo "==> SUMMARY: govulncheck FAILED TO RUN (exit ${govulncheck_status})"
  echo "    Nothing was scanned by govulncheck. NOT a clean security result."
  exit "${govulncheck_status}"
elif [[ "${trivy_skipped}" == "1" ]]; then
  echo "==> SUMMARY: govulncheck ran; trivy DID NOT RUN (skipped, see WARN above)"
  echo "    This run is NOT a clean security result."
elif (( trivy_status != 0 && trivy_status == SYVER_TRIVY_EXIT_CODE )); then
  echo "==> SUMMARY: govulncheck ran; trivy FOUND VULNERABILITIES (listed above)"
  echo "    Severities scanned: ${TRIVY_SEVERITY}"
  echo "    Fix the dependency, or add a documented .trivyignore.yaml entry if"
  echo "    there is genuinely no fix -- ci/trivyignore-check.sh re-validates those."
  exit "${trivy_status}"
elif (( trivy_status != 0 )); then
  # Reachable precisely because our findings code is 2: trivy uses 1 for its own
  # fatal errors, so this branch catches a DB fetch failure or bad invocation
  # rather than silently reporting it as vulnerabilities.
  #
  # ORDERING IS LOAD-BEARING: this must be tested BEFORE the enforcement-off
  # branch below. When SYVER_TRIVY_EXIT_CODE=0 a crashed scanner still exits
  # non-zero, and checking "is enforcement off?" first swallowed it as
  # "both ran" with RC=0 -- a scan that never ran reported as if it had, which
  # is the exact failure this script exists to prevent. Classify what happened
  # first, then decide whether to enforce it.
  echo "==> SUMMARY: govulncheck ran; trivy FAILED TO RUN (exit ${trivy_status})"
  echo "    Nothing was scanned. This run is NOT a clean security result."
  exit "${trivy_status}"
elif (( SYVER_TRIVY_EXIT_CODE == 0 )); then
  # Ran to completion, enforcement deliberately off. Say so: at this setting a
  # findings run and a clean run both exit 0, so "no findings" would be false.
  echo "==> SUMMARY: govulncheck and trivy both ran; findings NOT enforced"
  echo "    SYVER_TRIVY_EXIT_CODE=0, so any findings above did not fail this run."
else
  echo "==> SUMMARY: govulncheck and trivy both ran, no findings"
fi
