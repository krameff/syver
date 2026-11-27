# Syver: server, container and AI host validation in YAML

<!-- markdownlint-disable no-inline-html -->
<p align="center">
  <img src="images/syver-logo.svg" alt="Syver logo: SYstem VERification, by Krameff Solutions Ltd" width="280">
</p>
<!-- markdownlint-enable no-inline-html -->

[![Golang ci](https://github.com/krameff/syver/actions/workflows/golangci.yaml/badge.svg?branch=main)](https://github.com/krameff/syver/actions/workflows/golangci.yaml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/v/release/krameff/syver)](https://github.com/krameff/syver/releases/latest)
[![Documentation](https://img.shields.io/badge/docs-syver.readthedocs.io-blue)](https://syver.readthedocs.io/)

<!-- --8<-- [start:intro] -->

Syver (SYstem VERification) runs pass or fail checks against servers, containers and AI tool hosts. Describe the
state you expect in YAML, run it, and get a result you can put in CI, an image build or an audit pack. It is a
YAML-based alternative to serverspec, and the continuation of the krameff fork of goss: a single static Go binary
for Linux, with alpha support for macOS and Windows.

Syver checks that the machines and sandboxes running your AI tools are set up the way you think they are. It does
not secure models, prompts or agent behaviour.

<!-- --8<-- [end:intro] -->

![Syver demo: a file permission check fails with exit code 1, then passes with exit code 0 after chmod](docs/images/syver-demo.gif)

See it running: the [demo videos](https://krameff.com/syver.html#demos) show `autoadd` catching drift, `dsyver`
testing a Docker image and `dcsyver` checking a Docker Compose service.

**[Read the documentation](https://syver.readthedocs.io/)** for the full spec reference, every command and flag,
and platform notes.

## What it checks

* Ports and whether they listen on the right address
* Files, including their contents, owners and permissions
* Processes, services, users and groups
* Installed packages, and package baselines
* Kernel parameters and mounts
* Whether the host is in FIPS mode, and whether Syver itself is (see [FIPS 140-3](#fips-140-3))
* Commands and their output
* HTTP endpoints, DNS records, addresses and network interfaces
* Windows registry keys and values

## What it does not check

* Model weights, prompts or what an agent does at runtime
* Application code or known vulnerabilities in software
* Anything outside the host or sandbox where Syver runs
* Hosts you have not given it access to

Say what you need checked, and Syver tells you whether it is true. It does not decide what is safe for you.

## Quick start

Install the release binary on Linux or macOS. The script checks the download against the release's checksum file:

```bash
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/install.sh | sh
```

On Windows, download `syver-windows-amd64.exe` from the
[releases page](https://github.com/krameff/syver/releases/latest). Other options, including building from source,
are in [installation](docs/installation.md).

Write a spec, then validate:

```yaml
# syver.yaml
port:
  tcp:22:
    listening: true
```

```bash
syver -g syver.yaml validate
```

Exit code 0 means every check passed. Exit code 1 means at least one failed, which makes it easy to use as a CI
gate.

Run a spec once, retry it until it passes or times out, or serve it as a health endpoint:

```bash
syver -g syver.yaml validate --retry-timeout 60s   # wait for a service to come up
syver -g syver.yaml serve                          # results at http://localhost:8080/healthz
syver -g syver.yaml serve --tls-cert server.crt --tls-key server.key   # the same over HTTPS
```

Don't want to write a spec by hand? `syver autoadd sshd` writes one from the current state of the host.

Output formats: rspecish (the default), documentation, JSON, structured JSON, JUnit, TAP, Prometheus, Nagios, and
silent for health checks that should not expose system details.

## Containers, Compose and Kubernetes

Wrapper scripts run a spec against a container image ([dsyver](extras/dsyver/README.md)), a Docker Compose service
([dcsyver](extras/dcsyver/README.md)), a Kubernetes pod ([ksyver](extras/ksyver/README.md)) or a Docker Sandboxes
microVM ([sbxsyver](extras/sbxsyver/README.md)). The goss-named `dgoss`, `dcgoss` and `kgoss` still ship alongside
them and work unchanged. Syver is also published as a [container image](docs/container_image.md).

## Coming from goss?

Your gossfiles do not need to change. `gossfile:`, `goss.yaml`, `GOSS_*` environment variables, the `-g` flag and
the `dgoss`, `dcgoss` and `kgoss` wrappers all keep working. [goss vs Syver](docs/goss-vs-syver.md) lists what
differs and the upgrade steps.

## Documentation

Full documentation is at [syver.readthedocs.io](https://syver.readthedocs.io/), built from `docs/` in this
repository. It tracks the latest release.

* [Quick start](docs/quickstart.md): write and run your first spec
* [Spec reference](docs/gossfile.md): every resource and attribute, templates and variables
* [Command line](docs/cli.md): `validate`, `serve`, `add`, `autoadd`, `render` and their flags
* [Installation](docs/installation.md): install, verify signatures, FIPS 140-3 mode
* [Coming from goss](docs/goss-vs-syver.md): what differs, and the upgrade steps
* [Platform support](docs/platforms.md): Linux, macOS and Windows
* [Releases](RELEASES.md) and [changelog](CHANGELOG.md)

## AI tool hosts

### Agent sandboxes (Docker Sandboxes, sbx)

Docker Sandboxes run coding agents without approval prompts by default, so the sandbox boundary is the main
control. `sbxsyver` runs a spec inside a sandbox from the host, and checks that:

* the checks are running inside the sandbox and the tools the agent needs are present
* the workspace is mounted and writable, and the Docker daemon is the sandbox's own
* API keys hold only placeholders, and cloud and source-control tokens are not set
* no SSH, cloud or Git credential directories were mounted in
* the agent can reach the model API and GitHub, and cannot reach the cloud metadata address or your internal hosts

```bash
sbx create --name my-sandbox claude ~/src/my-project
cd examples/agent-sandbox-sbx    # after editing vars.yaml
SYVER_VARS=vars.yaml sbxsyver run my-sandbox
```

A pass shows the sandbox was configured safely when the check ran. Run the check after the agent has started and a
pass proves much less, because the agent can change the sandbox.

The profile is in [`examples/agent-sandbox-sbx/`](examples/agent-sandbox-sbx/), and
[what it cannot see](extras/sbxsyver/README.md#what-it-cannot-see) is listed in the sbxsyver docs.

## Platform support

Syver works well on Linux. Support on Windows and macOS is alpha, and some resources behave differently there: see
[platform support](docs/platforms.md). Release binaries cover Linux (amd64, arm64, s390x, ppc64le), macOS (amd64,
arm64) and Windows (amd64).

## Signed releases

Each release is signed, and ships with a signed SBOM (software bill of materials), so you can confirm what you are
running and who released it.

```bash
SYVER_VER=v0.22.0   # the release you downloaded
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/krameff-syver-key.asc | gpg --import
# download syver_<version>_SHA256SUMS and its .sig from the release page, then:
gpg --verify syver_${SYVER_VER#v}_SHA256SUMS.sig syver_${SYVER_VER#v}_SHA256SUMS
sha256sum --ignore-missing -c syver_${SYVER_VER#v}_SHA256SUMS
```

gpg in FIPS mode rejects the Ed25519 key. On those hosts, import `krameff-syver-key-rsa.asc` and verify the
`.rsa.sig` file instead. Key fingerprints are in [RELEASES.md](RELEASES.md).

## FIPS 140-3

Syver is pure Go, and its cryptography comes from Go's FIPS 140-3 module. `syver --version` says, on its third
line, whether Syver is running in FIPS 140-3 mode and which module version it was built with:

```console
$ syver --version
syver version 0.22.0
Krameff Solutions Ltd
fips140: mode=off module=none
```

Turn the mode on for a run with `GODEBUG=fips140=on`. A host in FIPS mode does not switch Syver into FIPS mode, so
check the host as part of your spec:

```yaml
file:
  /proc/sys/crypto/fips_enabled:
    exists: true
    contents: ["/^1$/"]
```

To find checks that use algorithms FIPS 140-3 does not approve, run with `GODEBUG=fips140=only`: an `md5:` check
then fails with a message pointing to `sha256`, and the file's other checks still run.

Details: [FIPS 140-3 mode](docs/installation.md#fips-140-3-mode).

## Background

Syver is a fork of [goss](https://github.com/goss-org/goss), the YAML-based server validation tool created by
[@aelsabbahy](https://github.com/aelsabbahy), and has diverged from upstream. Syver releases are signed, ship a
signed SBOM, and take fixes on their own schedule. The gossfile format stays compatible on purpose. Full credit goes
to the goss authors for the foundation this builds on.

Syver is licensed under Apache-2.0. See [LICENSE](LICENSE).

## Contributing

Issues and pull requests are welcome: see [CONTRIBUTING.md](.github/CONTRIBUTING.md). Profiles for other AI tools
are especially welcome. Each profile needs a spec, a vars file with an example, a short docs page that says what it
checks and what it does not, and an integration test that deliberately misconfigures a test host and expects a
failure.

## Maintainers

Built and maintained by [Krameff Solutions](https://krameff.com).
