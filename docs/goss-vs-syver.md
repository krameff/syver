# goss vs Syver

Syver is the renamed continuation of the `krameff/goss` fork of
[`goss-org/goss`](https://github.com/goss-org/goss). **The product renamed; the file
format did not.** Existing gossfiles, env vars, wrapper scripts and CI pipelines keep
working. The breaking changes are limited to things that referenced the product by
name, and are listed first.

For a step-by-step move from upstream goss, see [migrations](migrations.md). This page
is the quick reference for what is and isn't different.

---

## Breaking changes

Nothing that reads or writes a spec file changes. Almost everything that breaks is
something that referenced the product by name; the exception is the Go library API,
which changed to carry a `context.Context` (last two rows).

| What | goss | Syver | Affects you if |
| --- | --- | --- | --- |
| Binary name | `goss` | `syver` | Anything shells out to `goss` by name. `install.sh` no longer puts a `goss` on your `PATH` |
| Outbound `User-Agent` on `http` checks | `goss/<version>` | `syver/<version>` | You assert on the User-Agent server-side |
| Checksum file | `goss_<ver>_SHA256SUMS` | `syver_<ver>_SHA256SUMS` | You verify release checksums by filename |
| Container image | `ghcr.io/krameff/goss` | `ghcr.io/krameff/syver` | You pull the image |
| Go module path | `github.com/krameff/goss` | `github.com/krameff/syver` | You import this as a library, not as a CLI |
| `Resource` interface | `Validate(sys)` | `Validate(ctx, sys)` | You implement your own resource type against the library |
| Library entry points | `Validate(c)`, `ValidateResults(c)`, `ValidateConfig(c, cfg)`, `Serve(c)` | same, each taking `ctx` first | You call these directly instead of using the CLI |

The release archives themselves are still published under **both** names, so a
`goss-<os>-<arch>` download URL keeps resolving. Only the checksum file is single-named.

For the binary-name break specifically, the fix is one line:

```bash
sudo ln -s "$(command -v syver)" /usr/local/bin/goss
```

See [migrations](migrations.md#upgrading-from-krameffgoss-v060) for the full upgrade path.

---

## Nothing to change

These are the compatibility guarantees. If your setup relies on any of them, it keeps working.

| What | Still true in Syver | Notes |
| --- | --- | --- |
| Config file key | `gossfile:` | Still canonical, still what gets written |
| `goss.yaml` / `goss.yml` filenames | Accepted | Probed alongside the syver names |
| `GOSS_*` environment variables | All 16 still honoured | Permanent, not deprecated |
| `--gossfile` / `-g` flag | Still accepted | Demoted to an alias, not removed |
| `dgoss` / `dcgoss` / `kgoss` wrappers | Still shipped and working | Kept for one major version |
| JUnit suite name | `goss` | Deliberately unchanged |
| Nagios output prefix | `GOSS OK` / `GOSS CRITICAL` | Deliberately unchanged |
| `goss_tests_*` Prometheus metrics | Still emitted | Syver metrics emit alongside, not instead |
| `application/vnd.goss-*` Accept headers | Still accepted | No vendor header still yields `vnd.goss-` |
| `goss-<os>-<arch>` release artifact | Still published | Legacy archive kept for one major version |
| gossfile syntax, resource types, matchers | Unchanged | No spec rewrite needed |

---

## Behaviour differences

One thing Syver does that goss does not: **Ctrl-C stops work in progress.** goss
mints a fresh `context.Background()` inside each check, so interrupting a run
leaves any command it had already started to run to completion, orphaned. Syver
threads the signal handler's context all the way down to `exec`, so the child is
killed with the parent. `syver serve` shuts down in an orderly way on SIGTERM
rather than being killed mid-request, and a second signal always terminates the
process outright, so nothing becomes unkillable.

This is a difference, not a compatibility break: no spec file behaves differently,
and a run that is allowed to finish produces identical results either way.

## Product identity

| Item | goss | Syver |
| --- | --- | --- |
| Binary name | `goss` | `syver` |
| Go module path | `github.com/krameff/goss` | `github.com/krameff/syver` |
| Repository | `github.com/krameff/goss` | `github.com/krameff/syver` |
| Container image | `ghcr.io/krameff/goss` | `ghcr.io/krameff/syver` |
| Release artifacts | `goss-<os>-<arch>` | `syver-<os>-<arch>`, legacy name also published |
| Checksum file | `goss_<ver>_SHA256SUMS` | `syver_<ver>_SHA256SUMS`, no legacy twin |
| Container volume | `/goss` | `/syver`, and `/goss` is still declared |

---

## Config files

| Behaviour | goss | Syver |
| --- | --- | --- |
| Canonical key written on save | `gossfile:` | `gossfile:`, unchanged |
| Accepted input key | `gossfile:` | `gossfile:` or `syverfile:` |
| Is `syverfile:` ever written out? | n/a | No, input alias only, folded in at decode |
| Both keys present in one file | n/a | `gossfile:` wins, logs one `WARN` |
| Spec filenames probed | `goss.yaml`, `goss.yml` | `syver.yaml`, `syver.yml`, `goss.yaml`, `goss.yml` |
| Where `add` writes | The gossfile | Whichever file the read resolved to |

---

## CLI

| Item | goss | Syver |
| --- | --- | --- |
| Primary spec-file flag | `--gossfile` | `--syverfile`, with `--gossfile` and `-g` as aliases |
| Default spec path | Static `./goss.yaml` | Resolved dynamically across all four names |
| Nested add subcommand | `add goss` | `add syver`, and `add goss` still works |
| Subcommands, flags, matchers | n/a | Identical |

---

## Environment variables

Every variable is paired. `SYVER_*` takes priority when set to a non-empty value;
otherwise `GOSS_*` applies. An exported-but-empty `SYVER_*` is treated as unset and
never shadows a real `GOSS_*`.

| Item | goss | Syver |
| --- | --- | --- |
| Recognised prefixes | `GOSS_*` only | `SYVER_*` and `GOSS_*` |
| Precedence | n/a | `SYVER_*` first, `GOSS_*` fallback |
| Number paired | n/a | All 16, e.g. `_FILE`, `_VARS`, `_FMT`, `_LOGLEVEL`, `_SLEEP` |
| Wrapper script variables | `GOSS_*` only | Both, same precedence rule |
| `install.sh` variables | `GOSS_VER`, `GOSS_DST` | `SYVER_VER`, `SYVER_DST`, with the `GOSS_*` names still honoured |

---

## Wrapper scripts

| Item | goss | Syver |
| --- | --- | --- |
| Docker wrapper | `dgoss` | `dsyver`, with `dgoss` kept as a working shim |
| Compose wrapper | `dcgoss` | `dcsyver`, with `dcgoss` kept as a working shim |
| Kubernetes wrapper | `kgoss` | `ksyver`, with `kgoss` kept as a working shim |
| Binary discovery | `which goss` | `which syver`, falling back to `which goss` |
| Spec staged into container | `goss.yaml` | First of the four probed names that exists |

---

## Output and wire formats

| Format | goss | Syver |
| --- | --- | --- |
| Prometheus metric names | `goss_tests_*` | `goss_tests_*` and `syver_tests_*`, same labels and values |
| Accept header vendor prefix | `vnd.goss-` | Both prefixes accepted, the client's own is echoed back |
| Accept header absent | `vnd.goss-` | `vnd.goss-`, since no preference was expressed |
| `/metrics` endpoint | Served an empty registry, a bug | Serves the real metrics |
| JUnit, Nagios, TAP, JSON | n/a | Unchanged |

---

## Test matrix

| Distro | goss | Syver |
| --- | --- | --- |
| CentOS 7 | Present | Removed, EOL and its systemd never activates services in a container |
| almalinux10, alpine3, arch, bullseye, jammy, rockylinux9 | Present | Unchanged |

---

## Keeping this page current

This page is a living record of the rename and should be updated in the same commit as
any change that alters it. Update it when you:

* add or remove a `SYVER_*` / `GOSS_*` variable pair
* change which spec filenames are probed, or which key is canonical
* rename or retire a wrapper script, flag, or subcommand alias
* change a metric name, media type, or output-format identifier
* drop a compatibility shim, moving a row out of *Nothing to change*

The *Nothing to change* table is the promise this project makes to existing goss users.
Moving a row out of it is a breaking change and belongs in the changelog, not just here.
