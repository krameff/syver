# Installation

For macOS and Windows platform notes, see [platforms](platforms.md).

## Build from source

```bash
make build
```

Platform binaries are written under `release/` (for example `release/syver-linux-amd64`).

Install a built binary on your `PATH`:

```bash
cp release/syver-linux-amd64 /usr/local/bin/syver   # adjust OS/arch as needed
chmod +rx /usr/local/bin/syver
```

Build a single platform target:

```bash
make release/syver-linux-amd64
```

Alternatively, build with [GoReleaser](https://goreleaser.com/):

```bash
goreleaser build --clean --single-target --snapshot
```

The binary is written under `dist/`, in a directory named for the build id in
`.goreleaser.yaml` -- currently `binaries`, so for example
`dist/binaries_linux_amd64_v1/syver`.

## dsyver and other wrappers

* [dsyver](https://github.com/krameff/syver/blob/main/extras/dsyver/README.md): run syver against Docker/Podman containers
* [ksyver](https://github.com/krameff/syver/blob/main/extras/ksyver/README.md): run syver against a Kubernetes pod
* [dcsyver](https://github.com/krameff/syver/blob/main/extras/dcsyver/README.md): run syver against a Docker Compose service
* [sbxsyver](https://github.com/krameff/syver/blob/main/extras/sbxsyver/README.md): run syver inside a Docker Sandbox (`sbx`)

The goss-named `dgoss`, `kgoss` and `dcgoss` scripts still ship alongside
these as working shims, under the
[compatibility policy](goss-vs-syver.md#compatibility-policy).

## Release binaries

The supported install path is:

```bash
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/install.sh | sh
```

It runs on Linux and macOS and installs the binary for your OS and
architecture, along with the `dsyver` and `dgoss` wrappers. On macOS the
wrappers need a separate Linux binary to copy into the container; the script
prints the commands, and the [dsyver README](https://github.com/krameff/syver/blob/main/extras/dsyver/README.md#mac-osx)
explains why. On Windows, download the binary from the releases page.

The script checks the downloaded binary against the release's
`syver_<version>_SHA256SUMS` file and stops without installing anything if the
checksum does not match, the binary is not listed, or the file cannot be
downloaded. Set `SYVER_SKIP_CHECKSUM=1` to install without the check. The
`dsyver` and `dgoss` wrappers come from the source tree rather than the
release, so the checksum file does not cover them. Each file is moved into
`SYVER_DST` only once it has downloaded completely, so a failed download never
leaves a partial file behind. If the binary fails to download or verify, any
existing `syver` is left in place; the binary is installed before the
wrappers, so if a wrapper download fails, `syver` has already been updated,
and that wrapper and any after it keep their previous versions, or are not
installed if there were none.

Release assets are raw, uncompressed binaries named `syver-<os>-<arch>`
(for example `syver-linux-amd64`; Windows builds are named `syver-windows-amd64.exe`).
The goss-named binaries were last published with v0.9.1.

To install manually from a GitHub release:

```bash
SYVER_VER=v0.15.0   # any tag from the Releases page
curl -fL "https://github.com/krameff/syver/releases/download/${SYVER_VER}/syver-linux-amd64" \
  -o /tmp/syver
sudo mv /tmp/syver /usr/local/bin/syver
chmod +rx /usr/local/bin/syver
```

Adjust the version, OS, and architecture in the filename as needed. Release
binaries are 64-bit only: `linux` has `amd64`, `arm64`, `s390x` and `ppc64le`, `darwin`
has `amd64` and `arm64`, and `windows` has `amd64`. On a 32-bit system,
[build from source](#build-from-source).

## Verifying release signatures

Each release's `SHA256SUMS` checksum file is GPG-signed with the project's
signing key (fingerprint `CD218D529C95DC65A71F18D84C9E5095CABE5092`, published
as [`krameff-syver-key.asc`](https://github.com/krameff/syver/blob/main/krameff-syver-key.asc) at the repo root and
attached to every release).

From 0.20.0 each release is also signed with an RSA-4096 key (fingerprint
`AC1C7F6FEC28F09F8DCF352DDBEADDD13E62F866`, published as
[`krameff-syver-key-rsa.asc`](https://github.com/krameff/syver/blob/main/krameff-syver-key-rsa.asc)).
Its signatures are in separate `.rsa.sig` files. Use it where gpg runs in
FIPS mode, which rejects the Ed25519 key; anywhere else, either key will do.

```bash
SYVER_VER=v0.15.0   # the release you downloaded

# import the signing key once
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/krameff-syver-key.asc | gpg --import

# download the checksum file and its signature from the release page, then:
gpg --verify syver_${SYVER_VER#v}_SHA256SUMS.sig syver_${SYVER_VER#v}_SHA256SUMS
# --ignore-missing: the file lists every release asset, not only the one you downloaded
sha256sum --ignore-missing -c syver_${SYVER_VER#v}_SHA256SUMS
```

On a FIPS-mode host, import the RSA key and verify the `.rsa.sig` file
instead:

```bash
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/krameff-syver-key-rsa.asc | gpg --import
gpg --verify syver_${SYVER_VER#v}_SHA256SUMS.rsa.sig syver_${SYVER_VER#v}_SHA256SUMS
```

A `gpg --verify` output of `Good signature from "Krameff Solutions Limited..."`
confirms the checksum file (and therefore every archive listed in it) came
from this project and hasn't been tampered with.
