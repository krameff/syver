# Release History

A version-controlled record of every Syver release: what it was tagged, what
commit it points at, and what state the gate was in when it shipped.

This is deliberately **not** a second changelog. [CHANGELOG.md](CHANGELOG.md)
says what changed for users; this file says what was released, when, and from
where, so a version in the wild can always be traced back to a commit. Each
entry links to its changelog section rather than repeating it.

Newest first. Version numbers are continuous across the rename: this project
released as `krameff/goss` at v0.5.0 and v0.6.0, became Syver at v0.7.0, and
carried on counting rather than restarting at 1.0. A version number therefore
identifies one point in one history.

**Do not read those numbers as upstream's.** `goss-org/goss` at the time hadn't released
a v0.5.0 or v0.6.0 -- its versions run to v0.4.x. The v0.5.0 and v0.6.0 above
are this project's own, cut after the fork.

Releases are assembled on `devel` and merged to `main` at release time, so from
0.9.1 onward a release carries several branches rather than one.

Every tag from v0.7.0 onward is annotated and GPG-signed.

**Two different keys sign two different things, and confusing them makes a good
signature look like a bad one.**

| What | Signed by | Key |
| --- | --- | --- |
| Git tags | the maintainer's own key | `5154CE6E4F8712D87B9C870DCC071079D4E84F77` |
| Release artifacts: `SHA256SUMS`, the SBOMs | the project signing key, published as [`krameff-syver-key.asc`](krameff-syver-key.asc) | `CD218D529C95DC65A71F18D84C9E5095CABE5092` |

So importing `krameff-syver-key.asc` and then running `git verify-tag` will
report that it has no public key for the signature. That is expected, not a
problem with the tag. Verify each with the key that signed it:

```sh
git verify-tag v0.11.2                     # maintainer key
gpg --verify syver_0.11.2_SHA256SUMS.sig \
             syver_0.11.2_SHA256SUMS       # project key
```

This line previously read "signed with the same key", which had no antecedent
and invited exactly that mistake.

"Released" is the date the tag object was created, which is not always the
commit date: v0.7.0 was committed on 2026-08-18 and tagged on 2026-08-20.
v0.6.0 has no tag in this repository and uses the date its changelog entry
records.

## Re-cut tags

Four tags were deleted and re-created after first being pushed: `v0.9.1`,
`v0.10.0`, `v0.11.0` and `v0.11.1`.

**If you fetched one of those tags before it was re-cut, your copy is stale and
git will not correct it on its own.** Run `git fetch --tags --force`. Released
artifacts and signatures always correspond to the tag as it now stands.

## Contents

* [v0.11.2 - Signed SBOMs and a patched base image](#v0112---signed-sboms-and-a-patched-base-image)
* [v0.11.1 - Documentation site corrections](#v0111---documentation-site-corrections)
* [v0.11.0 - Windows: stop returning confident wrong answers](#v0110---windows-stop-returning-confident-wrong-answers)
* [v0.10.0 - Templating moved from sprig to sprout](#v0100---templating-moved-from-sprig-to-sprout)
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

## v0.11.2 - Signed SBOMs and a patched base image

| Field | Value |
| --- | --- |
| Released | 2026-09-08 |
| Tag | `v0.11.2` |
| Commit | `e888146` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel`, merged through PR #37. No feature branch; the commits were made directly on `devel` |
| Scope | 7 commits (6 excluding merges), 7 files, +237 / -109, measured at `v0.11.2` against `v0.11.1` |
| Changelog | [0.11.2](CHANGELOG.md#0112-based-on-krameffgoss-v060---signed-sboms-and-a-patched-base-image) |

Two supply-chain changes and no Go source change, so the binaries are
functionally identical to v0.11.1.

Every release now publishes a software bill of materials, one SPDX 2.3 document
per binary named to match it, each signed with the same key as the checksum
file. Signed releases were already a divergence from upstream, which publishes
checksums and neither signatures nor SBOMs; this extends that into the half of
the supply-chain story neither project told.

The container image now upgrades its Alpine packages at build time. The base
image is republished infrequently, so building alone shipped whatever package
set had been baked into it months earlier, and a container scan was reporting
OpenSSL advisories against the published image as a result. Pinning the base to
its point release does not help: the minor tag and the point release resolve to
the same digest. Syver's own binary is statically linked with cgo disabled and
calls none of those libraries, so nothing syver does was exploitable through
them, but the image is documented as a base image and an unpatched package in it
is inherited by every downstream `FROM`.

**Breaking:** none. Nothing in the program changed.

**Gate at release:** every workflow green **on `e888146` itself**, the tagged
commit, rather than on an ancestor: `Golang ci` across all twelve jobs including
`windows-latest` and macOS, `Validate YAML`, `Documentation`, `CodeQL Advanced`
and `Docker image for Syver`. `Build release artifacts` then ran on the tag and
completed success. That the gate and the tag name the same commit is the point:
the two preceding releases were tagged on trees their gates had never seen.

**Verified after the fact, not inferred.** The SBOM path had never run before
this release, and a failure at that stage would have come after the build and
the signing. The release carries eight `.spdx.json` documents and eight matching
`.sig` files alongside the signed checksum file. The published image
`ghcr.io/krameff/syver:v0.11.2` was pulled and inspected: it carries the
upgraded OpenSSL packages with nothing left upgradable, which also confirms the
upgrade step ran through goreleaser's arm64 build under QEMU and not only in the
workflow it was developed against. Open code-scanning alerts fell from
twenty-one to one, the remainder being a Go advisory that has no published fix
and that `govulncheck` reports as required but never called.

---

## v0.11.1 - Documentation site corrections

| Field | Value |
| --- | --- |
| Released | 2026-09-07 |
| Tag | `v0.11.1`. Cut twice; see [Re-cut tags](#re-cut-tags) |
| Commit | `f9f6b2c` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel`, merged through PR #36. No feature branch; the commits were made directly on `devel` |
| Scope | 6 commits (5 excluding merges), 10 files, +134 / -24, measured at `v0.11.1` against `v0.11.0` |
| Changelog | [0.11.1](CHANGELOG.md#0111-based-on-krameffgoss-v060---documentation-site-corrections) |

Documentation and CI only. No Go source, no `go.mod` change, so the binaries are
functionally identical to v0.11.0. It exists as its own version because the
things it fixes are user-visible and were wrong on a repository about to be made
public, not because anything in the program changed.

The substantive one is `docs/schema.yaml`. That file is what README and
`docs/gossfile.md` tell you to load into your editor from
`raw.githubusercontent.com`, and its field descriptions linked to `goss.rocks` in
nine places, so hovering a field opened upstream goss's documentation rather than
Syver's. Two of those links had no Syver equivalent at all and now point at
`gossfile.md#matchers`, which is where Syver's own prose sends a reader looking
for the same thing.

The rest: `docs/windows.md` was built but absent from the navigation, so the page
0.11.0 tells you to read was reachable only by typing its URL; every "Edit this
page" link returned a 404, because `edit_uri` named a `master` branch this
repository has never had; and `docs/goss.yaml` linked twice to a README heading
the rename had changed. The footer now carries Krameff Solutions Ltd's copyright
alongside the original author's.

One CI change with no user-facing effect: Dependabot now names `devel` as its
target branch. It had none, so it followed the repository default, and that
default moved to `main` when the repository was prepared for publication.
Dependency pull requests would have opened directly against the release branch.

**Breaking:** none. Nothing in the program changed.

**Gate at release:** `Golang ci` green on `4f317d9`, the tagged tree's parent,
across all twelve jobs including `windows-latest` and macOS; Validate YAML green;
`mkdocs build --strict` clean; markdown lint clean; `yamllint` clean across the
repository. One `Golang ci` attempt failed while downloading Trivy and passed on
re-run with no change; a transient fetch, not a finding.

---

## v0.11.0 - Windows: stop returning confident wrong answers

| Field | Value |
| --- | --- |
| Released | 2026-09-07 |
| Tag | `v0.11.0`. Cut twice; see [Re-cut tags](#re-cut-tags) |
| Commit | `5729f8a` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel`. One feature branch, `feature/windows-truthfulness` (PR #34), plus three dependency commits and two CI fixes taken during the release |
| Scope | 26 commits (22 excluding merges), 67 files, +2696 / -503, measured at `v0.11.0` against `v0.10.0` |
| Changelog | [0.11.0](CHANGELOG.md#0110-based-on-krameffgoss-v060---windows-stop-returning-confident-wrong-answers) |

Windows checks that could not run were reporting success. Fourteen such sites
were found, four of them serious. The root cause behind ten of the fourteen is
that the codebase treated a zero value with no error as "absent", a convention
that is invisible on Linux because "the tool is missing" and "the thing is
absent" coincide there. On Windows they never coincide.

This is a behaviour change, not a bugfix, which is why it is a MINOR. Windows
specs that passed before may now fail. The changelog lists every flip.

**This is the first release verified on Windows rather than inferred from
cross-compilation**, and that distinction earned its keep immediately. The work
arrived described as fully Linux-verified and passing every local gate. Running
it against a Windows Server 2025 guest found two regressions it had already
introduced, and one of them was the mirror image of the bug the release exists
to fix: `userLookupFoundNothing` tested for `user.UnknownUserError`, which is
the correct contract on Unix, while Windows returns raw `ERROR_NONE_MAPPED`
instead. A genuinely absent account was therefore classified as a lookup that
could not run, so `user: someone-absent: {exists: false}` FAILED. A false
failure, from the release built to remove false passes, and invisible to any
Linux gate because the Unix path is correct and its tests pass. The second was a
test asserting a hardcoded count that this release makes platform-dependent; the
code was right and the number was wrong.

**A pre-existing security defect was fixed along the way.** A service name from a
gossfile was interpolated into a PowerShell command line with `%q`, which is Go
string escaping and not PowerShell escaping. PowerShell evaluates `$(...)` inside
double-quoted strings, so a name containing a subexpression executed. It reached
`CreateProcess` as raw command-line text, so Go's own argument escaping never
applied. This predates the release: two of the three call sites used the
identical construction on `devel`. Names are now rendered as PowerShell
single-quoted literals, which interpolate nothing.

**Three dependencies moved in this release**, all verified on the platforms
they touch rather than on Linux alone. `golang.org/x/crypto` to v0.56.0, which
cleared the two advisories that failed v0.10.0's first release gate and which
reach syver only through sprout's bcrypt functions. `github.com/shirou/gopsutil/v4`
to v4.26.8 and `github.com/prometheus/common` to v0.71.0, taken here rather than
deferred because gopsutil backs `process:` and `port:`, both of which this
release changed and documented.

That mattered for a reason a test run could not have caught. `docs/platforms.md`
was changed to say `port:` is not implemented on Windows, on the strength of a
measured "not implemented yet" from gopsutil. That is a claim about a
dependency's behaviour, not syver's, so it is only true against a pinned
version, and the `port:` fixture is skipped and asserts nothing. It was checked
by hand against v4.26.8 rather than inferred from a green suite.

**Breaking:** none for the gossfile format or the CLI. The behaviour changes are
confined to Windows, and every one converts a silent pass into an explicit
error.

**Gate at release:** measured at `30f2de0`, which is the tag's tree apart from
this file: `git diff --stat 30f2de0 v0.11.0` reports `RELEASES.md` and nothing
else. Unit tests clean under `-race`
across 7 packages, 225 top-level tests and 634 counting subtests. `make check`
clean, with govulncheck and Trivy both reporting nothing and cross-vet clean for
windows/amd64 and darwin/amd64. Goldens 206 byte-identical. GitHub Actions run
`34112052013` green on all twelve jobs: lint, coverage, the five-distro Linux
suite (rockylinux9, almalinux10, jammy, alpine3, arch), linux/arm64,
linux/ppc64le, the serve suite, macOS and `windows-latest`. On Windows Server
2025, unit, validate and serve suites all green.

The golden run reports its baseline as `f15754c`, one commit behind the tag, so
`1cb724f` is the one commit the corpus did not gate. That commit deletes the
bullseye fixtures, which is exactly the change the corpus cannot see: a deleted
fixture leaves its golden in place and still matches. The count of 206 is
correct and was confirmed by hand rather than by the gate.

**Windows test coverage went from 33 live fixture entries to 42 of 47.** Two of
the previously skipped fixtures were not merely unexercised but wrong:
`interface` asserted that an interface did not exist while also asserting that
same interface's addresses and MTU, and the `user` and `group` fixtures were
unedited Linux copies asserting an NFS account's uid, home and shell against
`Administrator`. Skipping them is why nobody noticed. The validate harness also
could not host a fixture that is meant to fail: it ran under `errexit` with an
ERR trap, so the first failing fixture aborted the run and every later fixture
silently never executed. Fixtures now declare an expected exit code.

**Still not exercised, each for a stated reason:** `package` and `port` error
unconditionally on Windows because no backend exists, `mount` reports a
misleading-but-loud error whose fix needs cross-platform reordering, and
`kernel-param` has no Windows equivalent at all. `docs/windows.md` records this
rather than letting a green validate run imply coverage that is not there.

---

## v0.10.0 - Templating moved from sprig to sprout

| Field | Value |
| --- | --- |
| Released | 2026-09-04 |
| Tag | `v0.10.0` |
| Commit | `fc7201e` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel`. One feature branch, `feat/sprig-sprout-change`, plus a dependency fix made during the release itself |
| Scope | 11 commits (7 excluding merges), 12 files, +596 / -101 |
| Changelog | [0.10.0](CHANGELOG.md#0100-based-on-krameffgoss-v060---gossfile-templating-moved-from-sprig-to-sprout) |

The gossfile template engine moved from `Masterminds/sprig` to
`go-sprout/sprout`. Sprig has gone quiet since its last release; sprout is the
maintained community successor and its `sprigin` package is a near drop-in
replacement. The changelog lists what renders differently and why.

This is a MINOR rather than a patch because five functions genuinely render
differently. In every case sprout is fixing a sprig bug rather than introducing
one, but a gossfile that leaned on the old doubled-separator output from
`snakecase`, `camelcase` or `kebabcase` will produce different text now.

**The goldens do not gate this release, and it would be easy to think they do.**
Every note written before it said a template-engine swap changes rendered output,
so the golden corpus was the obvious gate. It is blind to the swap.
`ci/golden-baseline.sh` renders every spec it harvests, and all eight templated
fixtures in the tree needed vars or environment the harness never supplied, so
each one died at variable lookup and its golden recorded an error string that is
identical either side of the change. A byte-identical golden run was therefore
real and proved nothing about the engine.

What does gate it is `template_test.go`, which did not exist before this release
and is the template layer's first direct Go coverage, together with a new
`render-vars` case in the golden harness that renders one spec WITH its vars so
the corpus stops being blind here permanently. Adding that case also exposed a
second problem: the harness manifest listed only `validate`, `render` and `add`,
so the new golden would have been written on every run and never compared. Both
are fixed here.

**Breaking:** none for the CLI or the gossfile format. The template function
vocabulary changed as described above, which affects a gossfile only if it used
one of the five corrected functions and depended on the old output.

**Gate at release:** `make lint` clean; `make vet`, `make fmt` and
`go mod tidy -diff` clean; unit tests clean under `-race` across 7 packages;
`make check` clean with govulncheck and Trivy both reporting nothing; goldens
byte-identical; six-distro Docker suite green with the per-distro counts
106 arch / 127 alpine3 / 126 others unchanged, and serve 8/8.

---

## v0.9.4 - Housekeeping

| Field | Value |
| --- | --- |
| Released | 2026-09-01 |
| Tag | `v0.9.4` |
| Commit | `0927187` |
| Base | krameff/goss v0.6.0 |
| Integration branch | `devel`. Three branches: `fix/gofmt-and-modtidy`, `fix/docker-image-branch-triggers`, `fix/release-gate-lint` |
| Scope | 8 commits (5 excluding merges), 6 files, +165 / -46 |
| Changelog | [0.9.4](CHANGELOG.md#094-based-on-krameffgoss-v060---housekeeping) |

Nothing here changes what syver does.

The release gate now lints. `release.yaml` installs golangci-lint and runs
`make lint` before the unit tests, so the commit a tag points at is checked
rather than assumed: `golangci.yaml` triggers on branch pushes, which per
GitHub's documentation do not fire for tag pushes, and `make check` does not
include lint either. A doc comment in `toplevel_guard.go` and the `go` directive
in `go.mod` were tidied in the same release, which is what made `make lint` and
`make fmt` clean again.

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

**Not run:** the six-distro Docker suite on `go_builder`. The Go delta is one
comment and one `go.mod` directive, so it has nothing new to exercise.

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
goss-org/goss            upstream, v0.4.x
  └── this project       forked from v0.4.x, then:
        v0.5.0, v0.6.0     released under the name krameff/goss
        v0.7.0 onward      renamed to Syver, same history, same numbering
```

**`krameff/goss` and Syver are one project under two names, not two projects.**
The repository split is an artefact of the rename: `krameff/goss` is frozen at
v0.6.0 and ships nothing further. Treat v0.5.0 and v0.6.0 as Syver's own early
releases when reasoning about what has changed since the fork -- the whole of
v0.5.0 onward is this project's divergence from upstream.

Two version numbers to keep straight, because they collide in conversation and
not in fact. `v0.6.0` here is this project's; upstream's newest is v0.4.10.
There is no upstream v0.6.0 to compare against, so "five versions ahead" is not
a meaningful statement about the two projects.

Every release so far is cut from the v0.6.0 baseline; upstream `goss-org/goss`
`devel` has never been merged, which is why `lint.go` and `lint/` are absent
here and that absence is correct.
