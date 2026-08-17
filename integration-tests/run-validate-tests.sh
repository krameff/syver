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
export GOSS_BINARY="${repo_root}/release/syver-${platform_spec}"
log_info "Using: '${GOSS_BINARY}', cwd: '$(pwd)', os: ${os}"

export GOSS_USE_ALPHA=1
# Prefer a platform-spec directory (e.g. darwin-arm64/) over an os-only directory (e.g. darwin/)
# so that arch-specific command files use the correct binary. Fall back to os-only for platforms
# that have not been split by arch (e.g. windows/).
if find integration-tests -type d -name "${platform_spec}" | grep -q .; then
  search_dir="${platform_spec}"
else
  search_dir="${os}"
fi
for file in $(find integration-tests -type f -name "*.goss.yaml" | grep "/${search_dir}/" | sort | uniq); do
  args=(
    "-g=${file}"
    "validate"
  )
  log_action "\nTesting \`${GOSS_BINARY} ${args[*]}\` ...\n"
  "${GOSS_BINARY}" "${args[@]}"
done
