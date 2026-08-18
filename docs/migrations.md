# Migration guide

> For a side-by-side reference of exactly what changed between goss and Syver, plus the
> longer list of what deliberately did not, see [goss vs Syver](goss-vs-syver.md).

## Coming from `goss-org/goss`

This project is a fork of [`goss-org/goss`](https://github.com/goss-org/goss), now
maintained at `github.com/krameff/syver`. **Your gossfiles don't need to change.**
Syntax, resource types, matchers and CLI flags are all identical. If it worked
before, it still works.

### What you need to change

| If you | Change to |
| --- | --- |
| Install manually or by script | The [`krameff/syver` releases page](https://github.com/krameff/syver/releases) and this repo's [`install.sh`](https://github.com/krameff/syver/blob/main/install.sh). See [Installation](installation.md) |
| Use the container image | `ghcr.io/krameff/syver`, replacing the old `aelsabbahy` / `goss-org` image |
| Import syver as a Go library | `github.com/krameff/syver` |

That's the whole list. Everything else carries over as-is.

### What this fork adds

None of these are required. Leave them out and nothing changes.

| Addition | What it does |
| --- | --- |
| [discovery](gossfile.md#discovery) | Runs lightweight checks before the main suite and feeds results into templates |
| [`depends-on`](gossfile.md#test-dependencies) | Declares test prerequisites, so dependents are skipped rather than failed |
| [`process.status`](gossfile.md#process) | Catches zombie processes |
| [`process.user`](gossfile.md#process) | Catches something running as root that shouldn't be |
| [`port.pid`](gossfile.md#port) | Identifies which process owns a listening socket |

Under the hood, process and port lookups moved from two unmaintained libraries
(`goss-org/go-ps` and `goss-org/GOnetstat`) to the actively maintained `gopsutil`.
That should be invisible to you, gossfiles and output are unchanged, but it is why
the three resource fields above were straightforward to add.

## Upgrading from krameff/goss v0.6.0

If you are already on the `krameff/goss` fork, this is the rename release. Your
gossfiles need no changes; syntax, resource types and matchers are identical.
What changes is the product's own name.

### What you need to do

| If you | Do this |
| --- | --- |
| Invoke `goss` by name in scripts or CI | Call `syver`, or symlink it (below) |
| Use the community integrations | Symlink, see below |
| Pull the container image | Use `ghcr.io/krameff/syver` |
| Import this as a Go library | Update the path to `github.com/krameff/syver` |
| Verify release checksums by filename | It is now `syver_<version>_SHA256SUMS` |
| Nothing above | Nothing. Install the new binary and carry on |

The third-party integrations listed in the README (`goss-ansible`,
`kitchen-goss`, `packer-provisioner-goss` and the rest) all invoke a binary
named `goss`, and `install.sh` installs `syver`, `dsyver` and `dgoss` but no
`goss`. One symlink covers all of them:

```bash
sudo ln -s "$(command -v syver)" /usr/local/bin/goss
```

### What you do NOT need to do

`gossfile:` is still the canonical import key, `goss.yaml` / `goss.yml` are
still accepted filenames, all 16 `GOSS_*` environment variables still work,
`--gossfile` / `-g` still work, and `dgoss` / `dcgoss` / `kgoss` still ship and
run. The JUnit suite name, the Nagios prefix, the `goss_tests_*` metrics, the
`application/vnd.goss-*` media types and the `goss-<os>-<arch>` release
archives are all unchanged.

See [goss vs Syver](goss-vs-syver.md) for the complete side-by-side.

### Verifying releases

Upstream `goss-org/goss` publishes no signatures. This fork GPG-signs its
release checksums, so verification is available here that was not available
upstream.

The key for 0.7.0 onward is `krameff-syver-key.asc`, fingerprint
`CD218D529C95DC65A71F18D84C9E5095CABE5092`. If you imported the earlier
`krameff-goss-key.asc` from a 0.6.0 release, import the new one as well: the
old key will not verify 0.7.0 artifacts. See
[Verifying release signatures](installation.md#verifying-release-signatures).

## v4 migration

Three breaking changes when moving from v0.3.x to v0.4.x. Check whether any apply
before reading the detail below.

| Change | Affects you if |
| --- | --- |
| Array matchers reject duplicates | You repeat the same value in an array, e.g. `user.groups` |
| `rpm` reports the full EVR version | You pin exact rpm version strings |
| `file.contains` renamed to `file.contents` | You use `file.contains` |

### Array matchers (e.g. user.groups) no longer allows duplicates

Goss v0.3.X allowed:

```yaml
user:
  root:
    exists: true
    groups:
      - root
      - root
      - root
```

Goss v0.4.x, will fail with the above as group "root" is only in the slice once. However, with goss v0.4.x the array may
contain matchers. The test below is valid for v0.4.x but not valid for v0.3.x

```yaml
user:
  root:
    exists: true
    groups:
      - have-prefix: r
```

## rpm now contains the full EVR version

To enable the ability to compare RPM versions in the future, The version matching of rpm has changed

from:

```console
rpm -q --nosignature --nohdrchk --nodigest --qf '%{VERSION}\n' package_name
```

to:

```console
rpm -q --nosignature --nohdrchk --nodigest --qf '%|EPOCH?{%{EPOCH}:}:{}|%{VERSION}-%{RELEASE}\n' package_name
```

## `file.contains` -> `file.contents`

File contains attribute has been renamed to file.contents

from:

```yaml
file:
  /tmp/foo:
    exists: true
    contains: []
```

to:

```yaml
file:
  /tmp/foo:
    exists: true
    contents: []
```
