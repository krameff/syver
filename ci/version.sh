#!/usr/bin/env bash
# Prints the version stamp for a local build, in `git describe --tags` form:
# v0.12.1 on a release, v0.12.1-3-gabc1234 three commits past one.
#
# Plain `git describe` gets this wrong here. A release is a pull request from
# devel into main, merged as a merge commit, and the tag goes on that merge
# commit, which never returns to devel. From devel the newest reachable tag is
# therefore an old one, and every devel build was stamped v0.9.4-<n>-g<sha>
# long after v0.12.1 shipped.
#
# So a release counts as contained when its tagged commit is an ancestor of
# HEAD (a build from main), or when that commit's second parent is (a build
# from devel: the second parent of a release merge is the devel commit it
# released, so HEAD has everything that release had). Commits are counted
# from whichever of the two matched.
#
# Only vX.Y.Z tags are considered, so older pre-release tags such as
# v0.5.1_alpha never win. With no match it falls back to `git describe`.
set -euo pipefail

head="$(git rev-parse HEAD)"
short="$(git rev-parse --short HEAD)"

while read -r tag; do
  [[ -n "$tag" ]] || continue
  commit="$(git rev-parse "${tag}^{commit}")"
  base=""
  if git merge-base --is-ancestor "$commit" "$head"; then
    base="$commit"
  elif released="$(git rev-parse -q --verify "${commit}^2")" &&
    git merge-base --is-ancestor "$released" "$head"; then
    base="$released"
  fi
  if [[ -n "$base" ]]; then
    count="$(git rev-list --count "${base}..${head}")"
    if [[ "$count" == 0 ]]; then
      echo "$tag"
    else
      echo "${tag}-${count}-g${short}"
    fi
    exit 0
  fi
done < <(git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$')

git describe --tags --always
