#!/usr/bin/env bash
# Shared discovery and depends-on E2E assertion steps.
# Callers define a goss_runner function, then invoke:
#   run_discovery_e2e_steps <examples-dir> goss_runner [fixture-check-dir]
#   run_depends_on_e2e_steps <examples-dir> goss_runner [fixture-check-dir]
#
# examples-dir is passed to goss_runner (e.g. /goss/examples/discovery in Docker).
# fixture-check-dir defaults to examples-dir; set it to the host checkout path when
# examples-dir is only valid inside the test container.

run_discovery_e2e_steps() {
  local examples_dir="$1"
  local runner="$2"
  local fixture_dir="${3:-${examples_dir}}"

  for f in discovery.yaml goss.yml goss-inline.yml goss-with-deps.yml; do
    if [[ ! -f "${fixture_dir}/${f}" ]]; then
      echo "discovery example fixture missing: ${fixture_dir}/${f}" >&2
      return 1
    fi
  done

  local disc_out
  disc_out="$("${runner}" validate -g "${examples_dir}/discovery.yaml" --format discovery)"
  echo "${disc_out}"
  echo "${disc_out}" | grep -q '"Discovered"'
  echo "${disc_out}" | grep -q 'hosts_exists'

  local output
  output="$("${runner}" validate -g "${examples_dir}/goss.yml" \
    --discover "${examples_dir}/discovery.yaml" --format documentation)"
  echo "${output}"
  echo "${output}" | grep -q 'Failed: 0'

  local inline_out
  inline_out="$("${runner}" validate -g "${examples_dir}/goss-inline.yml" --format documentation)"
  echo "${inline_out}"
  echo "${inline_out}" | grep -q 'Failed: 0'

  local deps_out
  deps_out="$("${runner}" validate -g "${examples_dir}/goss-with-deps.yml" \
    --discover "${examples_dir}/discovery.yaml" --format documentation || true)"
  echo "${deps_out}"
  echo "${deps_out}" | grep -q 'Failed: 1'
  echo "${deps_out}" | grep -q 'Skipped: 1'
}

run_depends_on_e2e_steps() {
  local examples_dir="$1"
  local runner="$2"
  local fixture_dir="${3:-${examples_dir}}"

  if [[ ! -f "${fixture_dir}/goss.yml" ]]; then
    echo "depends-on example fixture missing under ${fixture_dir}" >&2
    return 1
  fi

  local output
  output="$("${runner}" validate -g "${examples_dir}/goss.yml" --format documentation || true)"
  echo "${output}"
  echo "${output}" | grep -q 'Failed: 1'
  echo "${output}" | grep -q 'Skipped: 1'
}
