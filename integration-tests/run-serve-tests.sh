#!/usr/bin/env bash
# shellcheck source=../ci/lib/setup.sh
source "$(dirname "${BASH_SOURCE[0]}")/../ci/lib/setup.sh" || exit 67

platform_spec="${1:?Must supply name of release binary to build e.g. goss-linux-amd64}"

# Split platform_spec into platform/arch segments
IFS='- ' read -r -a segments <<< "${platform_spec}"

os="${segments[0]}"
arch="${segments[1]}"

find_open_port() {
  local startAt="${1:?"Supply start of port range"}"
  local endAt="${2:?"Supply end of port range"}"
  local how_many="${3:-"1"}"

  if [[ "$(go env GOOS)" == "windows" ]]; then
    # ss (see unix implementation below) doesn't exist on Windows, so fall back on just choosing a random number inside the range (since netstat is _slow_).
    # Thanks also to https://blog.netspi.com/15-ways-to-bypass-the-powershell-execution-policy/
    powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "integration-tests/Find-AvailablePort.ps1 -startAt ${startAt} -endAt ${endAt}"
  elif [[ "$(go env GOOS)" == "darwin" ]]; then
    jot -n -r 1 1025 65535
  else
    # Thanks to https://unix.stackexchange.com/questions/55913/whats-the-easiest-way-to-find-an-unused-local-port
    comm -23 \
      <(seq "${startAt}" "${endAt}" | sort) \
      <(ss -tan | tail -n +2 | awk '{print $4}' | cut -d':' -f2 | sort -u) |
      shuf -n "${how_many}" ||
      shuf -i "${startAt}-${endAt}" -n "${how_many}"
  fi
}

cleanup() {
  # MUST be the first statement in this function: $? still holds the script's
  # real exit status here, and any command below would overwrite it. This was
  # previously `exit "${ret:-0}"` with `ret` never assigned anywhere, so the
  # trap unconditionally exited 0 and every serve-test failure was reported as
  # a pass.
  local ret=$?
  local binary_name
  binary_name="$(basename "${SYVER_BINARY}")"
  log_info "Killing syver serve process to clean up, exit code for tests was ${ret}..."
  # The kills must not change the outcome: if no process is left to reap,
  # killall's non-zero status would otherwise become the script's exit code
  # under `set -o errexit`, turning a clean run into a spurious failure.
  if [[ "${os}" == "darwin" ]]; then
    killall "${binary_name}" || true
  elif [[ "${os}" == "linux" ]]; then
    killall "${binary_name}" || true
  elif [[ "${os}" == "windows" ]]; then
    # Can't use killall, doesn't exist on Windows. Also would interfere with concurrent runs.
    ps -W |
      awk "/${binary_name}/,NF=1" |
      xargs kill || true
  fi
  exit "${ret}"
}
trap cleanup EXIT

repo_root="$(git rev-parse --show-toplevel)"
export SYVER_BINARY="${repo_root}/release/syver-${platform_spec}"
log_info "Using: '${SYVER_BINARY}', cwd: '$(pwd)'"

export GOSS_USE_ALPHA=1
open_port="$(find_open_port 1025 65335)"
echo "${open_port}"
args=(
  "-g=${repo_root}/integration-tests/goss/goss-serve.yaml"
  "serve"
  "--listen-addr=127.0.0.1:${open_port}"
)
log_action "\nTesting \`${SYVER_BINARY} ${args[*]}\` ...\n"
"${SYVER_BINARY}" "${args[@]}" &
base_url="http://127.0.0.1:${open_port}"
[[ "$(go env GOOS)" == "darwin" ]] && sleep 2

assert_response_contains() {
  local url="${1:?"1st arg: url"}"
  local test_name="${2:?"2nd arg: test name"}"
  local expectation="${3:?"3rd arg: response body match"}"
  local accept_header="${4:-""}"

  curl_args=("--silent")
  [[ -n "${accept_header:-}" ]] && curl_args+=("-H" "Accept: ${accept_header}")
  curl_args+=("${url}")
  log_info "curl ${curl_args[*]}"
  curl="curl"
  [[ "$(go env GOOS)" == "windows" ]] && curl="curl.exe"
  response="$(${curl} "${curl_args[@]}")"
  if grep --quiet "${expectation}" <<<"${response}"; then
    log_success "Passed: ${test_name}"
    return 0
  fi
  log_error "Failed: ${test_name}"
  log_error "  Expected: ${expectation}"
  log_error "  Response: ${response}"
  return 1
}
failure="false"
on_test_failure() {
  failure="true"
}

# /healthz endpoint
assert_response_contains "${base_url}/healthz" "no accept header" "Count: 2, Failed: 0, Skipped: 0" "" || on_test_failure
assert_response_contains "${base_url}/healthz" "tap accept header" "Count: 2, Failed: 0, Skipped: 0" "application/vnd.goss-documentation" || on_test_failure
assert_response_contains "${base_url}/healthz" "json accept header" "\"failed-count\":0" "application/json" || on_test_failure
assert_response_contains "${base_url}/healthz" "prometheus accept header" "goss_tests_outcomes_total" "application/vnd.goss-prometheus" || on_test_failure

# /metrics - specific prometheus metrics endpoint
assert_response_contains "${base_url}/metrics" "prometheus accept header" "goss_tests_outcomes_total" "" || on_test_failure

# Deliberately an `if` rather than `[[ ... ]] && log_fatal ...`: as an and-list,
# a passing run leaves the failed `[[ ]]` as the script's last command, so the
# script's natural exit status was 1 even when every assertion passed. That was
# invisible only because the EXIT trap discarded it.
if [[ "${failure}" == "true" ]]; then
  log_fatal "Test(s) failed, check output above."
fi
log_success "All serve tests passed."
exit 0
