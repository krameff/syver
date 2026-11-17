#!/usr/bin/env bash
set -euo pipefail


usage() {
  echo "Usage: $0 -f <filename> -o <output-dir> -p <platform-spec> -v <version-stamp>"
  echo "   -f <filename>"
  echo "       Name of the release binary to build e.g. linux-amd64"
  echo "       If not provided it will default to the value of platform-spec"
  echo "   -o <output-dir>"
  echo "       Output directory to place the release binary and checksum file"
  echo "       If not provided it will default to 'release/'"
  echo "   -p <platform-spec>"
  echo "       Platform-spec format: <os>-<arch> if unsure try command 'go tool dist list' for possibly combinations"
  echo "       Example platform-spec: linux-amd64, windows-amd64, darwin-amd64, alpha-linux-amd64"
  echo "   -v <version-stamp>"
  echo "       Version-stamp is optional, if not provided it will default to '0.0.0'"
  exit 0
}

# Default values
platform_spec="linux-amd64"
output_dir="release/"
output_fname=""
version_stamp="0.0.0"

OPTSTRING="f:o:p:v:h"
while getopts ${OPTSTRING} opt; do
  case ${opt} in
    f)
      output_fname="${OPTARG}"
      ;;
    o)
      output_dir="${OPTARG}"
      ;;
    p)
      platform_spec="${OPTARG}"
      ;;
    v)
      version_stamp="${OPTARG}"
      ;;
    h)
      usage
      ;;
    *)      usage
      ;;
  esac
done

# Derive os/arch and output_fname after flags are parsed
IFS='- ' read -r -a segments <<< "${platform_spec}"

os="${segments[0]}"
arch="${segments[1]}"
if [[ "${segments[0]}" == "alpha" ]]; then
  os="${segments[1]}"
  arch="${segments[2]}"
fi

if [[ -z "${output_fname}" ]]; then
  output_fname="syver-${platform_spec}"
fi

if [[ "${os}" == "windows" ]]; then
  output_fname="${output_fname}.exe"
fi

output(){
  echo
  output="${output_dir}/${output_fname}"
}

build_pkg() {
    mkdir -p "${output_dir}"
  GOOS="${os}" GOARCH="${arch}" CGO_ENABLED=0 go build \
    -ldflags "-X github.com/krameff/syver/util.Version=${version_stamp} -s -w" \
    -o "${output}" \
    github.com/krameff/syver/cmd/syver

  chmod +x "${output}"
}

hash() {

  function __sha256sum {
    if [[ "$OSTYPE" == "darwin"* ]]; then
      shasum -a 256 "$1"
    else
      sha256sum "$1"
    fi
  }

  (cd "$output_dir" && __sha256sum "${output_fname}" > "${output_fname}.sha256")

}

output
build_pkg
hash
