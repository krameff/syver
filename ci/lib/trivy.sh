#!/usr/bin/env bash
# Trivy settings shared by ci/security-scan.sh and ci/trivyignore-check.sh, and
# the one place the scanner version is pinned. Sourced, never run; the caller
# sets ROOT to the repo root first.
#
# To bump: change SYVER_TRIVY_VERSION and SYVER_TRIVY_IMAGE together. The
# golangci and release workflows read SYVER_TRIVY_VERSION from this file for
# aquasecurity/setup-trivy, so CI follows without a workflow edit.

# Pinned, not latest: scan findings fail the build, so a floating scanner can
# turn a build red with no repo change.
SYVER_TRIVY_VERSION=0.75.0

# The container fallback, for machines with docker or podman but no trivy
# binary. CI has the binary (setup-trivy) and never takes this path.
#
# Digest, not tag: a version tag is re-pointable, so it floats slowly rather
# than not at all. This must be the manifest LIST digest so every architecture
# keeps working; a per-platform digest (podman manifest inspect lists them)
# breaks arm64. Read it from the registry with
#   curl -sI -H "Authorization: Bearer $(curl -s 'https://auth.docker.io/token?service=registry.docker.io&scope=repository:aquasec/trivy:pull' | jq -r .token)" \
#     -H 'Accept: application/vnd.oci.image.index.v1+json' \
#     https://registry-1.docker.io/v2/aquasec/trivy/manifests/<version> | grep -i docker-content-digest
SYVER_TRIVY_IMAGE="${SYVER_TRIVY_IMAGE:-docker.io/aquasec/trivy@sha256:af6acf9a6b85dfe389a1941505c0ce9efef52a4719635e1a962f022a3d855daa}"  # 0.75.0

TRIVY_SKIP_DIRS="${TRIVY_SKIP_DIRS:-integration-tests,release,site,.venv,.git}"
# trivy's own default is [vuln,secret]. Naming just "vuln" is not a no-op, it
# NARROWS -- it turned the secret scanner off on what is now a blocking gate.
# Shared so the suppression check looks wherever the gate looks; otherwise a
# suppressed secret or misconfig ID would read as "no longer found".
SYVER_TRIVY_SCANNERS="${SYVER_TRIVY_SCANNERS:-vuln,secret,misconfig}"
# The DB is ~700MB and is re-pulled into a throwaway layer on every containerised
# run without this. Honours XDG_CACHE_HOME so it can be moved off a full volume.
SYVER_TRIVY_CACHE_DIR="${SYVER_TRIVY_CACHE_DIR:-${XDG_CACHE_HOME:-${HOME:-/tmp}/.cache}/trivy}"

# trivy_container ENGINE ARGS... runs `trivy ARGS...` in SYVER_TRIVY_IMAGE with
# the repo mounted read-only as the working directory. ENGINE is docker or
# podman: the dev sandbox is podman-only, so a docker-only fallback skipped the
# scan on the machine most development happens on.
trivy_container() {
  local engine="$1"
  shift
  mkdir -p "${SYVER_TRIVY_CACHE_DIR}" || echo "WARN: cannot create ${SYVER_TRIVY_CACHE_DIR}, trivy will re-download its DB" >&2
  # :z relabels the bind mounts for SELinux, which podman on RHEL-family hosts
  # needs and docker ignores harmlessly.
  "${engine}" run --rm \
    -v "${ROOT}:/src:ro,z" \
    -v "${SYVER_TRIVY_CACHE_DIR}:/root/.cache/trivy:z" \
    -w /src \
    "${SYVER_TRIVY_IMAGE}" \
    "$@"
}
