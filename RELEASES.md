# Release History

A version-controlled record of every Syver release: what it was tagged, what
commit it points at, and what state the gate was in when it shipped.

This is deliberately **not** a second changelog. [CHANGELOG.md](CHANGELOG.md)
says what changed for users; this file says what was released, when, and from
where, so a version in the wild can always be traced back to a commit. Each
entry links to its changelog section rather than repeating it.

Newest first. Syver continues goss's version numbering rather than restarting
at 1.0, so that `v0.6.0` means the same lineage point in both projects.

"Released" is the date the tag object was created, which is not always the
commit date: v0.7.0 was committed on 2026-08-18 and tagged on 2026-08-20.
v0.6.0 has no tag in this repository and uses the date its changelog entry
records.

## Contents

* [Unreleased](#unreleased)
* [v0.9.0 - Correctness and shutdown fixes](#v090---correctness-and-shutdown-fixes)
* [v0.8.0 - Registry-driven dispatch](#v080---registry-driven-dispatch)
* [v0.7.0 - Rename to Syver](#v070---rename-to-syver)
* [v0.6.0 - Upstream baseline (krameff/goss)](#v060---upstream-baseline-krameffgoss)
* [Lineage](#lineage)

---

## Unreleased

| Field | Value |
| --- | --- |
| Branch | `feat/yaml-fork` |
| Base | krameff/goss v0.6.0 |

Both yaml dependencies moved to `go.yaml.in/yaml`, the maintained fork.
`gopkg.in/yaml.v2` and `v3` were archived together, so v3 was never the
supported option. Marshal output is byte-identical, verified against all 204
goldens.

Not purely a no-op on the decode side: the newer yaml v3 converts an
unrecovered panic into a normal parse error for a gossfile that uses `<<:` in
the same mapping as a complex key (a list or map used as a key). Such a file
previously crashed Syver with a stack trace. This is a startup-path fix, not a
request-triggered one, and the exit code for that input changes from 2 to 1
(render, serve) or 78 (validate).

**Breaking:** none.

---

## v0.9.0 - Correctness and shutdown fixes

| Field | Value |
| --- | --- |
| Released | 2026-08-23 |
| Tag | `v0.9.0` |
| Commit | `42f470f` |
| Base | krameff/goss v0.6.0 |
| Branch | `feat/aug_issues` |
| Scope | 18 commits, 75 files, +2313 / -294 |
| Changelog | [0.9.0](CHANGELOG.md#090-based-on-krameffgoss-v060---correctness-and-shutdown-fixes) |

Correctness release. Fixes two crashes reachable from a single unauthenticated
request to `syver serve`, and several cases where Syver reported success when it
should not have. Gossfiles need no changes.

**Breaking:**

* `--format structured` now exits non-zero when checks fail. It always exited 0,
  so a failing run reported success to any CI step consuming it.
* `/healthz` takes its status from the results rather than the output format's
  exit code, so no `Accept` header can report a failing host as healthy.
* Library API: `Validate` and the top-level entry points take a
  `context.Context` as their first argument.

**Gate at release:** 474 tests / 7 packages `-race` clean; 204/204 goldens
byte-identical; `make check` clean; Docker suite green on all six distros with
its three hardcoded per-distro counts (106/127/126) unchanged.

---

## v0.8.0 - Registry-driven dispatch

| Field | Value |
| --- | --- |
| Released | 2026-08-22 |
| Tag | `v0.8.0` |
| Commit | `11629a8` |
| Base | krameff/goss v0.6.0 |
| Branch | `modularization` |
| Scope | 8 commits, 40 files, +2487 / -2377 |
| Changelog | [0.8.0](CHANGELOG.md#080-based-on-krameffgoss-v060---registry-driven-dispatch) |

FEAT-007, Modularisation Phase 1. Adding a resource type went from 19 edit sites
across 6 files to one `Register(Descriptor{...})` call plus one dispatch entry.
Deleted 1,629 generated lines and the `genny` dependency in favour of one
generic `ResourceMap`. Verified no-op refactor.

**Breaking:** one deliberate behaviour change. `Matching.SetSkip()` was a no-op,
so `util.WithDisabledResourceTypes("matching")` silently failed to disable it.
No CLI flag reaches it, so no golden changed.

**Gate at release:** test floor 339 -> 365 cases, 7 packages, `-race` clean;
204/204 pre-refactor goldens byte-identical, which is the no-op proof;
`make check` clean; Docker suite 7/7 with per-distro counts unchanged.

---

## v0.7.0 - Rename to Syver

| Field | Value |
| --- | --- |
| Released | 2026-08-20 |
| Tag | `v0.7.0` |
| Commit | `146b606` |
| Base | krameff/goss v0.6.0 |
| Branch | `syver_initial` |
| Scope | 76 commits, 343 files, +5069 / -1974 |
| Changelog | [0.7.0](CHANGELOG.md#070-based-on-krameffgoss-v060---rename-to-syver) |

goss becomes Syver. The product renamed; the file format did not. `gossfile:`,
`goss.yaml`, the `GOSS_*` environment variables, `-g`, and the `dgoss` /
`dcgoss` / `kgoss` wrappers all keep working. `SYVER_*` variables and
`syver_tests_*` metrics are emitted alongside the goss-named originals.

Also fixed `/metrics` serving the default global registry instead of the
`outputs` private one.

**Breaking:** limited to things that referenced the product by name. The binary
is now `syver`; the Go module path is `github.com/krameff/syver`; the
User-Agent, checksum filename and container image renamed. A legacy
`goss-<os>-<arch>` archive is still published.

* Upgrading: [docs/migrations.md](docs/migrations.md#upgrading-from-krameffgoss-v060)
* Side-by-side: [docs/goss-vs-syver.md](docs/goss-vs-syver.md)

**Gate at release:** test floor 339 cases / 7 packages.

---

## v0.6.0 - Upstream baseline (krameff/goss)

| Field | Value |
| --- | --- |
| Released | 2026-07-26 |
| Commit | `6ef84cf` |
| Project | krameff/goss |

The seed. Syver was cloned from `krameff/goss` at this commit with 851 commits
of history preserved, and every Syver release to date is still "based on
krameff/goss v0.6.0".

`krameff/goss` is **frozen** at v0.6.0 and ships nothing further. Syver replaces
it rather than running alongside it.

Its changelog entries are retained below the Syver ones in
[CHANGELOG.md](CHANGELOG.md) for continuity.

---

## Lineage

```text
goss-org/goss
  └── krameff/goss          fork; v0.4.0, v0.5.0, v0.6.0
        └── krameff/syver   renamed continuation; v0.7.0 onward
```

Version numbering is continuous across the rename on purpose: Syver picks up at
v0.7.0 rather than restarting, so a version number identifies a single point in
one lineage. `krameff/goss` is frozen at v0.6.0, so the two never collide.

Every release so far is cut from the v0.6.0 baseline; upstream `goss-org/goss`
`devel` has never been merged, which is why `lint.go` and `lint/` are absent
here and that absence is correct.
