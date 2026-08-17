# Changelog

## 0.7.0 based on v0.6.0

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
  - tests 282 -> 317
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
