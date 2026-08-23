#!/usr/bin/env bash
# Re-validates every entry in .trivyignore against a fresh, unfiltered trivy
# scan so suppressed findings don't go stale silently. Flags entries that:
#   - no longer appear in the scan (may be fixed/removed -- candidate to delete)
#   - now have a fixed version available (was blank before)
# Non-blocking by default: prints warnings but exits 0 unless
# TRIVYIGNORE_STRICT=1, since a missing trivy binary shouldn't block commits.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

IGNORE_FILE=".trivyignore"
# NO severity filter here, deliberately. This script asks "does this suppressed
# ID still exist?", not "does it breach the gate?" -- a filter is the same bug
# class as the missing --ignorefile was: an entry for a finding below the
# threshold reads as "no longer found" and the script advises deleting a live
# suppression. The old default omitted LOW entirely. Verified: unfiltered, the
# scan still returns exactly GO-2026-5932 and nothing spurious.
TRIVY_SKIP_DIRS="${TRIVY_SKIP_DIRS:-integration-tests,release,site,.venv,.git}"
# Keep in step with ci/security-scan.sh: the validator must look wherever the
# gate looks, or a suppressed secret/misconfig ID would read as "no longer found".
SYVER_TRIVY_SCANNERS="${SYVER_TRIVY_SCANNERS:-vuln,secret}"
SYVER_TRIVY_CACHE_DIR="${SYVER_TRIVY_CACHE_DIR:-${XDG_CACHE_HOME:-${HOME:-/tmp}/.cache}/trivy}"
# Pinned for the same reasons as ci/security-scan.sh; keep the two in step.
SYVER_TRIVY_IMAGE="${SYVER_TRIVY_IMAGE:-docker.io/aquasec/trivy@sha256:62b1e65e8869bc4b4c6aa4fa2b21595256c7c2f6018a9d9ad61caf87187c1969}"  # 0.74.0

if [[ ! -f "${IGNORE_FILE}" ]]; then
  echo "[trivyignore-check] no ${IGNORE_FILE}, skipping"
  exit 0
fi

mapfile -t ignored_ids < <(grep -vE '^\s*(#|$)' "${IGNORE_FILE}")

if [[ ${#ignored_ids[@]} -eq 0 ]]; then
  echo "[trivyignore-check] ${IGNORE_FILE} has no active entries, skipping"
  exit 0
fi

# --ignorefile /dev/null is load-bearing, not tidiness. trivy picks up
# ./.trivyignore automatically, so without it this scans FILTERED BY THE VERY
# FILE IT IS VALIDATING: every suppressed ID is missing from the results, the
# loop below reports each one as "no longer found", and following that advice
# deletes a suppression that was still doing its job. Verified: with the
# auto-loaded ignorefile the scan returns only the two x/mod CVEs; with it
# bypassed the same scan also returns GO-2026-5932, the sole .trivyignore entry.
# --exit-code 0 is explicit and load-bearing: this script reports staleness and
# must never fail on findings. trivy reads TRIVY_<FLAG> from the environment, so
# without it an exported TRIVY_EXIT_CODE (which ci/security-scan.sh invites you
# to set) makes trivy exit non-zero here, and the check below silently degrades
# to "cannot validate" -- defeating the --ignorefile fix. Our own knob is now
# SYVER_-prefixed, but the bare name still arrives from muscle memory or other
# tooling, so this guard stays. Verified: exporting
# TRIVY_EXIT_CODE=1 turned this into "trivy scan failed to run".
run_trivy_json() {
  trivy fs --scanners "${SYVER_TRIVY_SCANNERS}" \
    --ignorefile /dev/null --exit-code 0 \
    --skip-dirs "${TRIVY_SKIP_DIRS}" --format json --quiet .
}

run_trivy_json_container() {
  local engine="$1"
  mkdir -p "${SYVER_TRIVY_CACHE_DIR}" || echo "WARN: cannot create ${SYVER_TRIVY_CACHE_DIR}" >&2
  # :z relabels for SELinux -- security-scan.sh already does this and this one
  # did not, so on a RHEL-family host the bind mount could be unreadable here
  # while the other scan worked.
  "${engine}" run --rm -v "${ROOT}:/src:ro,z" \
    -v "${SYVER_TRIVY_CACHE_DIR}:/root/.cache/trivy:z" \
    -w /src "${SYVER_TRIVY_IMAGE}" \
    fs --scanners "${SYVER_TRIVY_SCANNERS}" \
    --ignorefile /dev/null --exit-code 0 \
    --skip-dirs "${TRIVY_SKIP_DIRS}" --format json --quiet .
}

scan_json=""
scan_failed=0

if command -v trivy >/dev/null 2>&1; then
  scan_json="$(run_trivy_json)" || scan_failed=1
elif command -v docker >/dev/null 2>&1; then
  scan_json="$(run_trivy_json_container docker)" || scan_failed=1
elif command -v podman >/dev/null 2>&1; then
  scan_json="$(run_trivy_json_container podman)" || scan_failed=1
else
  echo "WARN: [trivyignore-check] trivy not found and no container engine (docker/podman) available -- cannot re-validate ${IGNORE_FILE} entries: ${ignored_ids[*]}" >&2
  echo "WARN: install trivy or run with docker/podman available to verify these are still needed" >&2
  [[ "${TRIVYIGNORE_STRICT:-}" == "1" ]] && exit 1
  exit 0
fi

if [[ "${scan_failed}" -eq 1 || -z "${scan_json}" ]]; then
  echo "WARN: [trivyignore-check] trivy scan failed to run (e.g. vulnerability DB unreachable) -- cannot re-validate ${IGNORE_FILE} entries: ${ignored_ids[*]}" >&2
  echo "WARN: re-run manually once the trivy DB is reachable to verify these are still needed" >&2
  [[ "${TRIVYIGNORE_STRICT:-}" == "1" ]] && exit 1
  exit 0
fi

status=0
for id in "${ignored_ids[@]}"; do
  match="$(echo "${scan_json}" | jq -r --arg id "${id}" \
    '[.Results[]? | (.Vulnerabilities[]? | select(.VulnerabilityID == $id)),
        (.Secrets[]? | select(.RuleID == $id)),
        (.Misconfigurations[]? | select(.ID == $id))] | .[0] // empty')"

  if [[ -z "${match}" ]]; then
    echo "WARN: [trivyignore-check] ${id} no longer found in scan results -- consider removing it from ${IGNORE_FILE}" >&2
    status=1
    continue
  fi

  fixed_version="$(echo "${match}" | jq -r '.FixedVersion // empty')"
  if [[ -n "${fixed_version}" ]]; then
    echo "WARN: [trivyignore-check] ${id} now has a fixed version (${fixed_version}) -- consider upgrading instead of suppressing" >&2
    status=1
  fi
done

if [[ ${status} -eq 0 ]]; then
  echo "[trivyignore-check] all ${#ignored_ids[@]} ${IGNORE_FILE} entries still apply, no fix available"
fi

[[ "${TRIVYIGNORE_STRICT:-}" == "1" ]] && exit "${status}"
exit 0
