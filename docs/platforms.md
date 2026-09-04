---
render_macros: true

fully_supported: :fontawesome-solid-circle-check:{ .green title="Fully supported/tested" }
community_supported: :fontawesome-solid-circle-check:{ .blue title="Fully working/tested, community support" }
not_automated: :fontawesome-solid-circle-pause:{ .green title="Works but is not covered by automated tests" }
work_partially: :fontawesome-solid-circle-minus:{ .orange title="Works partially / partially tested" }
not_implemented: :fontawesome-solid-circle-xmark:{ .red title="Not implemented / needs implementation" }
broken: :material-image-broken-variant:{ .red title="Currently broken" }
n_a: :fontawesome-regular-circle:{ .grey title="Not applicable for this platform" }
no_data: :fontawesome-regular-circle-question:{ .grey title="Not yet tried, no data" }
---
# Platform feature-parity

macOS and Windows binaries are new and considered alpha-quality.
Some functionality may be missing, some may be broken.
(Enhancements and bug-reports welcome, please see #551)

To clearly signal that, syver emits a log message on every invocation saying so, linking here, then exits with a clear error.

To try out the alpha functionality, you must do one of:

* pass `--use-alpha=1` to the root command (e.g. `syver --use-alpha=1 validate`).
* set an environment variable `SYVER_USE_ALPHA=1` (or the legacy `GOSS_USE_ALPHA=1`,
  which is still honoured; `SYVER_USE_ALPHA` wins when both are set to a non-empty value).

One concrete difference worth knowing before you rely on Windows: syver puts a
timed-out check's process into its own process group and kills the group, and
Windows has no equivalent, so there the started process is killed and anything it
spawned survives. On Linux and macOS only a process that deliberately detaches
into a new session escapes that way. Syver still bounds how long it waits, so a
check fails rather than hanging, but on Windows expect leftover processes after a
timeout more often than the other platforms.

The macOS and Windows support is community driven;
there is no commitment to adding features / fixing bugs for those platforms.
[See thread](https://github.com/goss-org/goss/pull/585#discussion_r429968540).

This matrix attempts to track parity across platforms.

!!! tip "Windows users start here"

    This page is the status grid. [Windows](windows.md) explains *why* each
    Windows cell is what it is, which of them can never change, and what the
    Windows test suite actually covers.

## Legend

| Symbol                  | Meaning                                |
|:-----------------------:|----------------------------------------|
| {{ fully_supported }}   | Fully supported/tested                 |
| {{community_supported}} | Full working/tested, community support |
| {{ not_automated }}     | Works but without automated tests      |
| {{ work_partially }}    | Works partially / partially tested     |
| {{ not_implemented }}   | Not implemented / needs implementation |
| {{ broken }}            | Currently broken                       |
| {{ n_a }}               | Not applicable for this platform       |
| {{ no_data }}           | Not yet tried, no data                 |

!!! note "About partial support"

    This is ambiguous. Where you see this, check into the test coverage within `integration-tests/syver/{darwin|windows}/{test}.goss.yaml` for more detail.
    It might be that not all features work as on `linux`, it might be that not all features are covered by automated tests.

## Tests/assertions support matrix

| Test                | Option              | Linux                   | macOS                  | Windows                 |
|:--------------------|:--------------------|:-----------------------:|:----------------------:|:-----------------------:|
| **addr**            |                     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | reachable           | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | local-address       | {{ fully_supported }}   | {{ no_data }}          | {{ work_partially }}    |
|                     | timeout             | {{ fully_supported }}   | {{ not_automated }}    | {{ not_automated }}     |
| **command**         |                     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | exit-status         | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | stdout              | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | stderr              | {{ fully_supported }}   | {{ not_automated }}    | {{ not_automated }}     |
|                     | timeout             | {{ fully_supported }}   | {{ not_automated }}    | {{ not_automated }}     |
| **dns**             |                     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | resolvable          | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | addrs               | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | server              | {{ fully_supported }}   | {{ no_data }}          | {{ work_partially }}    |
|                     | timeout             | {{ fully_supported }}   | {{ not_automated }}    | {{ work_partially }}    |
| **file**            |                     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | exists              | {{ fully_supported }}   | {{ work_partially }}   | {{community_supported}} |
|                     | mode                | {{ fully_supported }}   | {{ work_partially }}   | {{ not_implemented }}   |
|                     | size                | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | owner               | {{ fully_supported }}   | {{ broken }}           | {{ not_implemented }}   |
|                     | group               | {{ fully_supported }}   | {{ broken }}           | {{ not_implemented }}   |
|                     | filetype            | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | contains            | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | md5                 | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | sha256              | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | linked-to           | {{ fully_supported }}   | {{ no_data }}          | {{ no_data }}           |
| **gossfile**        |                     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
| **group**           |                     | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | exists              | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | gid                 | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
| **http**            |                     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | status              | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | allow-insecure      | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | no-follow-redirects | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | timeout             | {{ fully_supported }}   | {{ not_automated }}    | {{ work_partially }}    |
|                     | request-headers     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | headers             | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | body                | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | username            | {{ fully_supported }}   | {{ not_automated }}    | {{ work_partially }}    |
|                     | password            | {{ fully_supported }}   | {{ not_automated }}    | {{ work_partially }}    |
| **interface**       |                     | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | exists              | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | addrs               | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | mtu                 | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
| **kernel-param**    |                     | {{ fully_supported }}   | {{ n_a }}              | {{ n_a }}               |
|                     | value               | {{ fully_supported }}   | {{ n_a }}              | {{ n_a }}               |
| **registry**        |                     | {{ n_a }}               | {{ n_a }}              | {{ work_partially }}    |
|                     | exists              | {{ n_a }}               | {{ n_a }}              | {{ work_partially }}    |
|                     | value               | {{ n_a }}               | {{ n_a }}              | {{ work_partially }}    |
|                     | type                | {{ n_a }}               | {{ n_a }}              | {{ work_partially }}    |
| **mount**           |                     | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | exists              | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | opts                | {{ fully_supported }}   | {{ not_implemented }}  | {{ n_a }}               |
|                     | source              | {{ fully_supported }}   | {{ not_implemented }}  | {{ n_a }}               |
|                     | filesystem          | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | usage               | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
| **matching**        |                     | {{ fully_supported }}   | {{ no_data }}          | {{ no_data }}           |
| **package**         |                     | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | installed           | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | versions            | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
| **port**            |                     | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | listening           | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | ip                  | {{ fully_supported }}   |  {{ no_data }}         | {{ not_implemented }}   |
| **process**         |                     | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | running             | {{ fully_supported }}   | {{ work_partially }}   | {{ work_partially }}    |
|                     | status              | {{ fully_supported }}   | {{ work_partially }}   | {{ broken }}            |
|                     | user                | {{ fully_supported }}   | {{ work_partially }}   | {{ no_data }}           |
| **service**         |                     | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | enabled             | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | running             | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | runlevels           | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
| **user**            |                     | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | exists              | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | uid                 | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | gid                 | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |
|                     | groups              | {{ fully_supported }}   | {{ not_implemented }}  | {{ broken }}            |
|                     | home                | {{ fully_supported }}   | {{ not_implemented }}  | {{ work_partially }}    |
|                     | shell               | {{ fully_supported }}   | {{ not_implemented }}  | {{ not_implemented }}   |

**`user:` `groups:` on Windows is broken, for every user.** Reporting a user's
groups calls Go's `user.LookupGroupId` once per SID in the account's token, and
every Windows token carries a mandatory integrity label -- `Mandatory Label\High
Mandatory Level`, which is `SID_NAME_USE` 10 (`SidTypeLabel`), not a group. Go
rejects it with `lookupGroupId: should be group account type, not 10`, so the
attribute fails for any user at all rather than for unusual ones. It fails
loudly with that message rather than reporting an empty or wrong list, so it
will not silently mislead you, but do not use `groups:` in a Windows spec.

**`uid:` and `gid:` on Windows are not a missing feature.** Windows identifies
accounts and groups by SID (`S-1-5-21-...`), and syver parses these attributes
as integers. There is no integer to report, so no implementation could satisfy
them. They are listed as not implemented because that is the closest available
status, not because the work is pending.

**`kernel-param:` on Windows.** There is no Windows equivalent -- no `/proc/sys`.
`exists` and `value` both return an explicit `kernel-param is not supported on
this platform` error rather than silently answering `false`. Use `registry:`
instead: most tunables `kernel-param:` would address on Linux have a
`HKLM\SYSTEM\CurrentControlSet\...` equivalent.

**`process: status` on Windows is `{{ broken }}`, not merely unimplemented.**
It silently returns an empty result -- gopsutil has no Windows implementation
for this field, and syver's own aggregation currently swallows that
per-process rather than surfacing it, so `status: []` passes even for a
running process. Tracked for a real fix in the Windows depth roadmap; do not
rely on this assertion on Windows in the meantime.

On Windows, `process:` names must include the `.exe` suffix (e.g.
`process: httpd.exe:`, not `process: httpd:`) -- Windows process names as
reported by the OS always carry it, unlike Linux/macOS.

### Windows: behaviour changed in this release

The following used to silently pass, or silently write a placeholder value
into a `syver add`-generated spec, on Windows. Each of these now returns an
explicit error instead:

* `package: <name>: {installed: false}` -- Windows has no supported
  package-manager backend. `installed` and `versions` both fail with
  `could not detect Package type on this system, please use --package flag
  to explicitly set it`. `syver add package` **hard-fails** the same way
  (this differs from `file`, below, which only omits keys -- there is no
  honest "installed: unknown" to write for a package).
* `syver add file <path>` -- `mode`, `owner` and `group` are simply
  **omitted** from the generated spec rather than written as a fabricated
  `"-1"`. `syver add` still exits 0.
* `registry: <key>: {exists: false}` -- a key that exists but could not be
  read (e.g. `ERROR_ACCESS_DENIED`) now errors, instead of being reported as
  absent. A genuinely absent key still reports `exists: false` as before.
* `service: <name>: {enabled: false}` / `{running: false}` -- a service that
  does not exist now errors, instead of being reported as disabled/not
  running. Detection does not depend on English-language Windows output.
* `syver add service <name>` -- **hard-fails** for a service that does not
  exist, rather than writing a plausible `enabled: false` block for a name
  that was never there. This is the same shape as `package` above, and the
  opposite of `file`, which only omits keys.
* `user: <name>: {exists: false}`, `group:` and `interface:` -- these now
  distinguish "the lookup ran and found nothing" from "the lookup could not
  run". The first still reports `exists: false` exactly as before; the second
  now errors instead of being reported as absent. On a domain-joined host an
  unreachable domain controller is the case that changes: a spec asserting a
  user is absent used to pass when the lookup had simply failed. This fix is
  not Windows-specific -- the code is in untagged files and applies
  everywhere -- but Windows is where the two cases come apart in practice.

If you have existing Windows specs, re-run them after upgrading: a spec that
passed before may now fail where it was never actually being checked.

## Commands support matrix

| Test       | Linux                  | macOS               | Windows              |
|:-----------|------------------------|---------------------|----------------------|
| `add`      | {{ fully_supported }}  | {{ no_data }}       | {{ work_partially }} |
| `autoadd`  | {{ fully_supported }}  | {{ no_data }}       | {{ no_data }}        |
| `help`     | {{ fully_supported }}  | {{ no_data }}       | {{ work_partially }} |
| `render`   | {{ fully_supported }}  | {{ no_data }}       | {{ no_data }}        |
| `serve`    | {{ fully_supported }}  | {{ not_automated }} | {{ no_data }}        |
| `validate` | {{ fully_supported }}  | {{ not_automated }} | {{ work_partially }} |

### `command` testing notes

Run all of the `darwin`/`windows` integration tests:

```bash
make test-int-validate-darwin-amd64
make test-int-validate-windows-amd64
```

The script finds all goss spec files within `integration-tests` then filters to just ones matching the passed OS-name,
then runs `validate` against them.

### Command: `serve`

This is a special-case test since it requires a persistent process,
then to make the http request, then to tear down the process.

#### macOS `serve`

```bash
make "test-int-serve-darwin-amd64"
```

#### Windows `serve`

```bash
make "test-int-serve-windows-amd64"
```

## Contributing

The current integration test approach is only appropriate for validating `linux` binaries against `linux` OS/arch combinations.

Validating `macOS` and `Windows` binaries requires native runners on those platforms in GitHub Actions.
Because neither platform uses the Linux Docker integration-test containers,
assertions are limited to the state of the CI hosts, where we rely on that being predictable.

You can find goss-files that are used to populate this matrix within `integration-tests/syver/{darwin|windows}/{test}.goss.yaml`.
Where a feature does note work the same as linux, it is commented.
The intent is to end up with a set of running-and-passing tests.
