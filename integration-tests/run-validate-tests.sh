#!/usr/bin/env bash
# shellcheck source=../ci/lib/setup.sh
source "$(dirname "${BASH_SOURCE[0]}")/../ci/lib/setup.sh" || exit 67

platform_spec="${1:?"Must supply name of release binary to build e.g. goss-linux-amd64"}"
# Split platform_spec into platform/arch segments
IFS='- ' read -r -a segments <<< "${platform_spec}"

os="${segments[0]}"
arch="${segments[1]}"

if [[ "${os}" == "linux" && "${arch}" == "amd64" ]]; then
  echo "OS is ${os}/${arch}. This script is not for running tests on linux/amd64."
  echo "linux/amd64 is exercised via the integration-tests/test.sh using Docker containers."
  echo "Non-amd64 Linux architectures (e.g. arm64) are tested here since no Docker containers exist for them."
  exit 1
fi

repo_root="$(git rev-parse --show-toplevel)"
export SYVER_BINARY="${repo_root}/release/syver-${platform_spec}"
log_info "Using: '${SYVER_BINARY}', cwd: '$(pwd)', os: ${os}"

export GOSS_USE_ALPHA=1
# Prefer a platform-spec directory (e.g. darwin-arm64/) over an os-only directory (e.g. darwin/)
# so that arch-specific command files use the correct binary. Fall back to os-only for platforms
# that have not been split by arch (e.g. windows/).
if find integration-tests -type d -name "${platform_spec}" | grep -q .; then
  search_dir="${platform_spec}"
else
  search_dir="${os}"
fi
# A fixture may declare the exit code it EXPECTS, with a comment line of the
# form `# expect-exit: N` (default 0). The expectation lives in the fixture so
# that whoever writes a negative test cannot forget to register it somewhere
# else.
#
# WHY THIS EXISTS. ci/lib/setup.sh sets `errexit` and an ERR trap, and this loop
# used to invoke the binary bare. So the FIRST fixture whose validate exited
# non-zero aborted the entire script, and every remaining fixture silently never
# ran. That made a deliberately-failing fixture impossible to host: it did not
# fail itself, it took the whole suite with it. FEAT-010 hit this directly --
# `package:` and `kernel-param:` now error on every assertion on Windows by
# design, so no passing fixture can be written for them at all, and they had to
# stay `skip: true` with nothing exercising them.
#
# `|| actual=$?` is what makes this safe: a command on the left of `||` is
# exempt from both errexit and the ERR trap, so a non-zero exit is captured
# rather than fatal. Do not "simplify" it to a bare call.
#
# Mismatches are counted and reported at the END rather than exiting on the
# first one, so a single bad fixture can no longer hide the state of every
# fixture after it -- which was the original defect in a second form.
declare -i mismatches=0
for file in $(find integration-tests -type f -name "*.goss.yaml" | grep "/${search_dir}/" | sort | uniq); do
  expected=0
  if grep -qE '^[[:space:]]*#[[:space:]]*expect-exit:[[:space:]]*[0-9]+' "${file}"; then
    expected="$(grep -m1 -oE '^[[:space:]]*#[[:space:]]*expect-exit:[[:space:]]*[0-9]+' "${file}" | grep -oE '[0-9]+$')"
  fi

  args=(
    "-g=${file}"
    "validate"
  )
  log_action "\nTesting \`${SYVER_BINARY} ${args[*]}\` (expect exit ${expected}) ...\n"

  actual=0
  "${SYVER_BINARY}" "${args[@]}" || actual=$?

  if [[ "${actual}" -ne "${expected}" ]]; then
    log_error "${file}: expected exit ${expected}, got ${actual}"
    mismatches+=1
  fi
done

if [[ "${mismatches}" -gt 0 ]]; then
  log_error "${mismatches} fixture(s) exited differently than declared."
  exit 1
fi
log_success "All validate fixtures exited as declared."
