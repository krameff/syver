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
#
# TWO MORE DIRECTIVES, in the same inline form and read the same way:
#
#   # expect-count:   N   total assertions the fixture must produce
#   # expect-skipped: N   how many of them must be SKIPPED
#
# WHY THE EXIT CODE IS NOT ENOUGH. It answers one bit: did anything fail. It
# cannot see an assertion that stopped existing, and it cannot see one that
# quietly turned into a skip -- a skipped assertion never fails, so a resource
# degrading to `skip` reads as a clean pass on a suite that got smaller. Over a
# third of the fixtures here carry `skip: true`, so that is not hypothetical.
#
# WHICH NUMBERS ARE SAFE TO PIN, measured 2026-09-08 rather than assumed:
#   * `Count` is a property of the FIXTURE, not the host. Running every
#     platform's fixtures against a locally built linux/amd64 binary reproduced
#     CI's totals exactly -- 74 for linux-arm64, 82 for darwin, 122 for windows.
#     Safe to pin everywhere, and seedable from any machine.
#   * `Skipped` is host-independent on linux and darwin (32 and 44, both exactly
#     matching CI) but NOT on Windows: the same fixtures skip 33 assertions when
#     driven from a Linux host and 19 on a real Windows Server runner. So
#     Windows fixtures deliberately declare no expect-skipped. Seed it from a
#     real run on that platform, never by inference from another one.
#   * `Failed` is host-dependent by design and is NOT pinned here. That is what
#     the exit code above is for.
#
# Both directives are optional: absent means unchecked, so a new fixture is not
# blocked on someone knowing its numbers. Declaring them is how a fixture stops
# being able to shrink silently.
declare -i mismatches=0
declare -i fixtures=0 total_count=0 total_skipped=0

# Reads `# <name>: N` from a fixture. Prints the number, or nothing when the
# directive is absent, so the caller can tell "declared 0" from "not declared".
read_directive() {
  local name="$1" file="$2"
  grep -m1 -oE "^[[:space:]]*#[[:space:]]*${name}:[[:space:]]*[0-9]+" "${file}" \
    | grep -oE '[0-9]+$' || true
}

for file in $(find integration-tests -type f -name "*.goss.yaml" | grep "/${search_dir}/" | sort | uniq); do
  expected=0
  if grep -qE '^[[:space:]]*#[[:space:]]*expect-exit:[[:space:]]*[0-9]+' "${file}"; then
    expected="$(grep -m1 -oE '^[[:space:]]*#[[:space:]]*expect-exit:[[:space:]]*[0-9]+' "${file}" | grep -oE '[0-9]+$')"
  fi
  want_count="$(read_directive 'expect-count' "${file}")"
  want_skipped="$(read_directive 'expect-skipped' "${file}")"

  args=(
    "-g=${file}"
    "validate"
  )
  log_action "\nTesting \`${SYVER_BINARY} ${args[*]}\` (expect exit ${expected}) ...\n"

  # Output is captured AND echoed. Captured so the summary line can be read
  # back; echoed so a failing run still shows what failed, which is the whole
  # value of the log on a platform nobody can reproduce locally.
  actual=0
  output="$("${SYVER_BINARY}" "${args[@]}" 2>&1)" || actual=$?
  printf '%s\n' "${output}"

  if [[ "${actual}" -ne "${expected}" ]]; then
    log_error "${file}: expected exit ${expected}, got ${actual}"
    mismatches+=1
  fi

  # The LAST summary line, not the first: a fixture that includes others
  # (gossfile.goss.yaml) prints one per included run and only the final line
  # totals them.
  summary="$(printf '%s\n' "${output}" \
    | grep -oE 'Count: [0-9]+, Failed: [0-9]+, Skipped: [0-9]+' | tail -1 || true)"

  if [[ -z "${summary}" ]]; then
    # Not fatal on its own -- but a fixture that declared numbers and produced
    # no summary has not been checked, and silence is exactly the failure mode
    # this block exists to remove.
    if [[ -n "${want_count}${want_skipped}" ]]; then
      log_error "${file}: declares expectations but produced no summary line"
      mismatches+=1
    fi
    continue
  fi

  got_count="$(sed -E 's/Count: ([0-9]+).*/\1/' <<< "${summary}")"
  got_skipped="$(sed -E 's/.*Skipped: ([0-9]+)/\1/' <<< "${summary}")"
  fixtures+=1
  total_count+="${got_count}"
  total_skipped+="${got_skipped}"

  if [[ -n "${want_count}" && "${got_count}" -ne "${want_count}" ]]; then
    log_error "${file}: expected ${want_count} assertions, got ${got_count}"
    mismatches+=1
  fi
  if [[ -n "${want_skipped}" && "${got_skipped}" -ne "${want_skipped}" ]]; then
    log_error "${file}: expected ${want_skipped} skipped, got ${got_skipped}"
    mismatches+=1
  fi
done

# Printed on every run, pass or fail. Each platform job in CI is named
# identically and renders as an identical green tick, while the suites behind
# them differ by close to an order of magnitude. This line is what tells a
# reader which green they are looking at.
log_info "${platform_spec}: ${fixtures} fixtures, ${total_count} assertions, ${total_skipped} skipped"

if [[ "${mismatches}" -gt 0 ]]; then
  log_error "${mismatches} expectation(s) not met."
  exit 1
fi
log_success "All validate fixtures matched their declared exit codes and counts."
