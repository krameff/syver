# Changelog

## 0.9.3 based on krameff/goss v0.6.0 - dependency maintenance

- deps/update-2026-08-29 branch
  - a routine refresh of fourteen dependencies. Nothing changes for anyone
    running syver: no check behaves differently, no output moved, and the CLI
    and gossfile format are untouched
  - this is maintenance rather than a security fix. The vulnerability scanners
    were already reporting nothing before the update, and still are
  - of the three direct dependencies that moved, two are the DNS client behind
    the `dns` resource and the command line framework. The rest are indirect

## 0.9.2 based on krameff/goss v0.6.0 - timeout reporting, unknown key warnings and release plumbing

- fix/drop-legacy-artifacts branch
  - releases no longer build duplicate `goss-` named binaries. Nothing used them
  - a scratch tag such as `vtest` can no longer trigger a signed release

- feat/command-output-ownership branch
  - `serve` no longer hangs forever when a check starts a background process. It
    now gives up on that check after about 35 seconds instead
  - documented that a command which detaches itself survives its own timeout
  - documented the fixed 30 second bound on the checks syver runs for you

- fix/ci-concurrency branch
  - pushing again to a branch now cancels the CI run still in flight for the
    previous push, so results arrive sooner and only the newest commit is
    reported on

- fix/add-swallows-timeout branch
  - `syver add` no longer records a package as missing when the package manager
    stops responding. It reports the failure instead
  - `syver add` no longer drops file owner and group when the directory service
    stops responding. It reports the failure instead

- fix/windows-powershell-timeouts branch
  - raised the Windows powershell timeouts, which were failing CI on slow runners
  - Windows integration tests no longer fail on a slow CI runner

- feat/toplevel-key-guard branch
  - a gossfile with a typo'd or unrecognised top-level key -- `prot:` instead
    of `port:`, or a key from a newer syver than the one running -- now gets a
    warning naming the file, the line, and a suggestion when one is close
    enough. It used to be dropped without a word, and the run would report a
    clean pass having checked less than it claimed
  - this warning does not fail the run or change its result. A top-level key
    beginning `x-`, and any block that exists only to carry a shared YAML
    anchor, is exempt and never warns
  - covers YAML gossfiles only; JSON gossfiles have the same gap and are not
    covered yet. `--vars` files are never checked, since they have no fixed
    vocabulary to check against
  - breaking change for library users only, not for the CLI: the exported
    `ReadJSONData` function takes an additional `path string` argument, naming
    the spec the data came from (used only to say which file a warning above
    came from). Nothing shells out to `syver` differently, and gossfile
    behaviour is unchanged either way. If you call `ReadJSONData` directly as
    a library, pass the file path if you have one, or `""` if you don't

## 0.9.1 based on krameff/goss v0.6.0 - Correctness fixes and CI gating

- feat/yaml-fork branch
  - YAML handling moved to the maintained fork of go-yaml. Output is unchanged
  - a gossfile that mixes a `<<:` merge with an unusual key now reports a parse
    error instead of crashing

- feat/trivy-gate branch
  - security scanning now fails the build on findings. It used to print them and
    pass anyway
  - updated a dependency, clearing two high severity advisories
  - the scanner is pinned, also checks for committed secrets and Dockerfile
    problems, and can tell "found something" from "could not run"
  - fixed a check that kept advising us to remove suppressions still in use

- feat/trivy-ignore-yaml branch
  - accepted risks now record why they were accepted, in a form tooling can read
  - the Dockerfile is scanned too. It runs as root, which is deliberate and now
    written down

- fix/timeout-message branch
  - a command that times out now says how long it waited
  - a command that failed to start could report success, so a check expecting
    exit status 0 would have passed for a command that never ran
  - a negative `timeout:` left a command running with no limit. It now warns and
    uses the default
  - fixed a race that could corrupt what a timed-out command reported
  - warnings about a spec now appear once, instead of on every check

- fix/ci-branch-filter branch
  - removed a workflow filter that had never matched anything
  - a test that failed because of its own setup now says so, instead of looking
    like a timing problem

- fix/release-path-gating branch
  - releases are now gated. Nothing previously checked the commit a release tag
    pointed at
  - code analysis runs on the release branch, not just the development branch

## 0.9.0 based on krameff/goss v0.6.0 - Correctness and shutdown fixes

- feat/patches branch
  - **breaking:** `--format structured` now exits non-zero when checks fail. It
    always exited 0, so a failing run reported success to anything reading the
    exit code
  - **breaking:** `/healthz` no longer reports a failing host as healthy. Its
    status comes from the results, so no `Accept` header can override it.
    `--format prometheus` still exits 0 by design: it renders metrics, not a
    verdict
  - **library API:** `Validate` and the top-level entry points now take a
    `context.Context` as their first argument
  - an empty matcher such as `stdout: {}` now reports a syntax error instead of
    crashing, and a panic in one check fails that check rather than the process
  - `syver add` no longer writes empty list matchers, and `render` no longer
    prints `contents: null`. None of them asserted anything. Writing one
    yourself now warns
  - Ctrl-C stops commands that are already running instead of orphaning them,
    and `syver serve` shuts down cleanly on SIGTERM
  - a cancelled run reports `context canceled` instead of exit status -1, which
    was indistinguishable from a process killed by a signal
  - a hung `systemctl`, `rpm`, `apk` or `getent` no longer wedges `serve`
    forever
  - warnings appear once per attribute instead of once per check, so they no
    longer flood the log under `serve`

## 0.8.0 based on krameff/goss v0.6.0 - Registry-driven dispatch

- modularization branch
  - FEAT-007: registry-driven dispatch (Modularisation, Phase 1). Verified
    no-op internal refactor -- `./ci/golden-baseline.sh verify` passes
    against all 204 pre-refactor goldens -- with exactly one deliberate
    behaviour change (see below)
  - new `resource.Descriptor`/`Register`/`Descriptors()` (`resource/descriptor.go`)
    replaces the old `registerResource`/`Resources()` registry; a
    mutex-guarded, panic-on-duplicate table mirroring
    `outputs.RegisterOutputer`
  - `matching` is registered for the first time -- it had no `init()` at
    all before this. goss-modular (the prior-art branch this spec's design
    was informed by) couldn't register it without colliding with a
    `MatchingResourceKey = "mount"` copy-paste bug it never fixed; Syver
    fixed that bug in `802454a` and can register cleanly
  - `resource/resource_list.go` (1,629 generated lines) and
    `resource/resource_list_genny.go` deleted outright, along with the
    `genny` dependency and the Makefile `gen` target. Replaced by one
    generic `ResourceMap[T, ST, PT]` (`resource/resource_map.go`) carrying
    `AppendSysResource`/`AppendSysResourceIfExists`/`UnmarshalJSON`/
    `UnmarshalYAML` for all 17 types via type aliases (`type PortMap =
    ResourceMap[Port, system.Port, *Port]`, etc.)
  - new `dispatch.go` (root package): one `configAccessors`/
    `discoveryAccessors` table replaces the switch/list/map literals
    that used to be duplicated once per type across `syver_config.go`,
    `discovery_config.go` and `add.go`. An `init()`-time guard panics if
    the accessor tables and the resource registry ever fall out of sync
    (the resolution of `syver_config.go`'s old `// FIXME: Can this be
    moved to a safer compile-time check?`)
  - `syver add`'s 16-case switch (`add.go`) replaced with a descriptor
    lookup by name; a live-field accessor (not a copy) keeps newly-added
    resources actually landing in the written file
  - `cmd/syver`'s ~180-line hand-written `add` subcommand list replaced
    with a loop over the resource registry
  - fixed `resource/validate.go` deriving a resource's printed type via
    `reflect.TypeOf` instead of calling `TypeName()` -- the reflect-based
    version breaks for any out-of-package type, which matters once
    external types exist (see below)
  - **the one deliberate behaviour change:** `Matching.SetSkip()` was a
    no-op, so `util.WithDisabledResourceTypes("matching")` silently failed
    to disable it (`total=1 skipped=0 failed=1` instead of `total=1
    skipped=1 failed=0`). Fixed; no golden exercises
    `DisabledResourceTypes` (there is no CLI flag for it), so this changes
    no golden. Regression test:
    `TestMatchingSetSkipDisablesValidation`
  - new conformance suite (`resource/conformance_test.go`), driven by the
    descriptor registry, gives the 15 previously-Go-untested resource
    types (of 17) real coverage for the first time
  - test floor: 339 -> 365 cases, still 7 packages, `-race` clean;
    `make check` clean (vet, lint, lint-markdown, gofmt), and the Docker
    integration suite passes 7/7 with its three hardcoded per-distro
    assertion counts (106/127/126) unchanged -- which is the no-op proof

## 0.7.0 based on krameff/goss v0.6.0 - Rename to Syver
# Keeping versioning due to goss legacy compatability

**Renaming release.** goss becomes Syver. Your gossfiles need no changes:
`gossfile:`, `goss.yaml`, the `GOSS_*` environment variables, `-g`, and the
`dgoss` / `dcgoss` / `kgoss` wrappers all keep working.

The breaking changes are limited to things that referenced the product by name
(the binary is now `syver`, plus the User-Agent, checksum filename, container
image and Go module path).

- **Upgrading:** [docs/migrations.md](docs/migrations.md#upgrading-from-krameffgoss-v060)
- **Full side-by-side of what did and did not change:** [docs/goss-vs-syver.md](docs/goss-vs-syver.md)

### Detail

- syver_initial branch
  - `/metrics` now serves the `outputs` private registry, not the default global one
  - added `outputs.MetricsRegistry()` accessor
  - registry initialised eagerly at `serve` startup from process-level format options
  - fixes `integration-tests/run-serve-tests.sh:102`; test assertion unchanged
  - module path `github.com/krameff/goss` -> `github.com/krameff/syver`
  - `package goss` -> `package syver`; 138 import lines across 85 files
  - `GossConfig`/`GossMatcher`/`Gossfile` and compounds -> `Syver*`
  - `cmd/goss/goss.go` -> `cmd/syver/syver.go`
  - genny source `resource/resource_list_genny.go` updated, not the generated file
  - wire contracts unchanged: nagios prefix, junit suite name, `goss_tests_*` metrics, `gossfile:` keys and struct tags, `GOSS_*` env vars, User-Agent, media type
  - `Makefile` `release/goss-%` -> `release/syver-%`; `make build` works again
  - `.goreleaser.yaml` gains `project_name: syver`; binary, main, ldflags and image repo renamed
  - second archives entry `archives-legacy` still emits `goss-<os>-<arch>`
  - `Dockerfile` installs `syver`; `/goss` kept as a second volume
  - `install.sh` and `release-build.sh` repointed at `krameff/syver`
  - `syverfile:` accepted as an input alias for `gossfile:`, never emitted on write
  - `gossfile:` wins on collision, with one warning
  - config filename probe: `syver.yaml`, `syver.yml`, `goss.yaml`, `goss.yml`
  - `add` writes back to whichever file the read resolved to
  - CLI name `syver`; `--syverfile` aliases `gossfile`/`g`; `add syver` aliases `add goss`
  - 16 `SYVER_*` env vars paired with `GOSS_*`; an exported-empty one no longer shadows
  - `syver_tests_*` metrics emitted alongside `goss_tests_*`
  - `vnd.goss-` and `vnd.syver-` both accepted, the client's own prefix echoed back
  - no Accept header still answers `vnd.goss-`
  - first valid Accept candidate honoured, not discarded by a later invalid one
  - User-Agent -> `syver/` (intended break)
  - new wrappers `dsyver`, `dcsyver`, `ksyver`; `dgoss`, `dcgoss`, `kgoss` kept as shims
  - wrappers resolve the binary as `which syver` then `which goss`
  - wrappers stage `syver.yaml` before `goss.yaml`, wait step gated on the same set
  - `ksyver` in-pod exec pointed at the files it actually stages
  - tests 282 -> 325
  - renamed docs, site branding (mkdocs.yml), and repo-hygiene references from goss to Syver
  - wrappers now honour `SYVER_*` env vars too, same precedence as the binary
  - `dgoss`, `dcgoss`, `kgoss` shims paired as well, so both names behave identically
  - harness variables `GOSS_BINARY`/`GOSS_ARGS` -> `SYVER_BINARY`/`SYVER_ARGS`
  - windows validate fixture repointed at `release\syver-windows-amd64`
  - `docs/installation.md` checksum file corrected to `syver_<ver>_SHA256SUMS`
  - new `docs/goss-vs-syver.md`: side-by-side comparison, linked from README and nav
  - `docs/migrations.md` restructured into tables; content unchanged
  - serve integration tests now report failures: `cleanup()` captured no exit code, so every failure passed
  - same script's `[[ ]] && log_fatal` tail meant a passing run's natural status was 1, also masked
  - `killall` in that trap guarded, so reaping nothing cannot fail a clean run
  - Go toolchain 1.26.5 -> 1.26.6 for the stdlib fixes; `go.mod`/`go.sum` already tidy
  - `install.sh` wrapper branch `master` -> `main`; the old value 404'd unless `GOSS_VER` was pinned
  - `install.sh` now installs `dsyver`, keeping `dgoss` alongside as a shim
  - dead `Dockerfile_*.md5` cache branch removed from `test.sh`; no such file ever existed
  - `build_images.sh` labels `image.title` and `rocks.goss.dockerfile-md5` renamed to syver
  - docker object names renamed: image `goss_<os>` -> `syver_<os>`, container `goss_int_test_*`, network `goss-test`
  - the `/goss` bind mount and the `goss/` fixture tree keep their names, matching the file-format rule
  - `install.sh` takes `SYVER_VER`/`SYVER_DST`, still honouring `GOSS_VER`/`GOSS_DST`
  - an exported-but-empty `SYVER_VER` falls through to `GOSS_VER` rather than shadowing it
  - `docs/installation.md` manual-install block now names the `syver-<os>-<arch>` assets
  - signing key updates
  - new release signing key, fingerprint `CD218D529C95DC65A71F18D84C9E5095CABE5092`
  - key file `krameff-goss-key.asc` -> `krameff-syver-key.asc`; import the new one to verify 0.7.0+
  - `.goreleaser.yaml` and `docs/installation.md` repointed at the new filename
  - serve integration tests now wait for the server to bind before asserting
  - previously only darwin slept; on linux the first curl raced the listener
  - cross-arch runs never won that race: ppc64le needs ~500ms to bind under qemu
  - serve tests also assert the `vnd.syver-` and `syver_tests_*` side, not just `goss`
  - `ci/security-scan.sh` accepts podman, not just docker, so trivy runs on this sandbox
  - a skipped trivy scan now says so in a summary line instead of exiting 0 silently
  - removed the goss-era asciinema demo from the README
  - `serve` no longer runs on a zero-value `http.Server`: read, read-header and idle deadlines set
  - `WriteTimeout` deliberately left unset; validation runtime is set by the spec, and a `command` resource can legitimately run for minutes
  - `serve` registers on its own `ServeMux` instead of `DefaultServeMux`
  - `syverMu` is now actually taken: concurrent cache misses collapse to one validation instead of one per request
  - the mutex had been allocated and never locked since upstream, so a burst of probes ran N full sweeps of the system
  - `color.NoColor` writes collapsed onto a `sync.Once` in `outputs` and `serve`
  - that was a genuine write-write data race in `serve` mode, where JSON/JUnit responses render concurrently; `go test -race` reports it against the previous code
  - wrapper staging directory is no longer world-writable at the top level
  - the 0777 directory the container mounts now nests inside the 0700 directory `mktemp -d` creates, so other local users cannot reach the staged `syver` binary the container then executes
  - applied to all six wrappers: `dsyver`/`dgoss`, `dcsyver`/`dcgoss`, `ksyver`/`kgoss`
  - `docs/schema.yaml` gains `contents`; `contains` kept but described as deprecated
  - `serviceTest` no longer requires `skip`: it was the only definition that did, and it rejected the output of `syver add service`
  - `contents` accepts null as well as an array, which is what `syver render` emits for a file resource that does not set it
  - `docs/goss.yaml` example switched from the deprecated `contains:` to `contents:`
  - `docs/rendered_syver.yaml` regenerated from the current binary; it had drifted, and now validates cleanly against the schema
  - README's yajsv sample output corrected; it showed a duplicated block and two failures that the schema no longer produces
  - `docs/platforms.md` documents `SYVER_USE_ALPHA` alongside the legacy `GOSS_USE_ALPHA`
  - the alpha bypass hint printed on macOS/Windows now names `SYVER_USE_ALPHA`; `GOSS_USE_ALPHA` still works
  - `docs/installation.md` GoReleaser output path corrected; it still named a `goss` build id and binary
  - a `command:` resource that times out now has its process group killed
  - previously only the timeout error was returned: the child kept running, reparented to init, with its goroutine parked in `Wait`. Under `serve` that leaked one process per cache refresh against a hanging command
  - killing the direct child is not enough, because commands run through `sh -c` and the hang is usually the shell's own child; the whole group is signalled instead
  - Windows keeps `exec.CommandContext`'s default (the started process only): a process tree there needs a Job Object, which is out of scope while Windows support is alpha
  - `go test -race ./...` is clean, and `ci/go-test.sh` now runs it
  - the suite could not pass under the detector before, which is why the `color.NoColor` write-write race above went unnoticed for the project's entire history
  - the cause was two `t.Parallel()` tests in serve_test.go both retargeting the process-global logger via `log.SetOutput`; every race the detector reported came from test parallelism, none from the CLI, which never loads configs concurrently
  - `mkdocs.yml` `site_url` still pointed at `goss.readthedocs.io`, which would have published this fork's site with upstream goss's canonical links, sitemap and OG metadata
  - `docs/container_image.md` documented `docker exec weby /syver/syver autoadd nginx`; `/syver` is an empty VOLUME and the binary installs to `/usr/bin`, so the documented command could not work
  - `docs/cli.md`'s `validate --loglevel` block listed a `FATAL` level that does not exist and described `serve`'s healthcheck behaviour; replaced with the five levels `logs.go` actually defines
  - `docs/testing.md` was absent from the nav and unreachable from the published site
  - both wrapper READMEs pointed at `/goss/docker_output.log`; the scripts mount `/syver`
  - `GOSS_FILE`'s documented default understated the real four-name probe
  - the dsyver macOS instructions downloaded a versioned tarball this project has never produced
  - `docs/goss.yaml` linked to `docs/manual.md`, which does not exist
  - `ksyver`'s own usage text contradicted its coded `GOSS_CONTAINER_PATH` default
  - `command:` documented as executable input, and `serve` documented as unauthenticated and echoing command output verbatim
  - removed the stale `.golangci.bck.yaml`; the dead docs-preview workflow keeps its `if: false` but gains the reason and a corrected project slug
- release signing and resource-type fixes
  - the release workflow now verifies, before building, that the imported GPG key is the key this project publishes. The `v0.7.0` run failed after 3m27s with `gpg: skipped "B2F0081380DB3968A9475BDE829BD6789AB4CEF0": No secret key` -- a fingerprint matching neither the key published at the time nor the one that replaced it, so the `GPG_PRIVATE_KEY` secret does not hold the published key
  - `docs/installation.md` and `docs/migrations.md` cited the superseded `326F2A90...67CD` fingerprint after the key file was rotated; both now name the published `CD218D52...5092`, and the check derives the expected fingerprint from `krameff-syver-key.asc` at run time so a future rotation cannot leave it stale again
  - the check reads the expected fingerprint out of the checked-in public key rather than hardcoding it, asserts a signing-capable *primary* secret exists (a `--export-secret-subkeys` export cannot sign this key, whose only signing-capable key is the primary, and fails exactly as above), and signs a throwaway file to prove signing works before the cross-compile starts
  - `resource.Matching`'s `TypeKey`/`TypeName` were `mount`/`Mount`, copied from `mount.go`. A skipped `matching` test reported itself as `Mount`, `depends-on: matching:x` could not resolve, a `matching` and a `mount` sharing a key collided as a duplicate resource reference, and `util.WithDisabledResourceTypes("mount")` disabled matchings too. Now `matching`/`Matching`
  - `resource.AddResourceName` renamed to `AddrResourceName`; the value was always correct, only the identifier was missing its `r`
  - `.yamllint` ignores the templated `examples/` specs, which are Go templates and so are not valid YAML until rendered -- the same treatment the existing `integration-tests` template fixtures get. The non-templated examples stay in scope and lint clean
  - an empty list now means the same thing on every attribute. `isSet` (nil, or a list with no entries) was the guard on only six of the 41 optional attributes -- `command.stdout`/`stderr`, `file.contains`/`contents`, `http.headers`/`body` -- and the other 35 used a bare `!= nil`. So `stdout: []` was dropped while `port.ip: []`, `user.groups: []`, `service.runlevels: []` and the rest produced a vacuous passing test that inflated the count. All 35 now use `isSet` too; since `isSet` is identical to `!= nil` for every non-slice value, nothing else changes -- `enabled: false` and `uid: 0` are still expectations
  - the empty list is kept as "no expectation" rather than made to mean "is empty", because it is what `syver add`/`autoadd` emit for an attribute they found nothing to assert about; reinterpreting it would make every generated gossfile assert emptiness on re-validation. `have-len: 0` is the way to assert an attribute really is empty, and `docs/gossfile.md` now says so under Matchers
  - each resource's mandatory attribute (`file.exists`, `command.exit-status`, `http.status`, ...) is deliberately left unguarded, so a resource always produces at least one result
  - `resource/isset_test.go` pins both halves: the predicate, and that an empty list produces no result while a populated one still does
  - `integration-tests/syver/goss-service.yaml` dropped the `runlevels: []` branch, which existed only to emit an empty list and is now a no-op. This was the only empty list in the counted integration run that sat on a previously-unguarded attribute -- verified by rendering all six distro specs and diffing. `integration-tests/test.sh` splits its hardcoded count three ways accordingly: arch 106 (no `goss-service.yaml`), alpine3 127 (a real `runlevels` expectation), the other four 126
- ci permissions
  - the Trivy SARIF upload failed on every push to `main` with
    `Error: Resource not accessible by integration`, citing the Actions
    workflow-runs endpoint. The cause was **GitHub Advanced Security not
    being enabled on this repository**: code scanning is unavailable on a
    private repo without it, and the API rejection surfaces as a generic
    not-accessible error naming an unrelated-looking endpoint rather than
    saying so. Fixed by enabling Advanced Security in the repository
    settings -- a repo setting, not a code change, so nothing in the tree
    records it and it is easy to re-debug from scratch. Hence this note
  - `docker-syver.yaml` and `trivy-schedule.yaml` also gained
    `actions: read`, which `github/codeql-action` documents as required on
    private repositories for the run lookup that `security-events: write`
    does not cover. Kept because it is correct, but it was **not** what
    fixed the error above: both failing runs predated it (each reported
    `revision=000f6e84`, the merge commit, which does not contain it), so
    the error was never actually reproduced with it in place. Enabling
    Advanced Security is what resolved it
- integration-tests directory rename
  - `integration-tests/goss/` renamed to `integration-tests/syver/`
    (`git mv`, history preserved) -- the last goss-named path segment in
    the test harness; fixture filenames inside it (`goss.yaml`,
    `*.goss.yaml`, `hellogoss.txt`, etc.) are unchanged, per the standing
    "file format stays goss-named" rule
  - the in-container Docker mount path stays `/goss`
    (`-v "$PWD/syver:/goss"` in `integration-tests/test.sh`) --
    deliberately not renamed: internal test-harness plumbing, never
    documented or read outside the ephemeral test container
  - `test.sh`'s local `goss_bin` variable renamed to `syver_bin` (value
    unchanged)
  - `discovery_integration_test.go:167` built its fixture path with
    `filepath.Join("integration-tests", "goss", ...)`, so the string
    `integration-tests/goss` never appears contiguously in the source and
    no substring grep could find it. The directory move alone broke
    `TestValidateWithDiscoverFlag` and `TestValidateInlineDiscovery`; the
    comment two hundred lines below it had been updated while the
    functional line had not
  - `.yamllint`'s five ignore-list entries repointed. They name Go-template
    fixtures that are not valid YAML, so once the paths went stale
    `make lint-yaml` walked into all five and failed. Confirmed both ways
    on go-builder, where yamllint is actually installed: exit 0 with the
    fix, exit 1 and four syntax errors without it
  - verified end-to-end on the remote Docker host (go-builder): all three
    distro branches run and matched their hardcoded count assertion for
    the first time against a real Docker run -- rockylinux9 (`Count: 126,
    Failed: 0, Skipped: 5`), alpine3 (`Count: 127, Failed: 0, Skipped:
    5`), and arch (`Count: 106, Failed: 0, Skipped: 3`), all exit 0. Also
    ran `run-serve-tests.sh` (8/8 assertions passed) and both
    `ci/discovery-e2e.sh`/`ci/depends-on-e2e.sh` against the real built
    `linux-amd64` binary

---

## [0.6.0] - 2026-07-26

### Updated

- Bumped `golang.org/x/crypto` to v0.54.0 after a Trivy alert flagged the `openpgp` subpackage as unmaintained. We don't actually use openpgp anywhere (it's dragged in transitively by sprig's bcrypt template functions, and doesn't even show up in the build's import graph), so this is just good hygiene rather than a real fix. Added a `.trivyignore` entry with the reasoning, plus a small script (`ci/trivyignore-check.sh`) that re-checks every `.trivyignore` entry against a fresh scan whenever `go.mod`/`go.sum`/`.trivyignore` change, so we get nudged to revisit it instead of it quietly going stale
- Bumped `golang.org/x/text` to v0.40.0, picked up as a side effect of the `x/crypto` bump above. This one's a real fix, not just hygiene: v0.38.0 had CVE-2026-56852, an infinite loop on invalid input, and the fixed version (0.39.0) shipped in Trivy's report was already behind what we ended up with
- `docs/index.md` now links out to every doc page (installation, quickstart, CLI reference, gossfile, migrations, platforms, containers, contributing, changelog, license) instead of only rendering the README intro/about snippets; the gossfile link also calls out `discovery` and `depends-on` directly
- `docs/migrations.md` documents migrating from `goss-org/goss` to `krameff/goss`: gossfile content needs no changes, only the install source, container image, and Go module import path; also notes that `discovery` and `depends-on` don't currently exist upstream
- Gave the project its own logo: a checkmark-in-a-"G" icon with the Goss wordmark and a "by Krameff Solutions Ltd" credit line, replacing the plain Krameff badge on the README and docs homepage
- Fixed a CI failure where `govulncheck` was flagging a Go standard library vulnerability (`GO-2026-5856`, a TLS privacy leak); pinned the Go toolchain to 1.26.5, which has the fix
- Swapped out the process and port lookups: `github.com/goss-org/go-ps` and `github.com/goss-org/GOnetstat` (both unmaintained since 2023) are replaced by `github.com/shirou/gopsutil/v4`, an actively maintained library that does the same job. No behavior or YAML schema changes for the `process`/`port` resources; also added unit test coverage for both, which didn't exist before

### Added

- `system/process_test.go` and `system/port_test.go`: table-driven tests covering process/port found and not found, multiple PIDs per executable, multiple protocols on the same port number, and a regression test for the per-protocol error isolation the `port` lookup relies on
- New process resource fields, made possible by the gopsutil switch above: `status` (e.g. spot zombie processes) and `user` (e.g. flag anything unexpectedly running as root), both aggregated across every PID matching the executable
- New port resource field: `pid`, the process ID(s) that own a listening socket. Not auto-populated by `goss add`/discovery, unlike the other new fields -- PIDs get reassigned on every restart, so baking one into a generated gossfile would just be a value that's already stale by the time anyone runs it. Add it to a gossfile by hand if you want it checked
- `resource/process_test.go` and `resource/port_test.go`: table-driven tests for both resources, which had no tests at all before this
- `docs/gossfile.md` documents the three new fields (`process.status`, `process.user`, `port.pid`)

### Fixed

- `ValidateGomegaValue` (the core matcher dispatcher) had no case for a resource property backed by a `func() ([]int, error)`, so the new `port.pid` field would have silently failed every check with an "unknown method signature" error; added the missing case
- Integration tests were breaking on routine OS package updates because they pinned exact package version strings. Package checks now use a regex match, and generated-vs-expected snapshot diffs ignore version lines entirely
- `docs/schema.yaml`'s header comment pointed at `docs/manual.md`, which doesn't exist -- the resource docs live in `docs/gossfile.md` now. Updated the reference
- The `Golang ci` workflow's `pull_request` trigger had no `paths-ignore`, so a docs-only PR still ran the full lint/coverage/integration matrix. Gave it the same `paths-ignore` (`**/*.md`, `docs/**`) already used on the `push` trigger

---


v0.5.0
7th July 2026

### Updated

- updated jammy apache2 version
- Repo moved to its new home at `github.com/krameff/goss`; Go module path, install script, docs, and CI links all updated to match
- Local dev scripts (`development/build_images.sh`, `development/push_images.sh`, `integration-tests/test.sh`) now build/push/pull integration test images under `ghcr.io/krameff` instead of the old `aelsabbahy` Docker Hub namespace
- `docs/changelog.md` now points readers at this file and the releases page instead of saying no changelog exists
- Release binaries are now uncompressed (no more `tar.gz`/`zip` wrapper) and renamed to Go-native `goss-<os>-<arch>` (e.g. `goss-linux-amd64`, `goss-windows-amd64.exe`) instead of `goss_<version>_<os>_<arch>.tar.gz`; `install.sh` and `docs/installation.md` updated to match

### Added

- Release checksums (`SHA256SUMS`) are now GPG-signed 
- public key published as `krameff-goss-key.asc` at the repo root and attached to every release
- `docs/installation.md` documents how to import the signing key and verify a release's checksum signature
- goss -v now shows Krameff Solutions Ltd on the next line

---

3rd July 2026

### Added

- GoReleaser configuration for release builds ([#1052](https://github.com/goss-org/goss/pull/1052); thanks to [@kgaughan](https://github.com/kgaughan))
  - Cross-platform binary archives (`tar.gz` on Unix, `zip` on Windows) with SHA256 checksums
  - Multi-arch container images (`linux/amd64`, `linux/arm64`) published to `ghcr.io/<owner>/goss` with SBOMs
  - README documents local `goreleaser build` usage

### Updated

- Release workflow runs GoReleaser instead of `make release` and manual artifact upload; includes QEMU/Buildx for multi-platform Docker builds; images publish to `ghcr.io/${{ github.repository_owner }}/goss`
- `docker-goss.yaml` builds branch images via GoReleaser snapshot binaries instead of the removed multi-stage Dockerfile compile; tag pushes no longer duplicate release image builds
- `Dockerfile` simplified to copy the GoReleaser-built binary via `$TARGETPLATFORM`
- `install.sh` downloads compressed release archives and supports `s390x`; uses case-based architecture detection ([#1068](https://github.com/goss-org/goss/pull/1068))
- `make lint` and `make vet` now fail on violations instead of silently succeeding (`|| true` removed)
- Added `make pre-commit` (fmt, vet, unit tests) and `make pre-push` (fmt, vet, lint, `check`) targets for local testing before committing/pushing
- Added optional git hook (`.githooks/pre-commit`) that runs gofmt/vet/tests scoped to staged Go packages; enable via `git config core.hooksPath .githooks`
- `docs/testing.md` and `.github/CONTRIBUTING.md` document the new pre-commit/pre-push workflow
- CI's `integration-test-other` job (`macos-latest`, `windows-latest`) now runs `make test` before the integration tests, so Windows/macOS-only unit tests (e.g. `system/command_windows_test.go`) actually execute in CI
- README documents the `port` resource's Linux-only support and the new netstat parse-error behaviour, including how to investigate it

### Fixed

- `TestDiscoverFlagOverridesInline` and `TestValidateDiscoverWithDependsOn` no longer hardcode `/etc/hosts`, which doesn't exist on Windows; they now use a sentinel file created in `t.TempDir()`, fixing `found 0 tests` failures now that `make test` runs on Windows CI. `TestValidateWithDiscoverFlag` and `TestValidateInlineDiscovery` still assert against the `/etc/hosts`-based example fixture (also used by docs and the Linux-only discovery e2e test) and are skipped on non-Linux
- File `contents` checks now report the actual file content on failure instead of `"object: *bytes.Reader"` ([#1055](https://github.com/goss-org/goss/pull/1055); thanks to [@ckbaker10](https://github.com/ckbaker10))
- `have-patterns` matcher now honours trailing regex flags such as `/i`, `/m`, and `/s` on `/pattern/flags` style patterns ([#1057](https://github.com/goss-org/goss/pull/1057); thanks to [@ckbaker10](https://github.com/ckbaker10))
- Windows integration test service check uses `EventLog` instead of `MSDTC`; MSDTC is often stopped on GitHub Actions `windows-latest` runners despite being enabled
- `have-patterns` regex detection uses `strings.TrimPrefix` for optional negation prefix (staticcheck S1017)
- `port` resource no longer silently discards errors from the underlying netstat backend (`system.GetPorts`); a `/proc/net/{tcp,udp,tcp6,udp6}` line that fails to parse now fails the check with an explicit error instead of the port being reported as not listening

---

1st July 2026

### Updated

- Error handling uses static sentinel errors and `%w` wrapping where appropriate, enabling `errors.Is` / `errors.As` for callers ([#1066](https://github.com/goss-org/goss/pull/1066); thanks to [@kgaughan](https://github.com/kgaughan))

### Security

- `make check` and CI now run dependency scans (`govulncheck` plus Trivy on `go.mod` and docs Python requirements)
- CodeQL workflow added for Go static analysis on pull requests
- Docs build dependency `pygments` bumped to 2.20.0 (CVE-2026-4539)

### Fixed

- HTTP checks now close response bodies after validation, preventing connection leaks and OOM when running many concurrent HTTP tests ([#1058](https://github.com/goss-org/goss/pull/1058); thanks to [@dukelion](https://github.com/dukelion))

---

25th June 2026

### Updated

- Workflows updated to march branch naming

### Added

- **Discovery** — run lightweight checks before the main suite and expose results as template variables
  - ([#784](https://github.com/goss-org/goss/issues/784); thanks to [@uk-bolly](https://github.com/uk-bolly) for raising the issue
  - thanks to [@ekelali](https://github.com/ekelali) for the [`--vars` + `--format discovery` pipeline suggestion](https://github.com/goss-org/goss/issues/784#issuecomment-1251529683))
  - `--discover <file>` runs discovery tests before the main gossfile and injects `.Discovered` for templates
  - Inline `discovery:` in the main `-g` file is also supported; `--discover` wins when both are set
  - `--format discovery` still exports `{"Discovered": {...}}` for tooling and CI vars files
  - Examples: [`integration-tests/goss/examples/discovery/`](integration-tests/goss/examples/discovery/)
- **`depends-on`** — declare test prerequisites on any resource; dependents are **skipped** (not failed) when a prerequisite test fails
  - ([#1043](https://github.com/goss-org/goss/issues/1043); thanks to [@petkapou](https://github.com/petkapou) for raising the issue
  - thanks to [@sshipway](https://github.com/sshipway) for the [explicit `depends-on` design feedback](https://github.com/goss-org/goss/issues/1043#issuecomment-3765767824))
  - References use the gossfile map key, or `type:key` when the key is ambiguous across resource types
  - Independent chains still run in parallel; only declared dependencies are serialized
  - Composes with discovery: templates can gate which tests exist; `depends-on` orders and skips among tests in the main run
  - Examples: [`integration-tests/goss/examples/depends-on/`](integration-tests/goss/examples/depends-on/), discover+depends-on in [`goss-with-deps.yml`](integration-tests/goss/examples/discovery/goss-with-deps.yml)

---

24th June 2026
### Security
- CLI announce output logs resource type and ID only; no longer marshals and prints full resource JSON (fixes CodeQL clear-text logging of password and other sensitive fields)
- Health probe debug logging records HTTP status only; no longer logs response body on non-OK status (avoids clear-text logging of password and other sensitive fields from test output)
- Health probe content negotiation errors no longer log the raw `Accept` header value from untrusted requests

### Fixed
- Integration test `add.goss.yaml` expectations updated for announce output change (resource type and ID only, not full marshaled YAML)
- `bullseye` apache2 version bumped to `2.4.67-1~deb11u3` in `vars.yaml` (Debian security update)
- Integration test fixtures: removed duplicate `service: apache2` / `service: httpd` definitions between `goss-shared.yaml` and `goss-service.yaml`; eliminates duplicate-key warnings during `validate` in CI

### Added

- Multiple `--vars` files supported; vars are merged in flag order with later files overriding overlapping keys; `--vars-inline` still applies last ([#1023](https://github.com/goss-org/goss/issues/1023); thanks to [@Lirt](https://github.com/Lirt) for [PR #1024](https://github.com/goss-org/goss/pull/1024))

### PRs Incorporated

- Thanks to [@kgaughan](https://github.com/kgaughan) for authoring the `urfave/cli` v3 migration
  - [#1060](https://github.com/goss-org/goss/pull/1060) - CLI migrated from `urfave/cli` v1 to `urfave/cli/v3` v3.9.0; v3 is actively maintained and drops transitive dependencies (`go-md2man`, `blackfriday`)
- Thanks to [@kgaughan](https://github.com/kgaughan) for restoring clearer `ContainElements` matcher error messages
  - [#1067](https://github.com/goss-org/goss/pull/1067) - pre-validates array/slice/map types before delegating to gomega, restoring the pre-iterator error text for invalid inputs (e.g. strings)
- Thanks to [@kgaughan](https://github.com/kgaughan) for the dependency and tooling refresh
  - [#1064](https://github.com/goss-org/goss/pull/1064) - golangci-lint v2.12.2 config (staticcheck settings, `noinlineerr` disabled), dependency bumps (`gomega` v1.41.0, `prometheus/common` v0.68.1), Trivy action updates, and minor lint cleanups; fork already had most workflow, Dockerfile, and code changes at equal or newer versions

- updated workflows
  - actions/checkout@v6.0.3 to 7.0.0
  - softprops/action-gh-release@v3.0.0 to softprops/action-gh-release@v3.0.1
  - Added permissions
---

## [0.5.0] - 2026-06-09

### Security
- `Dockerfile` final stage bumped from `alpine:3.19` to `alpine:3.21`; resolves CVE-2026-40200, CVE-2026-6042 (`musl-utils`) and CVE-2025-46394, CVE-2024-58251 (`ssl_client`/busybox)
- `Dockerfile` build stage `GO_VERSION` updated from `1.22` to `1.26`
- `SECURITY.md` added with vulnerability reporting policy and response timeline
- `CODE_OF_CONDUCT.md` added based on Contributor Covenant 2.1

### Added
- `linux/arm64` integration tests via native `ubuntu-24.04-arm` GitHub Actions runner
- `linux/ppc64le` integration tests via QEMU binfmt_misc emulation on `ubuntu-latest`
- `linux/ppc64le` binary added to release builds
- `integration-tests/goss/linux-arm64/` and `integration-tests/goss/linux-ppc64le/` test directories covering commands, addr, dns, file, group, kernel-param, http, and process resources
- `docker/setup-qemu-action` in CI to support transparent ppc64le binary execution without a container wrapper
- `.claude/agents/test-runner.md` agent for running integration tests in parallel

### PRs Incorporated

- [#1013](https://github.com/goss-org/goss/pull/1013) -- `http` resource: `request-query-params` added to support URL query parameters with proper encoding and duplicate key support; thanks to [@riton](https://github.com/riton)
- [#1017](https://github.com/goss-org/goss/pull/1017) -- `service` resource: Windows service checks implemented via PowerShell (`Get-Service`); supports `enabled`, `running`, and `exists`; `skip: true` removed from Windows integration test; thanks to [@cthiel42](https://github.com/cthiel42)
- [#1053](https://github.com/goss-org/goss/pull/1053) -- `registry` resource added for Windows registry validation; supports `exists`, `value`, and `type` checks; uses native `golang.org/x/sys/windows/registry` API (no PowerShell); five hives supported (HKLM, HKCU, HKCR, HKU, HKCC); six data types (REG_SZ, REG_EXPAND_SZ, REG_DWORD, REG_QWORD, REG_BINARY, REG_MULTI_SZ); `::` separator for value names containing backslashes; thanks to [@Blankf](https://github.com/Blankf)

### Changed
- `CODEOWNERS` updated to `@uk-bolly`
- `dependabot.yml` -- `docker` ecosystem added for Dockerfile base image tracking; assignees and reviewers added to all entries; `open-pull-requests-limit: 0` removed from gomod entry
- `ISSUE_TEMPLATE` -- all templates assigned to `uk-bolly`; `config.yml` added to redirect security reports to GitHub private vulnerability reporting
- `pull_request_template.md` -- `make test-all` corrected to `make test`
- `run-validate-tests.sh` Linux block narrowed to `linux-amd64` only; non-amd64 Linux architectures now testable via this path
- All platform command test files normalised: `--use-alpha=1` removed from exec commands (env var `GOSS_USE_ALPHA=1` is sufficient); `help.goss.yaml` stdout check unified to `validate` across all platforms
- Per-distro Makefile targets now depend on `release/goss-linux-amd64` only instead of the full `build` target

### Fixed
- `bullseye` apache2 updated to `2.4.67-1~deb11u2` in `vars.yaml` and expected files
- `generate_goss.sh` strips `::1` from generated localhost DNS entry; docker injects `::1` but podman does not, causing golden file mismatches in CI
- Service key order corrected for `rockylinux9` and `almalinux10`: `httpd` before `webservice` (alphabetical)
- `Dockerfile_jammy` and `Dockerfile_bullseye`: tinyproxy overridden to `Type=simple` with foreground mode; fixes startup failure in Docker CI due to `Type=forking` and `PrivateDevices=yes`

---

## [0.4.0] - Initial fork from goss-org/goss

### Documentation
- `README.md` updated to show origins, credits, and Apache 2.0 license retention

### Release pipeline
- `release.yaml` release tag env var standardised as `RELEASE_TAG`
- `release.yaml` `attach-assets` job file glob corrected to match actual download paths
- `release-build.sh` fixed so `-p` flag correctly sets target platform, `os`, `arch`, and output filename
- `Makefile` release rule updated to pass `-p` and `-v` flags to `release-build.sh`

### CI
- `integration-test-linux-arm64` job added using native `ubuntu-24.04-arm` GitHub Actions runner; tests `goss-linux-arm64` binary via `run-validate-tests.sh` and `run-serve-tests.sh`
- `integration-test-linux-ppc64le` job added using QEMU binfmt_misc emulation on `ubuntu-latest`; `docker/setup-qemu-action` registers ppc64le handlers so the binary runs transparently without a container wrapper
- `run-validate-tests.sh` Linux block narrowed from all Linux to `linux-amd64` only; non-amd64 Linux architectures are now testable via this path
- `linux-arm64/` and `linux-ppc64le/` test directories added under `integration-tests/goss/`; cover commands, addr, dns, file, group, kernel-param, http, process, and gossfile resources
- `linux-arm64/commands/add.goss.yaml` and `linux-ppc64le/commands/add.goss.yaml` -- `add addr 127.0.0.1` (no port) marked `skip: true`; the format behaves differently on Linux vs Darwin
- All platform command test files (`darwin-amd64`, `darwin-arm64`, `linux-arm64`, `linux-ppc64le`, `windows`) normalised for consistency: `--use-alpha=1` removed from all `exec` commands (env var `GOSS_USE_ALPHA=1` set by `run-validate-tests.sh` is sufficient); `help.goss.yaml` stdout check changed from `alpha` to `validate` across all platforms
- `bullseye` apache2 version updated to `2.4.67-1~deb11u2` in `vars.yaml`, `goss-expected.yaml`, and `goss-aa-expected.yaml`
- `macos-13` (Intel) removed from CI matrix -- deprecated and no longer available on GitHub Actions; Apple Silicon testing continues via `macos-latest`
- Legacy CI config removed; GitHub Actions is the sole CI pipeline
- `docs.yaml` lint job re-enabled; build/deploy remains disabled
- `preview-docs.yaml` disabled
- `dependabot.yml` assignee and reviewer updated to `uk-bolly`
- `docker-integration-tests` workflow build context corrected to `integration-tests/`

### Security
- `Dockerfile` final stage bumped from `alpine:3.19` to `alpine:3.21`; resolves CVE-2026-40200 and CVE-2026-6042 (`musl-utils`) and CVE-2025-46394 and CVE-2024-58251 (`ssl_client`/busybox), all fixed in Alpine 3.21
- `Dockerfile` build stage `GO_VERSION` default updated from `1.22` to `1.26` to match `go.mod` requirement

### Build targets
- `linux/ppc64le` binary added to release builds
- `darwin/arm64` (Apple Silicon) binary added to release builds
- Removed 32-bit build and testing support

### Markdown lint fixes
- `README.md` attribution blockquote moved below heading (MD041); long lines wrapped (MD013); `[here]` link text made descriptive (MD059)
- `docs/gossfile.md` long admonition line wrapped (MD013); `[here]` link text made descriptive (MD059)
- `extras/dgoss/README.md` long line wrapped (MD013)
- `extras/kgoss/README.md` table pipe separators spaced correctly (MD060)

### Go and dependencies
- Updated to Go 1.26
- Replaced `github.com/achanda/go-sysctl` with `github.com/lorenzosaino/go-sysctl v0.3.1`
- Upgraded `github.com/BurntSushi/toml` v1.3.2 => v1.6.0
- Upgraded `golang.org/x/exp/typeparams`, `golang.org/x/lint`, `honnef.co/go/tools`
- GitHub Actions updated to later versions
- 386 build targets removed

### Linter
- `.golangci.yaml` migrated from v1 to v2 format
- CI updated to golangci-lint v2.12.2
- `go install` of golangci-lint removed from Makefile
- Error strings lowercased throughout to comply with ST1005
- Golden files updated for `TestMatchers` (`iter.Seq/iter.Seq2` support)
- `semver_constraint_test.go` assertions updated to match lowercased strings

### Integration tests
- `generate_goss.sh` strips `::1` from the generated localhost DNS entry after `goss add`; docker injects `::1` into `/etc/hosts` but podman does not, causing golden file mismatches
- `goss-expected.yaml` and `goss-expected-q.yaml` localhost DNS entries updated to remove `::1` across all distros to match the normalised generated output
- `goss-expected.yaml` and `goss-expected-q.yaml` service key order corrected for `rockylinux9` and `almalinux10`: `httpd` before `webservice` (alphabetical, matching goss output)
- `Dockerfile_jammy`: tinyproxy service overridden to `Type=simple` with foreground mode (`-d`), `ExecStartPre` creates `/run/tinyproxy`, and `PrivateDevices=no`; fixes tinyproxy failing to start in docker CI due to `Type=forking` PID file races and private device namespace restrictions
- `Dockerfile_jammy`: `/var/log/tinyproxy` pre-created with correct ownership; required by tinyproxy 1.11.0 on Ubuntu Jammy
- `Dockerfile_bullseye`: same tinyproxy service override applied as jammy; identical root cause
- `Makefile`: per-distro targets (`jammy`, `bullseye`, `rockylinux9`, etc.) now depend on `release/goss-linux-amd64` only instead of the full `build` target; avoids cross-compiling Windows and Darwin binaries for Linux integration tests
- `goss-expected.yaml` updated for `rockylinux9`, `almalinux10`, `bullseye`, `jammy`, and `alpine3` to include `::1` in `localhost` DNS addresses
- `goss-shared.yaml` User-Agent regex relaxed to match any goss version string, not just strict semver
- `goss-service.yaml` test service renamed from `foobar` to `webservice`; all distro `goss-expected.yaml` files and `generate_goss.sh` updated to match
- Redundant `bypath: goss-dummy.yaml` removed from all distro `goss.yaml` files; `goss-shared.yaml` is the single import point
- Thanks to [@kgaughan](https://github.com/kgaughan) for the integration test infrastructure overhaul
  ([PR #1061](https://github.com/goss-org/goss/pull/1061))
- External `dnstest.io` dependency replaced with a local dnsmasq zone on `127.0.0.1:8053`, making DNS tests self-contained
- Debian Bullseye and Ubuntu Jammy added with full test suites
- Ubuntu Trusty and Debian Wheezy removed (end of life); `.md5` sidecar files removed
- Alpine upgraded to 3.20; dnsmasq added
- Arch Linux dnsmasq and tinyproxy added
- RockyLinux 9 dnsmasq added
- `integration-test` CI job split into `integration-test-linux` and `integration-test-other`
- `integration-test-linux` further split into a per-distro matrix so each distro runs as an independent CI job
- AlmaLinux 10 integration test support added
- Integration test directories split by arch: `darwin/` renamed to `darwin-amd64/`; `darwin-arm64/` added
- CI matrix extended with `macos-13` (Intel) alongside `macos-latest` (Apple Silicon)
- `CentOS 7` yum redirected to `vault.centos.org` after EOL decommission
