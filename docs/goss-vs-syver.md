# goss vs Syver

Syver is the renamed continuation of the `krameff/goss` fork of
[`goss-org/goss`](https://github.com/goss-org/goss). **The product renamed; the file
format did not.** Existing gossfiles, env vars, wrapper scripts and CI pipelines keep
working. There is exactly one intentional breaking change, called out below.

For a step-by-step move from upstream goss, see [migrations](migrations.md). This page
is the quick reference for what is and isn't different.

---

## The one breaking change

| Behaviour | goss | Syver |
| --- | --- | --- |
| Outbound `User-Agent` on `http` resource checks | `goss/<version>` | `syver/<version>` |

If you assert on the User-Agent string server-side, update that assertion. Nothing else
on this page is a hard break.

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
