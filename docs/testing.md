# Testing

This page describes how to run the Syver test suite locally and how those checks map to CI.

Last verified: 2026-08-16 — **317** Go test cases passing, discovery E2E passing, markdown lint clean.

## Quick start (local)

One-time setup so the pre-commit hook runs automatically:

```bash
git config core.hooksPath .githooks
```

This runs `gofmt`, `go vet`, and `go test` scoped to whatever Go packages have staged
changes on every `git commit` (see [`.githooks/pre-commit`](../.githooks/pre-commit)).

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
| `test-security` | `./ci/security-scan.sh` | `govulncheck` + Trivy scan of `go.mod` and `docs/requirements.txt` |
| `lint-yaml` | `yamllint` | YAML lint for integration-test gossfiles |
| `test-short-all` | `fmt lint vet test` | Formatter, golangci-lint, vet, unit tests |
| `test-int-*` | Docker / platform scripts | Full integration matrix (slow) |

`lint` and `vet` now fail the build on violations (previously both swallowed errors
with `|| true`), so `test-short-all`, `pre-push`, and CI's separate lint job agree.

## CI workflows

| Workflow | Job | Tests run |
| --- | --- | --- |
| [`.github/workflows/golangci.yaml`](../.github/workflows/golangci.yaml) | `lint` | golangci-lint |
| | `coverage` | `make cov`, **`make test-discovery-e2e`**, **`make test-depends-on-e2e`**, **`./ci/security-scan.sh`** |
| | `integration-test-*` | `make rockylinux9`, `jammy`, darwin, windows, etc. (includes discovery + depends-on E2E) |
| [`.github/workflows/codeql.yml`](../.github/workflows/codeql.yml) | `analyze` | CodeQL static analysis for Go and GitHub Actions workflows |
| [`.github/workflows/docs.yaml`](../.github/workflows/docs.yaml) | `lint` | markdownlint-cli2 on docs |
| [`.github/workflows/yamllint.yaml`](../.github/workflows/yamllint.yaml) | — | YAML lint |

## Discovery E2E

Script: [`ci/discovery-e2e.sh`](../ci/discovery-e2e.sh)

Fixtures: [`integration-tests/goss/examples/discovery/`](../integration-tests/goss/examples/discovery/)

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
goss validate -g integration-tests/goss/examples/discovery/goss.yml \
  --discover integration-tests/goss/examples/discovery/discovery.yaml \
  --format documentation
```

Export-only (unchanged):

```bash
goss validate -g integration-tests/goss/examples/discovery/discovery.yaml --format discovery \
  > /tmp/discovered.json
goss --vars /tmp/discovered.json \
  validate -g integration-tests/goss/examples/discovery/goss.yml --format documentation
```

## Depends-on E2E

Script: [`ci/depends-on-e2e.sh`](../ci/depends-on-e2e.sh)

Fixtures: [`integration-tests/goss/examples/depends-on/`](../integration-tests/goss/examples/depends-on/)

```bash
make test-depends-on-e2e
```

Steps performed:

1. Validate a gossfile where a file check fails and a command depends on it
2. Asserts `Failed: 1` (prerequisite) and `Skipped: 1` (dependent via `depends-on`)

## Docker integration matrix

Script: [`integration-tests/test.sh`](../integration-tests/test.sh)

Each distro target (`make rockylinux9`, `make jammy`, etc.) runs the main gossfile validate suite, then
reuses the same steps as the host E2E scripts via [`ci/lib/syver-e2e-steps.sh`](../ci/lib/syver-e2e-steps.sh):

* `run_discovery_e2e_steps` — `--format discovery`, `--discover`, inline `discovery:`, discover+depends-on
* `run_depends_on_e2e_steps` — pure `depends-on` skip semantics

Fixtures live under [`integration-tests/goss/examples/`](../integration-tests/goss/examples/) and are
mounted at `/goss/examples/` inside the test container.

## Go unit and integration tests (317 cases)

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

Linux distro matrix via [`integration-tests/test.sh`](../integration-tests/test.sh):

```bash
make rockylinux9    # example: one distro
make test-int-all   # full matrix (slow)
```

Non-amd64 / darwin / windows via [`integration-tests/run-validate-tests.sh`](../integration-tests/run-validate-tests.sh)
(find `*.goss.yaml` under platform dirs and run `goss validate`).

## Markdown lint

```bash
make lint-markdown
```

Lints: `docs/**/*.md`, `README.md`, `extras/**/README.md`, `.github/CONTRIBUTING.md`

Configuration: [`.markdownlint.yaml`](../.markdownlint.yaml)

## Security scan

Script: [`ci/security-scan.sh`](../ci/security-scan.sh)

```bash
make test-security
```

Runs on every `make check` and in the `coverage` CI job after dependency or docs updates.

Steps performed:

1. **`govulncheck ./...`** — Go vulnerability database scan for the module and its dependencies
2. **Trivy filesystem scan** — checks `go.mod` and `docs/requirements.txt` for known CVEs at **MEDIUM** severity and above

Locally, Trivy runs via the `trivy` binary if installed, otherwise via Docker
(`aquasec/trivy`). If neither is available, the scan is skipped with a warning unless
`SECURITY_STRICT=1` (always set in CI).

Docker image scanning (Alpine packages and compiled binary) continues to run in
[`.github/workflows/docker-syver.yaml`](../.github/workflows/docker-syver.yaml) and
[`.github/workflows/trivy-schedule.yaml`](../.github/workflows/trivy-schedule.yaml).

## CodeQL

Workflow: [`.github/workflows/codeql.yml`](../.github/workflows/codeql.yml)

Runs on pull requests and pushes to `devel`, plus a weekly schedule.
Uses GitHub's advanced CodeQL setup for Go and Actions with category
`/language:<language>` so PRs can be compared against the base branch.

CodeQL runs in CI only (not part of `make check`).

If the repository still has **CodeQL Default setup** enabled under **Settings → Code
security**, disable it in favour of this workflow. Default setup and an advanced workflow
cannot run together and will produce "configuration not found" warnings on pull requests.

## Adding tests for new features

* **Discovery / depends-on**: add cases to `discovery_*_test.go` and extend
  [`integration-tests/goss/examples/discovery/`](../integration-tests/goss/examples/discovery/)
* **Output formats**: add to `outputs/*_test.go`
* **Resource types**: add to `resource/validate_test.go` and platform integration gossfiles
* **CLI behaviour**: prefer root `syver_test.go` or integration command gossfiles under
  `integration-tests/goss/<platform>/commands/`

PRs should include automated tests; discovery changes should keep `make test-discovery-e2e` passing.
