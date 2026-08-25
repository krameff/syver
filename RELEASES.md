# Release History

A version-controlled record of every Syver release: what it was tagged, what
commit it points at, and what state the gate was in when it shipped.

This is deliberately **not** a second changelog. [CHANGELOG.md](CHANGELOG.md)
says what changed for users; this file says what was released, when, and from
where, so a version in the wild can always be traced back to a commit. Each
entry links to its changelog section rather than repeating it.

Newest first. Syver continues goss's version numbering rather than restarting
at 1.0, so that `v0.6.0` means the same lineage point in both projects.

Releases are assembled on `devel` and merged to `main` at release time, so from
0.9.1 onward a release carries several branches rather than one.

"Released" is the date the tag object was created, which is not always the
commit date. v0.9.1 is the exception: it was tagged through the GitHub release
UI, which creates a lightweight tag with no tag object and no signature, so its
date is the commit date instead. v0.7.0 through v0.9.0 are annotated and
GPG-signed. v0.7.0 was committed on 2026-08-18 and tagged on 2026-08-20.
v0.6.0 has no tag in this repository and uses the date its changelog entry
records.

## Contents

* [v0.9.1 - Correctness fixes and CI gating](#v091---correctness-fixes-and-ci-gating)
* [v0.9.0 - Correctness and shutdown fixes](#v090---correctness-and-shutdown-fixes)
* [v0.8.0 - Registry-driven dispatch](#v080---registry-driven-dispatch)
* [v0.7.0 - Rename to Syver](#v070---rename-to-syver)
* [v0.6.0 - Upstream baseline (krameff/goss)](#v060---upstream-baseline-krameffgoss)
* [Lineage](#lineage)

---

## v0.9.1 - Correctness fixes and CI gating

| Field | Value |
| --- | --- |
| Released | 2026-08-25 |
| Tag | `v0.9.1` |
| Commit | `b18a8bd` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel` |
| Scope | 20 commits, 33 files, +1196 / -110 |
| Changelog | [0.9.1](CHANGELOG.md#091-based-on-krameffgoss-v060---correctness-fixes-and-ci-gating) |

The first release assembled on `devel` rather than straight to `main`, and the
first cut through a gated release path: signing and publishing now depend on a
job that runs the tests and the security scan, which a tag push previously
bypassed entirely.

Six branches, in merge order: `feat/yaml-fork`, `feat/trivy-gate`,
`feat/trivy-ignore-yaml`, `fix/timeout-message`, `fix/ci-branch-filter`,
`fix/release-path-gating`.

Correctness fixes, none of which change the CLI contract or the gossfile format:

* a malformed gossfile using `<<:` alongside a complex key reports a parse error
  instead of crashing with a stack trace
* a timed-out command names the budget it exceeded, and says the same thing on
  every host. It previously reported whichever of two racing timers won
* a negative `timeout:` warns and uses the default, instead of leaving the
  command completely unbounded
* a command that failed to start could report exit status 0, so a spec asserting
  `exit-status: 0` would have passed
* spec warnings appear once per process rather than once per check, which made
  them unreadable under `serve`

Also fixed a data race in the command timeout path, present since before v0.8.0,
which affected every timed-out command rather than only ones producing output.
It is narrowed rather than closed: a child that escapes the process group can
still outlive the wait. See FEAT-008.

### CI, in more detail than the changelog carries

The security scan reported findings and exited 0, so `make check` and
`make pre-push` passed with HIGH CVEs on screen and the suppression file governed
a report nothing acted on. Findings now fail the build. Because Trivy exits 1 for
its own errors, findings use exit 2, so a scan that never ran is never reported
as a clean bill.

`ci/trivyignore-check.sh` scanned without `--ignorefile`, and Trivy auto-loads
`.trivyignore` from the working directory, so it validated suppressions against a
scan those suppressions had already filtered. Every entry eventually read as "no
longer found", advising deletion of a live suppression.

Two filters silently matched nothing: the lint workflow's release-branch filter
was a regex where GitHub accepts only globs, and CodeQL ran on `devel` only.
Neither failed as a parse error, so both looked correct.

**Breaking:** none. The one behaviour change users may notice is that a negative
`timeout:` now warns; it was silently unbounded before.

**Gate at release:** 480 tests / 7 packages `-race` clean (up from 474);
204/204 goldens byte-identical; `make check` clean; Docker suite green on all
six distros with the per-distro counts 106 arch / 127 alpine3 / 126 others
unchanged, and serve 8/8.

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
