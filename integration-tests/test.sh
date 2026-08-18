#!/usr/bin/env bash
# shellcheck source=../ci/lib/setup.sh
source "$(dirname "${BASH_SOURCE[0]}")/../ci/lib/setup.sh" || exit 67
# shellcheck source=../ci/lib/syver-e2e-steps.sh
source "${REPO_ROOT}/ci/lib/syver-e2e-steps.sh" || exit 67
# preserve current behaviour
set -x

: "${DOCKER_BIN:=docker}"

os="${1:?"Need OS as 1st arg. e.g. alpine arch rockylinux9 jammy bullseye"}"
arch="${2:?"Need arch as 2nd arg. e.g. amd64 arm64"}"

vars_inline="{inline: bar, overwrite: bar}"
container_repository="ghcr.io/krameff"

# setup places us inside repo-root; this preserves current behaviour with least change.
cd integration-tests

cp "../release/syver-linux-$arch" "syver/$os/"
# Always build. This was previously gated on `md5sum -c "Dockerfile_${os}.md5"`
# with a pull branch behind an elif, but no Dockerfile_*.md5 file has ever
# existed in this tree: development/build_images.sh records the digest as an
# image LABEL (rocks.syver.dockerfile-md5), never as a file on disk, so the two
# halves of that cache scheme were never connected. The check therefore always
# failed, the build branch always won, and the pull branch was unreachable.
# Behaviour is unchanged; this just drops the dead branch and the confusing
# "No such file or directory" printed on every run.
$DOCKER_BIN build -t "$container_repository/syver_${os}:latest" --file "Dockerfile_$os" .

container_name="syver_int_test_${os}_${arch}"
docker_exec() {
  $DOCKER_BIN exec "$container_name" "$@"
}

# Drops "versions:" keys and the version list items directly under them, so
# the generated-vs-expected snapshot diffs below don't break every time an
# OS image picks up a routine package patch release.
strip_versions() {
  awk '
    /versions:/ { skip=1; next }
    skip && /^[[:space:]]*-/ { next }
    { skip=0; print }
  '
}

# Cleanup any old containers
if $DOCKER_BIN ps -a | grep "$container_name";then
  $DOCKER_BIN rm -vf "$container_name"
fi

# Setup local httbin
# FIXME: this is a quick hack to fix intermittent CI issues
network=syver-test
$DOCKER_BIN network create --driver bridge --subnet '172.19.0.0/16' $network
$DOCKER_BIN run -d --name httpbin --network $network docker.io/kennethreitz/httpbin
opts=(--env OS=$os --cap-add SYS_ADMIN -v "$PWD/syver:/goss" -d --name "$container_name" --security-opt seccomp:unconfined --security-opt label:disable --privileged)
id=$($DOCKER_BIN run "${opts[@]}" --network $network "$container_repository/syver_$os" /sbin/init)
# Newer Docker (verified: 29.7.2) no longer populates the legacy top-level
# .NetworkSettings.IPAddress field for a container attached to a
# non-default (custom) network at creation time -- only the per-network
# .NetworkSettings.Networks.<name>.IPAddress map entry is populated. Podman
# supports the same per-network path, so this keeps the $DOCKER_BIN
# abstraction working identically on both runtimes. $ip itself is unused
# elsewhere in this script; this only fixes the inspect call from panicking.
ip=$($DOCKER_BIN inspect --format "{{ (index .NetworkSettings.Networks \"$network\").IPAddress }}" "$id")
trap "rv=\$?; $DOCKER_BIN rm -vf $id || :;$DOCKER_BIN rm -vf httpbin || :;$DOCKER_BIN network rm $network || :; exit \$rv" INT TERM EXIT
# Give httpd time to start up, adding 1 second to see if it helps with intermittent CI failures
docker_exec "/goss/$os/syver-linux-$arch" -g "/goss/goss-wait.yaml" validate -r 10s -s 100ms && sleep 1

out=$(docker_exec "/goss/$os/syver-linux-$arch" --vars "/goss/vars.yaml" --vars-inline "$vars_inline" -g "/goss/$os/goss.yaml" validate)
echo "$out"

# Three counts, because the distros genuinely differ:
#   arch    does not include goss-service.yaml at all
#   alpine3 is the one distro with a real runlevels expectation
#   the rest carried `runlevels: []`, which is no longer an expectation --
#           an empty list means "nothing to assert" everywhere now, so it
#           no longer contributes a vacuous passing test
case $os in
  arch)    egrep -q 'Count: 106, Failed: 0, Skipped: 3' <<<"$out" ;;
  alpine3) egrep -q 'Count: 127, Failed: 0, Skipped: 5' <<<"$out" ;;
  *)       egrep -q 'Count: 126, Failed: 0, Skipped: 5' <<<"$out" ;;
esac

syver_bin="/goss/$os/syver-linux-$arch"
syver_runner() {
  docker_exec "${syver_bin}" "$@"
}

run_discovery_e2e_steps "/goss/examples/discovery" syver_runner \
  "${REPO_ROOT}/integration-tests/syver/examples/discovery"
run_depends_on_e2e_steps "/goss/examples/depends-on" syver_runner \
  "${REPO_ROOT}/integration-tests/syver/examples/depends-on"

if [[ ! $os == "arch" ]]; then
  docker_exec /goss/generate_goss.sh "$os" "$arch"

  # docker exec $container_name bash -c "cp /goss/${os}/goss-generated-$arch.yaml /goss/${os}/goss-expected.yaml"
  diff -wu <(docker_exec cat "/goss/${os}/goss-expected.yaml" | strip_versions) \
           <(docker_exec cat "/goss/${os}/goss-generated-$arch.yaml" | strip_versions)

  # docker exec $container_name bash -c "cp /goss/${os}/goss-aa-generated-$arch.yaml /goss/${os}/goss-aa-expected.yaml"
  diff -wu <(docker_exec cat "/goss/${os}/goss-aa-expected.yaml" | strip_versions) \
           <(docker_exec cat "/goss/${os}/goss-aa-generated-$arch.yaml" | strip_versions)

  docker_exec /goss/generate_goss.sh "$os" "$arch" -q

  # docker exec $container_name bash -c "cp /goss/${os}/goss-generated-$arch.yaml /goss/${os}/goss-expected-q.yaml"
  diff -wu <(docker_exec cat "/goss/${os}/goss-expected-q.yaml" | strip_versions) \
           <(docker_exec cat "/goss/${os}/goss-generated-$arch.yaml" | strip_versions)
fi

#docker rm -vf syver_int_test_$os
