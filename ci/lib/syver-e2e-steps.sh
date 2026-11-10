#!/usr/bin/env bash
# Shared discovery and depends-on E2E assertion steps.
# Callers define a syver_runner function, then invoke:
#   run_discovery_e2e_steps <examples-dir> syver_runner [fixture-check-dir]
#   run_depends_on_e2e_steps <examples-dir> syver_runner [fixture-check-dir]
#
# examples-dir is passed to syver_runner (e.g. /goss/examples/discovery in Docker).
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

# run_pkg_baseline_e2e_steps <syver-runner> <shell-runner> <os>
#
# Records a pkg-baseline of the host the runners reach, then breaks it in each
# way the resource reports. syver-runner runs the syver binary with its
# arguments; shell-runner runs one sh -c script on the same host, to edit the
# scratch spec. os is the integration distro name (alpine3, arch, jammy, ...).
run_pkg_baseline_e2e_steps() {
  local runner="$1"
  local sh_runner="$2"
  local os="$3"
  local spec=/tmp/pkg-baseline.yaml
  local out rc

  # Fails with what was expected and what came back, rather than a bare exit.
  _pkgb_expect() {
    local label="$1" needle="$2" haystack="$3"
    if ! grep -qF -- "${needle}" <<<"${haystack}"; then
      echo "ERROR: pkg-baseline e2e (${os}): ${label}: expected to find '${needle}' in:" >&2
      echo "${haystack}" >&2
      return 1
    fi
  }
  _pkgb_rc() {
    local label="$1" want="$2" got="$3"
    if [[ "${got}" -ne "${want}" ]]; then
      echo "ERROR: pkg-baseline e2e (${os}): ${label}: expected exit ${want}, got ${got}" >&2
      return 1
    fi
  }
  _pkgb_validate() {
    rc=0
    out="$("${runner}" -g "${spec}" validate 2>&1)" || rc=$?
    echo "${out}"
  }

  # 8 (setup): a virtual package, which apk records with a version that has no
  # -rN suffix. Offline, with no dependencies, so it needs no repository.
  if [[ "${os}" == alpine3 ]]; then
    "${sh_runner}" 'apk add --no-network --virtual .syver-e2e >/dev/null'
  fi

  # 1. Record the host. The file must carry syver-version: first.
  "${sh_runner}" "rm -f ${spec}"
  rc=0
  out="$("${runner}" -g "${spec}" add pkg-baseline 2>&1)" || rc=$?
  echo "${out}"
  _pkgb_rc "add pkg-baseline" 0 "${rc}"
  local first manager
  first="$("${sh_runner}" "head -1 ${spec}")"
  _pkgb_expect "first line of the generated spec" "syver-version: 0.20.0" "${first}"
  manager="$("${sh_runner}" "sed -n 's/^  \\([a-z]*\\):\$/\\1/p' ${spec}")"
  echo "INFO: pkg-baseline e2e (${os}): package manager ${manager}"

  if [[ "${os}" == alpine3 ]]; then
    _pkgb_expect "virtual package recorded by name" "      - .syver-e2e" "$("${sh_runner}" "cat ${spec}")"
  fi

  # 2. A fresh baseline passes.
  _pkgb_validate
  _pkgb_rc "fresh baseline" 0 "${rc}"
  _pkgb_expect "fresh baseline" "Count: 2, Failed: 0," "${out}"

  # 3. Drop the last recorded name (installed, now unrecorded) and record one
  #    that is not installed: both properties fail, naming both.
  local dropped
  dropped="$("${sh_runner}" "sed -n 's/^      - //p' ${spec} | tail -1")"
  "${sh_runner}" "sed -i '/^      - ${dropped}\$/d' ${spec} &&
    { sed -n '1,/^      consist-of:\$/p' ${spec}; echo '      - zz-syver-not-installed'; sed '1,/^      consist-of:\$/d' ${spec}; } > ${spec}.new &&
    mv ${spec}.new ${spec}"
  _pkgb_validate
  _pkgb_rc "edited baseline" 1 "${rc}"
  _pkgb_expect "edited baseline" "Count: 2, Failed: 2," "${out}"
  _pkgb_expect "edited baseline names the unrecorded package" "${dropped}" "${out}"
  _pkgb_expect "edited baseline names the missing package" "zz-syver-not-installed" "${out}"

  # 4. Ignoring the dropped name leaves only the missing one.
  "${sh_runner}" "printf '    ignore:\\n    - %s\\n' '${dropped}' >> ${spec}"
  _pkgb_validate
  _pkgb_rc "ignore" 1 "${rc}"
  _pkgb_expect "ignore" "Count: 2, Failed: 1," "${out}"

  # 5. Re-add with versions: the list is rebuilt from the host, ignore is kept,
  #    and every version matches, including rpm's gpg-pubkey keys.
  rc=0
  out="$("${runner}" -g "${spec}" add pkg-baseline --versions exact 2>&1)" || rc=$?
  echo "${out}"
  _pkgb_rc "add --versions exact" 0 "${rc}"
  _pkgb_expect "re-add keeps ignore" "    - ${dropped}" "$("${sh_runner}" "sed -n '/^    ignore:/,\$p' ${spec}")"
  _pkgb_validate
  _pkgb_rc "exact baseline" 0 "${rc}"
  _pkgb_expect "exact baseline" "Count: 3, Failed: 0," "${out}"

  # The name whose recorded version the next steps change: the first one that
  # is neither rpm's gpg-pubkey (always compared exactly) nor an apk virtual
  # package (its version is a timestamp, higher than any fixed value below).
  local vname
  vname="$("${sh_runner}" "sed -n '/^    versions:/,/^    [a-z]/s/^      \\([^ ]*\\):\$/\\1/p' ${spec} | grep -v -e '^gpg-pubkey\$' -e '^\\.' | head -1")"
  _pkgb_set_version() {
    "${sh_runner}" "sed -i '/^      ${vname}:\$/{n;s/^      - .*/      - $1/}' ${spec}"
  }
  _pkgb_set_version "0.0.1"
  _pkgb_validate
  _pkgb_rc "exact, one version changed" 1 "${rc}"
  _pkgb_expect "exact, one version changed" "Count: 3, Failed: 1," "${out}"
  _pkgb_expect "exact names the package" "${vname}" "${out}"

  # 6. newer: a lower recorded version passes, a higher one fails.
  "${sh_runner}" "sed -i 's/^    version-match: exact\$/    version-match: newer/' ${spec}"
  _pkgb_set_version "0"
  _pkgb_validate
  _pkgb_rc "newer, lower recorded" 0 "${rc}"
  _pkgb_expect "newer, lower recorded" "Count: 3, Failed: 0," "${out}"
  if [[ "${manager}" == apk ]]; then
    _pkgb_set_version "99999-r0"
  else
    _pkgb_set_version "99999:1-1"
  fi
  _pkgb_validate
  _pkgb_rc "newer, higher recorded" 1 "${rc}"
  _pkgb_expect "newer, higher recorded" "Count: 3, Failed: 1," "${out}"

  # 7. A baseline for another package manager fails, saying so.
  local other=dpkg
  [[ "${manager}" == dpkg ]] && other=rpm
  "${sh_runner}" "sed -i 's/^  ${manager}:\$/  ${other}:/' ${spec}"
  _pkgb_validate
  _pkgb_rc "wrong key" 1 "${rc}"
  _pkgb_expect "wrong key" "baseline recorded for ${other}; this host's package manager is ${manager}" "${out}"

  "${sh_runner}" "rm -f ${spec}"
}
