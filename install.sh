#!/bin/sh

{
set -e

# SYVER_* is the primary name; the GOSS_* equivalent stays supported so
# existing pinned invocations keep working. `${VAR:-fallback}` treats an
# exported-but-empty value as unset, so a blank SYVER_VER falls through to a
# real GOSS_VER rather than shadowing it -- the same rule the binary and the
# wrapper scripts follow.
SYVER_VER="${SYVER_VER:-${GOSS_VER:-}}"
SYVER_DST="${SYVER_DST:-${GOSS_DST:-/usr/local/bin}}"

# Ref the wrapper scripts are fetched from: a release tag when one is pinned,
# otherwise the default branch.
WRAPPER_REF="$SYVER_VER"

if [ -z "$SYVER_VER" ]; then
    LATEST_URL="https://github.com/krameff/syver/releases/latest"
    LATEST_EFFECTIVE=$(curl -fsSL -o /dev/null "$LATEST_URL" -w '%{url_effective}') || LATEST_EFFECTIVE=""
    SYVER_VER=${LATEST_EFFECTIVE##*/}
    # This repo's default branch is `main`. It was `master` upstream, and that
    # stale value made the wrapper download below 404 for anyone who had not
    # pinned a version.
    WRAPPER_REF='main'
fi
if [ -z "$SYVER_VER" ]; then
    echo "ERROR: Could not automatically detect latest version, set SYVER_VER env var and re-run"
    exit 1
fi
INSTALL_LOC="${SYVER_DST%/}/syver"
DSYVER_INSTALL_LOC="${SYVER_DST%/}/dsyver"
DGOSS_INSTALL_LOC="${SYVER_DST%/}/dgoss"

# Every download lands in a private directory inside SYVER_DST and is moved
# into place only once it is complete (and, for the binary, verified), so a
# failed or interrupted install never leaves a truncated file or an HTML error
# page where the binary should be. Creating the directory doubles as the
# writability check, and the trap removes it however the script exits.
TMP_DIR=""
cleanup() {
    if [ -n "$TMP_DIR" ]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT
trap 'cleanup; exit 1' HUP INT TERM
TMP_DIR=$(mktemp -d "${SYVER_DST%/}/.syver-install.XXXXXX" 2>/dev/null) || {
    TMP_DIR=""
    echo "ERROR: Cannot write to $SYVER_DST set SYVER_DST elsewhere or use sudo"
    exit 1
}

# download URL FILE: -f turns an HTTP error (a mistyped SYVER_VER is a 404)
# into a failure instead of saving the error page.
download() {
    echo "Downloading $1"
    curl -fsSL "$1" -o "$2" || {
        echo "ERROR: Download failed: $1" >&2
        exit 1
    }
}

# sha256 FILE prints the file's SHA-256: sha256sum on Linux, shasum on macOS.
sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{ print $1 }'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{ print $1 }'
    else
        return 1
    fi
}

case "$(uname -s)" in
    Linux)
        os="linux"
        ;;
    Darwin)
        os="darwin"
        ;;
    *)
        # Windows has a release binary (syver-windows-amd64.exe) but no shell to
        # run this script in; download it from the releases page instead.
        echo "error: unsupported OS: $(uname -s), download a binary from https://github.com/krameff/syver/releases" >&2
        exit 1
        ;;
esac

# Reference: https://wiki.debian.org/ArchitectureSpecificsMemo
case "$(uname -m)" in
    x86_64)
        arch="amd64"
        ;;
    aarch64|arm64)
        arch="arm64"
        ;;
    s390x)
        arch="s390x"
        ;;
    ppc64le)
        arch="ppc64le"
        ;;
    *)
        # Release binaries are 64-bit only; 32-bit systems can build from source.
        echo "error: unknown/unsupported architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

# darwin is published for amd64 and arm64 only.
if [ "$os" = "darwin" ] && [ "$arch" != "amd64" ] && [ "$arch" != "arm64" ]; then
    echo "error: unknown/unsupported architecture: $(uname -m) on $(uname -s)" >&2
    exit 1
fi

release_url="https://github.com/krameff/syver/releases/download/$SYVER_VER"
asset="syver-$os-$arch"

download "$release_url/$asset" "$TMP_DIR/$asset"

# Each release publishes syver_<version>_SHA256SUMS (the version without its
# leading v) listing every release binary. SYVER_SKIP_CHECKSUM=1 skips the
# check, for a mirror or release that does not carry the file.
if [ "${SYVER_SKIP_CHECKSUM:-}" = "1" ]; then
    echo "WARNING: SYVER_SKIP_CHECKSUM=1, installing $asset without verifying its checksum" >&2
else
    sums="syver_${SYVER_VER#v}_SHA256SUMS"
    curl -fsSL "$release_url/$sums" -o "$TMP_DIR/$sums" || {
        echo "ERROR: Could not download $release_url/$sums to verify the binary; set SYVER_SKIP_CHECKSUM=1 to install without verifying" >&2
        exit 1
    }
    want=$(awk -v name="$asset" '$2 == name { print $1; exit }' "$TMP_DIR/$sums")
    if [ -z "$want" ]; then
        echo "ERROR: $asset is not listed in $sums" >&2
        exit 1
    fi
    got=$(sha256 "$TMP_DIR/$asset") || {
        echo "ERROR: Neither sha256sum nor shasum is available to verify the binary; set SYVER_SKIP_CHECKSUM=1 to install without verifying" >&2
        exit 1
    }
    if [ "$got" != "$want" ]; then
        echo "ERROR: Checksum mismatch for $asset: expected $want, got $got" >&2
        exit 1
    fi
    echo "Verified $asset against $sums"
fi
chmod +x "$TMP_DIR/$asset"
mv -f "$TMP_DIR/$asset" "$INSTALL_LOC"
echo "Syver $SYVER_VER has been installed to $INSTALL_LOC"
echo "syver --version"
"$INSTALL_LOC" --version

# The wrapper scripts come from the source tree rather than the release, so
# SHA256SUMS does not cover them.
dsyver_url="https://raw.githubusercontent.com/krameff/syver/$WRAPPER_REF/extras/dsyver/dsyver"
download "$dsyver_url" "$TMP_DIR/dsyver"
chmod +rx "$TMP_DIR/dsyver"
mv -f "$TMP_DIR/dsyver" "$DSYVER_INSTALL_LOC"
echo "dsyver $WRAPPER_REF has been installed to $DSYVER_INSTALL_LOC"

# The goss-named wrapper is still shipped as a working shim, under the
# compatibility policy in docs/goss-vs-syver.md.
dgoss_url="https://raw.githubusercontent.com/krameff/syver/$WRAPPER_REF/extras/dsyver/dgoss"
download "$dgoss_url" "$TMP_DIR/dgoss"
chmod +rx "$TMP_DIR/dgoss"
mv -f "$TMP_DIR/dgoss" "$DGOSS_INSTALL_LOC"
echo "dgoss $WRAPPER_REF has been installed to $DGOSS_INSTALL_LOC (compatibility shim)"

# dsyver copies the syver binary into the container it tests, and the darwin
# binary installed above cannot run in a Linux container.
if [ "$os" = "darwin" ]; then
    echo
    echo "NOTE: dsyver and dgoss need a Linux syver binary to copy into the container. For example:"
    echo "  curl -fL $release_url/syver-linux-$arch -o ~/syver-linux-$arch"
    echo "  chmod +x ~/syver-linux-$arch"
    echo "  export SYVER_PATH=~/syver-linux-$arch"
fi
}
