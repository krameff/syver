#!/bin/sh

{
set -e

LATEST_URL="https://github.com/krameff/syver/releases/latest"
LATEST_EFFECTIVE=$(curl -s -L -o /dev/null ${LATEST_URL} -w '%{url_effective}')
LATEST=${LATEST_EFFECTIVE##*/}

DGOSS_VER=$GOSS_VER

if [ -z "$GOSS_VER" ]; then
    GOSS_VER=${GOSS_VER:-$LATEST}
    # This repo's default branch is `main`. It was `master` upstream, and that
    # stale value made the wrapper download below 404 for anyone who had not
    # pinned GOSS_VER.
    DGOSS_VER='main'
fi
if [ -z "$GOSS_VER" ]; then
    echo "ERROR: Could not automatically detect latest version, set GOSS_VER env var and re-run"
    exit 1
fi
GOSS_DST=${GOSS_DST:-/usr/local/bin}
INSTALL_LOC="${GOSS_DST%/}/syver"
DSYVER_INSTALL_LOC="${GOSS_DST%/}/dsyver"
DGOSS_INSTALL_LOC="${GOSS_DST%/}/dgoss"
touch "$INSTALL_LOC" || { echo "ERROR: Cannot write to $GOSS_DST set GOSS_DST elsewhere or use sudo"; exit 1; }

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

url="https://github.com/krameff/syver/releases/download/$GOSS_VER/syver-linux-$arch"

echo "Downloading $url"
curl -L "$url" -o "$INSTALL_LOC"
chmod +x "$INSTALL_LOC"
echo "Syver $GOSS_VER has been installed to $INSTALL_LOC"
echo "syver --version"
"$INSTALL_LOC" --version

dsyver_url="https://raw.githubusercontent.com/krameff/syver/$DGOSS_VER/extras/dgoss/dsyver"
echo "Downloading $dsyver_url"
curl -L "$dsyver_url" -o "$DSYVER_INSTALL_LOC"
chmod +rx "$DSYVER_INSTALL_LOC"
echo "dsyver $DGOSS_VER has been installed to $DSYVER_INSTALL_LOC"

# The goss-named wrapper is still shipped as a working shim for one major
# version, matching the compatibility promise the rest of the project makes.
dgoss_url="https://raw.githubusercontent.com/krameff/syver/$DGOSS_VER/extras/dgoss/dgoss"
echo "Downloading $dgoss_url"
curl -L "$dgoss_url" -o "$DGOSS_INSTALL_LOC"
chmod +rx "$DGOSS_INSTALL_LOC"
echo "dgoss $DGOSS_VER has been installed to $DGOSS_INSTALL_LOC (compatibility shim)"
}
