#!/bin/sh

{
set -e

LATEST_URL="https://github.com/krameff/syver/releases/latest"
LATEST_EFFECTIVE=$(curl -s -L -o /dev/null ${LATEST_URL} -w '%{url_effective}')
LATEST=${LATEST_EFFECTIVE##*/}

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
    SYVER_VER="$LATEST"
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
touch "$INSTALL_LOC" || { echo "ERROR: Cannot write to $SYVER_DST set SYVER_DST elsewhere or use sudo"; exit 1; }

# Reference: https://wiki.debian.org/ArchitectureSpecificsMemo
case "$(uname -m)" in
    x86_64)
        arch="amd64"
        ;;
    aarch32|arm)
        arch="arm"
        ;;
    aarch64|arm64)
        arch="arm64"
        ;;
    s390x)
        arch="s390x"
        ;;
    i?86)
        arch="386"
        ;;
    *)
        echo "error: unknown/unsupported architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

url="https://github.com/krameff/syver/releases/download/$SYVER_VER/syver-linux-$arch"

echo "Downloading $url"
curl -L "$url" -o "$INSTALL_LOC"
chmod +x "$INSTALL_LOC"
echo "Syver $SYVER_VER has been installed to $INSTALL_LOC"
echo "syver --version"
"$INSTALL_LOC" --version

dsyver_url="https://raw.githubusercontent.com/krameff/syver/$WRAPPER_REF/extras/dsyver/dsyver"
echo "Downloading $dsyver_url"
curl -L "$dsyver_url" -o "$DSYVER_INSTALL_LOC"
chmod +rx "$DSYVER_INSTALL_LOC"
echo "dsyver $WRAPPER_REF has been installed to $DSYVER_INSTALL_LOC"

# The goss-named wrapper is still shipped as a working shim for one major
# version, matching the compatibility promise the rest of the project makes.
dgoss_url="https://raw.githubusercontent.com/krameff/syver/$WRAPPER_REF/extras/dsyver/dgoss"
echo "Downloading $dgoss_url"
curl -L "$dgoss_url" -o "$DGOSS_INSTALL_LOC"
chmod +rx "$DGOSS_INSTALL_LOC"
echo "dgoss $WRAPPER_REF has been installed to $DGOSS_INSTALL_LOC (compatibility shim)"
}
