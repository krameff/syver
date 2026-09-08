# Testing

This page describes how to run the Syver test suite locally and how those checks map to CI.

To see how many tests there are and whether they pass, run them:

```bash
go test -race ./...
```

This page deliberately does not print a passing count. One was published here
and went stale within a release, which is worse than no number at all: a reader
has no way to tell a figure that is merely old from one that is wrong.

## Quick start (local)

One-time setup so the pre-commit hook runs automatically:

```bash
git config core.hooksPath .githooks
```

This runs `gofmt`, `go vet`, and `go test` scoped to whatever Go packages have staged
changes on every `git commit` (see [`.githooks/pre-commit`](https://github.com/krameff/syver/blob/main/.githooks/pre-commit)).

Before pushing / opening a PR, run the fuller local bundle (mirrors both CI jobs —
lint + coverage):

```bash
make pre-push
```

`pre-push` runs `fmt`, `vet`, `lint` (strict — no longer swallows failures), then
`check` (unit tests, discovery E2E, depends-on E2E, markdown lint, security scan).
Individual targets:

```bash
make fmt
make vet
make lint
go test ./...
make test-discovery-e2e
make test-depends-on-e2e
make lint-markdown
make test-security
```

On macOS/Windows, `make test-discovery-e2e` builds a temporary syver binary and uses
`GOSS_USE_ALPHA=1` automatically. On Linux CI it uses `release/syver-linux-amd64`.

## Make targets

| Target | Command | Purpose |
| --- | --- | --- |
| `pre-commit` | `fmt vet` + `go test ./...` | Fast local check (also runs scoped via git hook) |
| `pre-push` | `fmt vet lint check` | Full local bundle before pushing / opening a PR |
| `check` | `test` + discovery/depends-on E2E + `lint-markdown` + `test-security` | PR check bundle |
| `test` | `./ci/go-test.sh` | Unit tests with coverage profile (`c.out`) |
| `cov` | `go test -coverpkg=./... ./...` | Coverage run (used in CI) |
| `test-discovery-e2e` | `./ci/discovery-e2e.sh` | `--discover` pipeline (flag, inline, discover+depends-on) |
| `test-depends-on-e2e` | `./ci/depends-on-e2e.sh` | `depends-on` skip when prerequisite fails |
| `lint-markdown` | `./ci/lint-markdown.sh` | Markdownlint on docs and README files |
| `test-security` | `./ci/security-scan.sh` | `govulncheck` + Trivy scan of `go.mod` and `docs/requirements.txt`; **fails on findings** |
| `lint-yaml` | `yamllint` | YAML lint for integration-test gossfiles |
| `test-short-all` | `fmt lint vet test` | Formatter, golangci-lint, vet, unit tests |
| `test-int-*` | Docker / platform scripts | Full integration matrix (slow) |

`lint` and `vet` now fail the build on violations (previously both swallowed errors
with `|| true`), so `test-short-all`, `pre-push`, and CI's separate lint job agree.

## CI workflows

| Workflow | Job | Tests run |
| --- | --- | --- |
| [`.github/workflows/golangci.yaml`](https://github.com/krameff/syver/blob/main/.github/workflows/golangci.yaml) | `lint` | golangci-lint |
| | `coverage` | `make cov`, **`make test-discovery-e2e`**, **`make test-depends-on-e2e`**, **`./ci/security-scan.sh`**, **`./ci/trivyignore-check.sh`** |
| | `integration-test-*` | `make rockylinux9`, `jammy`, darwin, windows, etc. (includes discovery + depends-on E2E) |
| [`.github/workflows/codeql.yml`](https://github.com/krameff/syver/blob/main/.github/workflows/codeql.yml) | `analyze` | CodeQL static analysis for Go and GitHub Actions workflows |
| [`.github/workflows/docs.yaml`](https://github.com/krameff/syver/blob/main/.github/workflows/docs.yaml) | `lint` | markdownlint-cli2 on docs |
| [`.github/workflows/yamllint.yaml`](https://github.com/krameff/syver/blob/main/.github/workflows/yamllint.yaml) | — | YAML lint |

## Discovery E2E

Script: [`ci/discovery-e2e.sh`](https://github.com/krameff/syver/blob/main/ci/discovery-e2e.sh)

Fixtures: [`integration-tests/syver/examples/discovery/`](https://github.com/krameff/syver/tree/main/integration-tests/syver/examples/discovery/)

```bash
make test-discovery-e2e
```

Steps performed:

1. `goss validate -g discovery.yaml --format discovery` → JSON with `Discovered` key
2. `goss validate -g goss.yml --discover discovery.yaml --format documentation` → `Failed: 0`
3. `goss validate -g goss-inline.yml --format documentation` → inline `discovery:` → `Failed: 0`
4. `goss validate -g goss-with-deps.yml --discover discovery.yaml` → `Failed: 1`, `Skipped: 1`

Manual equivalent:

```bash
goss validate -g integration-tests/syver/examples/discovery/goss.yml \
  --discover integration-tests/syver/examples/discovery/discovery.yaml \
  --format documentation
```

Export-only (unchanged):

```bash
goss validate -g integration-tests/syver/examples/discovery/discovery.yaml --format discovery \
  > /tmp/discovered.json
goss --vars /tmp/discovered.json \
  validate -g integration-tests/syver/examples/discovery/goss.yml --format documentation
```

## Depends-on E2E

Script: [`ci/depends-on-e2e.sh`](https://github.com/krameff/syver/blob/main/ci/depends-on-e2e.sh)

Fixtures: [`integration-tests/syver/examples/depends-on/`](https://github.com/krameff/syver/tree/main/integration-tests/syver/examples/depends-on/)

```bash
make test-depends-on-e2e
```

Steps performed:

1. Validate a gossfile where a file check fails and a command depends on it
2. Asserts `Failed: 1` (prerequisite) and `Skipped: 1` (dependent via `depends-on`)

## Docker integration matrix

Script: [`integration-tests/test.sh`](https://github.com/krameff/syver/blob/main/integration-tests/test.sh)

Each distro target (`make rockylinux9`, `make jammy`, etc.) runs the main gossfile validate suite, then
reuses the same steps as the host E2E scripts via [`ci/lib/syver-e2e-steps.sh`](https://github.com/krameff/syver/blob/main/ci/lib/syver-e2e-steps.sh):

* `run_discovery_e2e_steps` — `--format discovery`, `--discover`, inline `discovery:`, discover+depends-on
* `run_depends_on_e2e_steps` — pure `depends-on` skip semantics

Fixtures live under
[`integration-tests/syver/examples/`](https://github.com/krameff/syver/tree/main/integration-tests/syver/examples/)
and are
mounted at `/goss/examples/` inside the test container.

## Non-amd64 and non-Linux platform fixtures

Script: [`integration-tests/run-validate-tests.sh`](https://github.com/krameff/syver/blob/main/integration-tests/run-validate-tests.sh)

Platforms with no test container of their own -- macOS, Windows, and Linux on
arm64 and ppc64le -- run their fixtures directly against a release binary rather
than through Docker. Each fixture under
`integration-tests/syver/<platform>/` is validated in turn.

A fixture declares what it expects with comment directives, read from the file
itself so that whoever writes the fixture cannot forget to register the
expectation somewhere else:

| Directive | Meaning | Default |
| --- | --- | --- |
| `# expect-exit: N` | exit code the validate must return | `0` |
| `# expect-count: N` | total assertions the fixture must produce | unchecked |
| `# expect-skipped: N` | how many of those must be skipped | unchecked |

The exit code alone answers one question -- did anything fail. It cannot see an
assertion that stopped existing, and it cannot see one that turned into a skip,
because a skipped assertion never fails. Over a third of these fixtures use
`skip: true`, so a suite that quietly got smaller would still report a clean
pass. `expect-count` and `expect-skipped` close that.

`Failed` is deliberately not pinned: it depends on the host, which is what the
exit code is for. `Count` is a property of the fixture and is pinned everywhere.

`Skipped` is pinned on macOS and Linux but **not** on Windows, and the reason is
worth knowing because it is not obvious. Skips are not purely declarative: a
resource whose existence check fails has its remaining attributes reported as
*skipped* rather than failed, so one missing file turns five further assertions
into skips. The Windows fixtures therefore skip 33 assertions when driven from a
Linux host and 19 on a real Windows host, where the files and registry keys
actually exist. Seed that value from a run on the platform itself, never by
inference from another one.

That cascade is also what makes `expect-skipped` worth pinning at all: a
resource that quietly stops being present raises the skip count without failing
anything, which is precisely the case the exit code cannot see.

Every run ends with a line naming the platform, the number of fixtures, the
total assertions and the total skipped. Each platform job in CI is named
identically and renders as an identical green tick while the suites behind them
differ by close to an order of magnitude, and that line is what tells a reader
which green they are looking at.

## Go unit and integration tests

### Package `github.com/krameff/syver` (root)

| Test | File | Covers |
| --- | --- | --- |
| `TestDiscoveryConfigEntries` | `discovery_config_test.go` | Discovery `register` parsing |
| `TestDiscoveryConfigRequiresRegister` | `discovery_config_test.go` | Missing `register` error |
| `TestDiscoveredFromVars` | `discovery_config_test.go` | `.Discovered` vars extraction |
| `TestBuildScheduleDependsOn` | `discovery_config_test.go` | Dependency graph building |
| `TestBuildScheduleDetectsCycles` | `discovery_config_test.go` | Cycle detection |
| `TestValidateDiscoveryFormat` | `discovery_integration_test.go` | `--format discovery` end-to-end |
| `TestValidateWithDiscoverFlag` | `discovery_integration_test.go` | `--discover` pre-run + templated main |
| `TestValidateInlineDiscovery` | `discovery_integration_test.go` | Inline `discovery:` in main gossfile |
| `TestDiscoverFlagOverridesInline` | `discovery_integration_test.go` | `--discover` wins over inline |
| `TestValidateDiscoverWithDependsOn` | `discovery_integration_test.go` | `--discover` + `depends-on` skip semantics |
| `TestValidateDependsOnSkipsDependent` | `discovery_integration_test.go` | `depends-on` skip semantics |
| `TestTemplateDiscoveredVars` | `discovery_integration_test.go` | Template `.Discovered` rendering |
| `TestMergePreservesDiscovery` | `discovery_merge_test.go` | Discovery survives gossfile merge |
| `TestConfigMerge` | `syver_test.go` | Config merge behaviour |
| `TestUseAsPackage` | `syver_test.go` | Programmatic validate API |
| `TestSkipResourcesByType` | `syver_test.go` | Disabled resource types |
| `TestServeWithNoContentNegotiation` | `serve_test.go` | Health endpoint output |
| `TestServeNegotiatingContent` | `serve_test.go` | Accept header negotiation |
| `TestServeCacheWithNoContentNegotiation` | `serve_test.go` | Serve cache behaviour |
| `TestServeCacheNegotiatingContent` | `serve_test.go` | Serve cache + negotiation |
| `Test_varsFromString` | `store_test.go` | Inline vars parsing |
| `Test_loadVars` | `store_test.go` | Vars file merge |

### Package `matchers`

| Test | File | Covers |
| --- | --- | --- |
| `TestBeSemverConstraint` | `semver_constraint_test.go` | Semver matcher |
| `TestBeSemverConstraintMatcher_*` | `semver_constraint_test.go` | Matcher messages |
| `Test_toConstraint` / `Test_toVersion` / `Test_toVersions` | `semver_constraint_test.go` | Version parsing |

### Package `outputs`

| Test | File | Covers |
| --- | --- | --- |
| `TestDiscoveryOutput` | `discovery_test.go` | `--format discovery` JSON shape |
| `TestIsValidFormat` | `outputs_test.go` | Output format validation |
| `TestOutputers` | `outputs_test.go` | Registered formatters |
| `TestGetOutputer` | `outputs_test.go` | Formatter lookup |
| `TestOutputFormatOptions` | `outputs_test.go` | Format options |
| `TestOptionsRegistration` | `outputs_test.go` | Option registration |
| `TestPrometheusOutput` | `prometheus_test.go` | Prometheus metrics output |
| `TestCanChangeOverallOutcome` | `prometheus_test.go` | Outcome aggregation |

### Package `resource`

| Test | File | Covers |
| --- | --- | --- |
| `TestMatcherToGomegaMatcher` | `gomega_test.go` | Matcher conversion |
| `TestValidateValue` | `validate_test.go` | Property validation |
| `TestValidateValueErr` | `validate_test.go` | Validation errors |
| `TestValidateValueSkip` | `validate_test.go` | Skip results |
| `TestValidateContains*` | `validate_test.go` | Contains matcher |
| `TestResultMarshaling` | `validate_test.go` | TestResult JSON/YAML |
| `BenchmarkValidateValue` | `validate_test.go` | Validation performance |
| `TestNewProcess` | `process_test.go` | `process` resource construction (incl. `status`/`user`, ignore-list) |
| `TestProcessValidate` | `process_test.go` | `process` resource `Validate()` (running/status/user, skip-on-fail) |
| `TestNewPort` | `port_test.go` | `port` resource construction (incl. `pid`, ignore-list) |
| `TestPortValidate` | `port_test.go` | `port` resource `Validate()` (listening/ip/pid, skip-on-fail) |

### Package `system`

| Test | File | Covers |
| --- | --- | --- |
| `TestCommandWrapper` | `command_posix_test.go` / `command_windows_test.go` | Command execution |
| `TestParseServerString` | `dns_test.go` | DNS server parsing |
| `TestSplitMountInfo` | `mount_test.go` | Mount info parsing |
| `TestIsSupportedPackageManager` | `package_test.go` | Package manager detection |
| `TestParseRegistryKey` | `registry_test.go` | Windows registry keys |
| `TestPackageManager` | `system_test.go` | Package manager integration |
| `TestDetectService` | `system_test.go` | Service detection |
| `TestDetectDistro` | `system_test.go` | Distro detection |
| `TestHasCommand` | `system_test.go` | Command availability |
| `TestGroupsForUser` | `user_group_unix_test.go` | User/group lookup |
| `TestGetProcs` | `process_test.go` | Process snapshot (gopsutil), skip-on-error per pid |
| `TestDefProcessRunning` | `process_test.go` | `running` lookup, found/not-found/error |
| `TestDefProcessStatus` | `process_test.go` | `status` aggregation/dedup across pids |
| `TestDefProcessUser` | `process_test.go` | `user` aggregation/dedup across pids |
| `TestDefProcessPids` | `process_test.go` | `Pids()` returns all matching pids |
| `TestGetPorts` | `port_test.go` | Port snapshot (gopsutil), per-protocol error isolation |
| `TestDefPortListening` | `port_test.go` | `listening` lookup, found/not-found/error |
| `TestDefPortIP` | `port_test.go` | `IP()` returns all bound IPs |
| `TestDefPortPID` | `port_test.go` | `PID()` returns owning pids, omits unresolved (0) |
| `TestNormalizePort` | `port_test.go` | Port string normalization (`tcp:`/`udp:` prefix) |

### Package `util`

| Test | File | Covers |
| --- | --- | --- |
| `TestWithVarsBytes` | `config_test.go` | Vars from bytes |
| `TestWithVarsString` | `config_test.go` | Inline vars |
| `TestWithVarsFiles` | `config_test.go` | Vars file list |
| `TestWithVarsFile` | `config_test.go` | Single vars file |
| `TestWithVarsData` | `config_test.go` | Vars data helper |

### Package `cmd/syver`

No Go tests — behaviour covered by root package API tests and integration tests.

## Docker integration tests

Linux distro matrix via [`integration-tests/test.sh`](https://github.com/krameff/syver/blob/main/integration-tests/test.sh):

```bash
make rockylinux9    # example: one distro
make test-int-all   # full matrix (slow)
```

Non-amd64 / darwin / windows via [`integration-tests/run-validate-tests.sh`](https://github.com/krameff/syver/blob/main/integration-tests/run-validate-tests.sh)
(find `*.goss.yaml` under platform dirs and run `goss validate`).

## Markdown lint

```bash
make lint-markdown
```

Lints: `docs/**/*.md`, `README.md`, `extras/**/README.md`, `.github/CONTRIBUTING.md`

Configuration: [`.markdownlint.yaml`](https://github.com/krameff/syver/blob/main/.markdownlint.yaml)

## Security scan

Script: [`ci/security-scan.sh`](https://github.com/krameff/syver/blob/main/ci/security-scan.sh)

```bash
make test-security
```

Runs on every `make check` and in the `coverage` CI job after dependency or docs updates.

Steps performed:

1. **`govulncheck ./...`** — Go vulnerability database scan for the module and its dependencies
2. **Trivy filesystem scan** — checks `go.mod` and `docs/requirements.txt` for
   known CVEs at **MEDIUM** severity and above, scans the tree for committed
   secrets, and checks the `Dockerfile` for misconfigurations

Both scanners are pinned: Trivy by image digest, `govulncheck` by version. A
blocking gate should not float, and neither should code the gate executes.

**Findings fail the build.** Trivy previously ran without `--exit-code`, so it
printed CVEs and still exited 0, which meant `make check` and `make pre-push`
passed with vulnerabilities on screen. Fix the dependency, or add a documented
`.trivyignore.yaml` entry if there is genuinely no fix — `ci/trivyignore-check.sh`
re-validates those. Suppressions live in `.trivyignore.yaml`, which records a
`statement` for each entry and supports `expired_at` for anything expected to be
fixed upstream. Trivy does not auto-detect that filename, so the scripts pass
`--ignorefile` explicitly. It runs on every PR (reporting only), strictly on the
weekly `trivy-schedule.yaml` run, and via `.githooks/pre-commit` on any change
to `go.mod`, `go.sum` or `.trivyignore.yaml` if you have opted in with
`git config core.hooksPath .githooks`. It is deliberately non-blocking on PRs:
a stale suppression is not a vulnerability, and it depends on Trivy's database,
which cannot be pinned the way the scanner version is. To
inspect findings without failing, set `SYVER_TRIVY_EXIT_CODE=0`; the summary
line says explicitly when enforcement is off.

Locally, Trivy runs via the `trivy` binary if installed, otherwise via a
container runtime (Docker or Podman, `aquasec/trivy` pinned by digest). If
neither is available, the scan is skipped with a warning unless
`SECURITY_STRICT=1` (always set in CI).

The scan distinguishes three failure modes in its summary line, because they
need different responses: findings (exit 2), scanner failed to run such as an
unreachable vulnerability DB (exit 1, nothing was scanned), and skipped.

Docker image scanning (Alpine packages and compiled binary) continues to run in
[`.github/workflows/docker-syver.yaml`](https://github.com/krameff/syver/blob/main/.github/workflows/docker-syver.yaml)
and
[`.github/workflows/trivy-schedule.yaml`](https://github.com/krameff/syver/blob/main/.github/workflows/trivy-schedule.yaml).

## CodeQL

Workflow: [`.github/workflows/codeql.yml`](https://github.com/krameff/syver/blob/main/.github/workflows/codeql.yml)

Runs on pull requests and pushes to `devel`, plus a weekly schedule.
Uses GitHub's advanced CodeQL setup for Go and Actions with category
`/language:<language>` so PRs can be compared against the base branch.

CodeQL runs in CI only (not part of `make check`).

If the repository still has **CodeQL Default setup** enabled under **Settings → Code
security**, disable it in favour of this workflow. Default setup and an advanced workflow
cannot run together and will produce "configuration not found" warnings on pull requests.

## Adding tests for new features

* **Discovery / depends-on**: add cases to `discovery_*_test.go` and extend
  [`integration-tests/syver/examples/discovery/`](https://github.com/krameff/syver/tree/main/integration-tests/syver/examples/discovery/)
* **Output formats**: add to `outputs/*_test.go`
* **Resource types**: add to `resource/validate_test.go` and platform integration gossfiles
* **CLI behaviour**: prefer root `syver_test.go` or integration command gossfiles under
  `integration-tests/syver/<platform>/commands/`

PRs should include automated tests; discovery changes should keep `make test-discovery-e2e` passing.
