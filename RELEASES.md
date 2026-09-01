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
commit date. Every tag from v0.7.0 onward is annotated and GPG-signed with the
same key. v0.7.0 was committed on 2026-08-18 and tagged on 2026-08-20. v0.6.0
has no tag in this repository and uses the date its changelog entry records.

v0.9.1 was tagged twice, and the second tag is the real one. It was first cut
through the GitHub release UI, which creates a lightweight tag: no tag object,
no signature. It was re-cut the same day as an annotated, signed tag pointing at
the same commit, so that every release from v0.7.0 on carries a signature. The
commit never moved and the release contents are unaffected. The only practical
consequence is for anyone who fetched v0.9.1 during that window: they hold the
old lightweight tag, and `git fetch --tags --force` is needed to pick up the
signed one, because git will not overwrite an existing tag ref on its own.

## Contents

* [v0.9.4 - Housekeeping](#v094---housekeeping)
* [v0.9.3 - Dependency maintenance](#v093---dependency-maintenance)
* [v0.9.2 - Timeout reporting, unknown key warnings and release plumbing](#v092---timeout-reporting-unknown-key-warnings-and-release-plumbing)
* [v0.9.1 - Correctness fixes and CI gating](#v091---correctness-fixes-and-ci-gating)
* [v0.9.0 - Correctness and shutdown fixes](#v090---correctness-and-shutdown-fixes)
* [v0.8.0 - Registry-driven dispatch](#v080---registry-driven-dispatch)
* [v0.7.0 - Rename to Syver](#v070---rename-to-syver)
* [v0.6.0 - Upstream baseline (krameff/goss)](#v060---upstream-baseline-krameffgoss)
* [Lineage](#lineage)

---

## v0.9.4 - Housekeeping

| Field | Value |
| --- | --- |
| Released | 2026-09-01 |
| Tag | `v0.9.4` |
| Commit | pending |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel`. Three branches: `fix/gofmt-and-modtidy`, `fix/docker-image-branch-triggers`, `fix/release-gate-lint` |
| Scope | 8 commits (5 excluding merges), 6 files, +165 / -46 |
| Changelog | [0.9.4](CHANGELOG.md#094-based-on-krameffgoss-v060---housekeeping) |

Nothing here changes what syver does. It exists because cutting 0.9.3 exposed
two gaps that had been open since 0.9.2, and both are cheaper to close than to
keep working around.

The first is a formatting defect that reached two releases. A doc comment in
`toplevel_guard.go` was not gofmt clean, and `go.mod` recorded `go 1.26` rather
than `go 1.26.0`, which made `go fmt` abort on module resolution before it
formatted anything. So `make fmt` failed for a reason that had nothing to do
with formatting and reported nothing useful about it, while `make lint` failed
for the real one. Neither was visible at the time: the violation landed on
2026-08-26, inside the window when the Actions allowance was exhausted, and
`make check` does not run lint.

The second is why that could happen at all. `golangci.yaml` triggers on branch
pushes, which per GitHub's documentation do not fire for tag pushes, and
`release.yaml`'s gate ran the unit tests and the security scan but no lint. A
tag could therefore be cut on a lint-red tree, and twice was. That gate now
installs golangci-lint and runs `make lint` before the tests, so the release
path checks the commit it is about to sign rather than assuming a branch run
covered it.

The container image workflow also stopped building on `devel`. It published a
`:devel` tag that no documentation mentioned and nothing consumed, at the cost
of a full two-architecture image build on every commit to that branch,
documentation-only ones included. Release images are unaffected: `:latest` and
the versioned tags are produced by goreleaser on the tag push and never came
from that workflow.

**Breaking:** none for gossfiles or the CLI. The only user-visible change is the
withdrawal of the undocumented `ghcr.io/<owner>/syver:devel` image.

**Gate at release:** `make lint` clean at 0 issues, and in the release gate for
the first time; `make vet`, `make fmt` and `go mod tidy -diff` all clean; 509
tests / 7 packages `-race` clean; 205/205 goldens byte-identical; `make check`
clean with govulncheck and Trivy both reporting nothing.

**Not run:** the Docker suite on `go_builder`, plus Windows integration, macOS
integration and CodeQL. The Go delta is one comment and one `go.mod` directive,
so the distro suite has nothing new to exercise. The three CI legs should run on
the push to `main` this time, since the lint failure that skipped them for 0.9.3
is what this release fixes; correct this entry if any of them fails.

---

## v0.9.3 - Dependency maintenance

| Field | Value |
| --- | --- |
| Released | 2026-09-01 |
| Tag | `v0.9.3` |
| Commit | `3cb48e1` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel`. Assembled from `deps/update-2026-08-29` and dependabot PRs #25 and #28, with documentation commits made directly on `devel` |
| Scope | 13 commits (9 excluding merges), 13 files, +135 / -75 |
| Changelog | [0.9.3](CHANGELOG.md#093-based-on-krameffgoss-v060---dependency-maintenance) |

A dependency refresh and documentation. Fifteen modules moved; four of them are
direct dependencies and the rest are indirect. One Go file appears in the diff,
`template.go`, and it changed by a single comment: an upstream pull request link
repointed from `krameff/syver` to `goss-org/goss`. No executable line changed
anywhere in the release. (The changelog entry says "No source file changed",
which is right in substance and imprecise in wording.)

One of the fifteen is worth calling out because it shrinks the dependency
surface rather than just advancing a number. `stretchr/testify` was the last
thing in the graph requiring `gopkg.in/yaml.v3`, the unmaintained package syver
moved off in 0.9.1. Its latest release depends on `go.yaml.in/yaml/v3` instead,
the same maintained fork syver already uses, so the duplicate YAML v3 is gone
from the build. The 0.9.1 fork swap could not achieve this on its own, and it
was recorded at the time as something that would need a separate fix; it turned
out to arrive for free.

It is a release of its own rather than part of 0.9.2 for a reason worth
recording. A dependency sweep is the change most likely to break on a platform
the local gate cannot reach, and the Windows integration, macOS integration and
CodeQL legs were unavailable when this was prepared. Folding it into 0.9.2 would
have shipped the riskiest surface unvalidated inside a release whose purpose was
to prove the release plumbing works.

Two of the moves are worth naming because they touch what users see rather than
what builds: the command line framework, which owns flag parsing and help text,
and the assertion library behind every matcher message. Neither changed any
observable output, which the golden files confirm.

**Breaking:** none. No behaviour changed for any gossfile.

**Gate at release:** 509 tests / 7 packages `-race` clean; 205/205 goldens
byte-identical; `make check` clean with govulncheck and Trivy both reporting
nothing; Docker suite green on all six distros with the per-distro counts
106 arch / 127 alpine3 / 126 others unchanged, and serve 8/8.

That gate did not include lint, and it should have.

**Not run at preparation time:** Windows integration, macOS integration, CodeQL.
Those were the reason this was held back rather than folded into 0.9.2.

**What happened at tag time is not what was planned.** The tag was cut on
2026-09-01, the day the Actions allowance reset. The push to `main` did trigger
`golangci.yaml`, since the delta carries `go.mod`, `go.sum`, `template.go` and
three workflow files and so escapes that workflow's `paths-ignore`. But its
`lint` job fails at this commit: `toplevel_guard.go` is not gofmt clean and
`.golangci.yaml` enables the gofmt formatter. `coverage` declares `needs:
[lint]`, and all four integration groups declare `needs: [coverage]`, so the
Windows and macOS legs were skipped rather than run. Reproduce the trigger with
`git show v0.9.3:toplevel_guard.go > /tmp/t.go && gofmt -l /tmp/t.go`. CodeQL is
a separate workflow with no such dependency and was unaffected.

That paragraph is read off the workflow graph at this commit, not off an
observed run. Check it against the Actions tab and correct it here if those jobs
did report. The release itself stands either way: the defect is a comment's
formatting, `release.yaml`'s gate at the time ran tests and the security scan
but no lint, and the artifacts were built from a tree whose unit tests and
goldens are green. 0.9.4 carries the formatting fix and adds `make lint` to the
release gate, so a tag can no longer be cut on a lint-red tree.

---

## v0.9.2 - Timeout reporting, unknown key warnings and release plumbing

| Field | Value |
| --- | --- |
| Released | 2026-08-29 |
| Tag | `v0.9.2` |
| Commit | `759d36d` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel` |
| Scope | 26 commits (18 excluding merges), 39 files, +1588 / -153 |
| Changelog | [0.9.2](CHANGELOG.md#092-based-on-krameffgoss-v060---timeout-reporting-unknown-key-warnings-and-release-plumbing) |

Six branches, in merge order: `fix/drop-legacy-artifacts`,
`feat/command-output-ownership`, `fix/ci-concurrency`,
`fix/add-swallows-timeout`, `fix/windows-powershell-timeouts`,
`feat/toplevel-key-guard`.

The headline is the top-level key guard. A gossfile key that syver did not
recognise, whether a typo or a key from a newer version, was previously dropped
without a word: the run then reported a clean pass having checked less than the
file asked for. For a tool whose product is compliance evidence, silent
under-testing is the worst possible failure mode, and it is now a warning naming
the file, the line and a likely correction. It does not fail the run.

The rest is timeout honesty. Three paths that previously reported a definite
answer when they had learned nothing now report the failure instead: `serve` no
longer hangs forever on a check that starts a background process, `syver add` no
longer records a package as missing when the package manager stopped responding,
and it no longer drops file owner and group when the directory service did.

### Release plumbing, in more detail than the changelog carries

Releases no longer build the duplicate `goss-` named binaries. Nothing consumed
them: the first release from this repository was already renamed, so no
pre-rename download URL ever pointed here. The wrapper scripts `dgoss`, `dcgoss`
and `kgoss` are a separate thing and deliberately keep their names.

A scratch tag such as `vtest` can no longer trigger a signed release. The tag
filter was `v[0-9]*`, which matched more than intended.

Concurrency groups were added to seven workflows, so pushing again to a branch
cancels the run still in flight rather than leaving both to finish.

**Breaking:** none for the CLI or for any gossfile. One library-only change: the
exported `ReadJSONData` now takes a `path string` naming the spec the data came
from, used solely to say which file a warning refers to. Callers with no path
pass `""`. This was accepted in a patch release deliberately, since a v0.x Go
module carries no compatibility guarantee and there were two non-test call sites.

**Gate at release:** 509 tests / 7 packages `-race` clean (up from 480 at
v0.9.1); 205/205 goldens byte-identical, the 205th being the guard's own
`examples/unknown-top-level-key.yaml`; `make check` clean with govulncheck and
Trivy both reporting nothing; Docker suite green on all six distros with the
per-distro counts 106 arch / 127 alpine3 / 126 others unchanged, and serve 8/8.

**Artifacts were built after the tag, not with it.** GitHub Actions minutes were
exhausted when this was tagged on 2026-08-29, so the release workflow could not
run at tag time and was started manually once they reset on 2026-09-01. The tag
and the commit it points at never moved. Anyone comparing timestamps will see a
gap between the tag date and the asset dates, and that is why.

For the same reason, three legs of the gate had not run when the tag was cut:
Windows integration, macOS integration and CodeQL. Note this release contains
`fix/windows-powershell-timeouts`, whose whole purpose is to stop the Windows
suite failing on slow runners, so that leg in particular is the one to confirm
green on the deferred run.

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
