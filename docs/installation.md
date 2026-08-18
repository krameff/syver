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

## dgoss and other wrappers

* [dgoss](https://github.com/krameff/syver/blob/main/extras/dsyver/README.md) — run goss against Docker/Podman containers
* [kgoss](https://github.com/krameff/syver/blob/main/extras/ksyver/README.md) — Kubernetes wrapper
* [dcgoss](https://github.com/krameff/syver/blob/main/extras/dcsyver/README.md) — Docker Compose wrapper

## Release binaries

The supported install path is:

```bash
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/install.sh | sh
```

Release assets are raw, uncompressed binaries named `syver-<os>-<arch>`
(for example `syver-linux-amd64`; Windows builds are named `syver-windows-amd64.exe`).
Legacy `goss-<os>-<arch>` assets are published alongside them for one major
version, so existing download URLs keep resolving.

To install manually from a GitHub release:

```bash
SYVER_VER=v0.5.0
curl -L "https://github.com/krameff/syver/releases/download/${SYVER_VER}/syver-linux-amd64" \
  -o /tmp/syver
sudo mv /tmp/syver /usr/local/bin/syver
chmod +rx /usr/local/bin/syver
```

Adjust the version, OS, and architecture in the filename as needed (`amd64`,
`arm64`, `arm`, `s390x`, `386`, etc.).

When release artifacts are published for this fork, download the matching archive
from the repository **Releases** page. Until then, use [build from source](#build-from-source) above.

## Verifying release signatures

Each release's `SHA256SUMS` checksum file is GPG-signed with the project's
signing key (fingerprint `326F2A906EBB641DF88929D0306DF3B80A0667CD`, published
as [`krameff-syver-key.asc`](https://github.com/krameff/syver/blob/main/krameff-syver-key.asc) at the repo root and
attached to every release).

```bash
SYVER_VER=v0.5.0

# import the signing key once
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/krameff-syver-key.asc | gpg --import

# download the checksum file and its signature from the release page, then:
gpg --verify syver_${SYVER_VER#v}_SHA256SUMS.sig syver_${SYVER_VER#v}_SHA256SUMS
sha256sum -c syver_${SYVER_VER#v}_SHA256SUMS
```

A `gpg --verify` output of `Good signature from "Krameff Solutions Limited..."`
confirms the checksum file (and therefore every archive listed in it) came
from this project and hasn't been tampered with.
