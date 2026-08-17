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
