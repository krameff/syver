# Windows

Windows support is **opt-in alpha**. This page explains what works, what does
not, why, and what is actually tested. For the per-attribute status grid across
all three platforms, see [platform support](platforms.md) -- this page
deliberately does not repeat it.

## Before anything else: the alpha gate

syver refuses to run on Windows unless you opt in:

```powershell
$env:SYVER_USE_ALPHA=1     # or pass --use-alpha=1
syver --use-alpha=1 validate
```

Without it the binary exits with an error pointing here. `GOSS_USE_ALPHA=1` is
still honoured.

## Quoting: `cmd.exe` and PowerShell do not agree

**`cmd.exe` does not treat `'` as a quote character.** A single-quoted argument
is not one argument there: it is split on whitespace, so a flag receives only
the first fragment.

```bat
REM cmd.exe: WRONG. The flag receives only "{inline:"
syver --use-alpha=1 --vars-inline '{inline: bar}' -g audit.yaml validate

REM cmd.exe: RIGHT
syver --use-alpha=1 --vars-inline "{inline: bar}" -g audit.yaml validate
```

PowerShell accepts either style. This applies to any flag whose value contains
spaces, not just `--vars-inline`.

You do not have to diagnose this from behaviour. syver rejects the value while
parsing the flag and quotes what it actually received, so the split is visible
in the message:

```text
Incorrect Usage: invalid value "{inline:" for flag -vars-inline: unable to
determine format from content
```

If you asked for `{inline: bar}` and the error quotes `{inline:`, your shell
split the command line.

Every flag also has an environment variable, which sidesteps shell quoting
entirely and is often easier in scripts. These are validated the same way, and
the error names the variable rather than the flag:

```powershell
$env:SYVER_VARS_INLINE = '{"inline": "bar"}'
syver --use-alpha=1 -g audit.yaml validate
```

The legacy `GOSS_*` names are still honoured, and an exported-but-empty
`SYVER_*` will not shadow a real `GOSS_*` value.

## What works

`file:` (existence, contents, size), `command:`, `http:`, `dns:`, `addr:`,
`process:` (`running` and `user`, but not `status`), `service:`, `registry:`,
and `exists` on `user:`, `group:` and `interface:`.

`registry:` is Windows-only, and is the resource most worth using here.

`command:` timeouts are also **stronger** here than on Linux and macOS. A
timed-out command's whole process tree is terminated through a Job Object, which
a process cannot leave unless it was built to break away. On POSIX a process that
calls `setsid` escapes the process group and survives. A command that *succeeds*
is never touched, so a check that deliberately starts a background process and
exits zero leaves it running.

**Mind the trailing backslash.** The last path segment is read as a *value*
name, so a key check needs a trailing `\`:

```yaml
registry:
  HKLM\SOFTWARE\Policies\Microsoft\Windows\NetworkProvider\HardenedPaths\:
    exists: true          # the KEY exists

  HKLM\SOFTWARE\Policies\Microsoft\Windows\NetworkProvider\HardenedPaths:
    exists: true          # a VALUE named HardenedPaths exists. Different check.
```

Omitting the backslash does not error. It asks a different question and answers
it correctly, so a key check written that way reports `false` against a key that
plainly exists -- and because `exists: false` then *passes*, nothing fails to
draw your attention to it. It is no longer silent: when a value lookup misses
while a key of that name exists in the same place, syver logs a warning naming
the alternative path. The grammar itself is deliberately not guessed at, because
guessing moves the ambiguity somewhere you cannot see it.

Hive names may be written short (`HKLM`), long (`HKEY_LOCAL_MACHINE`), or with
the PowerShell provider colon (`HKLM:`), so a path pasted from `regedit`'s
address bar or from `Get-ItemProperty` works unedited. A per-entry
`view: 32|64|native` selects the WOW64 registry view.
See [gossfile](gossfile.md#registry) for the full path
grammar, including `::` for value names that themselves contain a backslash. It
distinguishes three outcomes rather than two: a key that is absent, a key that
exists, and a key that exists but could not be read. That last case used to be
reported as "does not exist", which is backwards for the hardening specs
registry checks are usually written for.

## What does not, and why

Some of these are permanent. Knowing which is which saves you filing a bug or
attempting a fix that cannot work.

### Cannot work, ever

**`user:` `uid` / `gid`, and `group:` `gid`.** Windows identifies accounts by
SID (`S-1-5-21-...`). syver parses these attributes as integers, so there is no
value an implementation could return. This is a category mismatch, not missing
work.

**`kernel-param:`.** There is no Windows equivalent of `/proc/sys`.

### Broken, with a known cause

**`user:` `groups`.** Fails for every user, not just unusual ones. Listing a
user's groups calls Go's `user.LookupGroupId` once per SID in the account's
token, and every Windows token carries a mandatory integrity label
(`Mandatory Label\High Mandatory Level`), which is `SID_NAME_USE` 10
(`SidTypeLabel`) and not a group. Go rejects it with
`lookupGroupId: should be group account type, not 10`. It fails loudly rather
than returning a wrong or empty list, so it will not mislead you, but do not use
`groups:` in a Windows spec.

**`process:` `status`.** Every assertion errors, because gopsutil has no
Windows implementation for this field and every matching process therefore
fails to read. It used to return an empty list with no error instead, so an
assertion could pass having learned nothing; it now fails loudly rather than
silently. `process:` `user` shares the same all-or-nothing rule but is
unaffected in practice: it works on Windows.

### Not implemented yet

**`package:`.** Windows has no package-manager backend, so every assertion
errors with `could not detect Package type on this system, please use --package
flag to explicitly set it`. It does not silently answer "not installed", which
it did before this release. A backend over the Add/Remove Programs registry
hives is planned.

**`port:`.** Every assertion errors with `not implemented yet`. gopsutil ships
a Windows backend for connection enumeration, so this one looked like it might
already work and nobody had checked; measured on Windows Server 2025 on
2026-09-09, it does not. The fixture stays skipped because un-skipping it would
fail rather than reveal anything new.

**`mount:`.** Every assertion errors with "not supported on this platform".
It used to report a mountpoint-not-found error instead -- loud, but blaming
the operator's path for what was actually a missing implementation. A real
backend over `GetLogicalDriveStringsW` and related Win32 calls is planned.

## What is actually tested

Re-derive rather than trust these numbers. Two commands give them, and neither
needs a Windows host: count the entries carrying `skip: true` under
`integration-tests/syver/windows/`, and read the `# expect-count:` directive at
the top of each fixture, which records how many assertions that fixture produces.
That directive is checked on every run, so it cannot drift from the fixture
silently.

A passing Windows run covers less than it looks like, and the honest position is
that four fixtures assert nothing at all.

| Fixture | Live entries | Assertions | Notes |
| --- | --- | --- | --- |
| `gossfile` | 13 of 13 | 51 | aggregate of the others |
| `command` | 6 of 6 | 18 | |
| `registry` | 12 of 15 | 20 | 3 skipped: one GPO-delivered, two Defender view-difference |
| `file` | 2 of 2 | 7 | includes an absent-file case |
| `http` | 1 of 1 | 3 | |
| `group` | 3 of 3 | 3 | includes an absent-account case |
| `addr` | 2 of 2 | 2 | |
| `dns` | 1 of 1 | 2 | |
| `interface` | 2 of 2 | 2 | includes an absent-adapter case |
| `process` | 2 of 2 | 2 | |
| `service` | 1 of 1 | 2 | |
| `user` | 2 of 2 | 2 | includes an absent-account case |
| `add`, `help`, `validate` | 1 of 1 each | 2 each | CLI command fixtures |
| `autoadd` | **0 of 1** | 2 | asserts nothing |
| `mount` | **0 of 1** | 4 | asserts nothing |
| `package` | **0 of 1** | 2 | asserts nothing |
| `port` | **0 of 1** | 2 | asserts nothing |
| `kernel-param` | **not run at all** | | excluded by filename; nothing to assert |

**"Assertions" is not the same as "assertions that ran."** A resource whose
existence check fails has its remaining attributes reported as *skipped* rather
than failed, so one missing file turns five further assertions into skips. That
cascade is why the same fixtures skip substantially more assertions when driven
from a Linux host than on a real Windows Server host: on Windows the files and
registry keys are actually there, so the dependent attributes run instead of
cascading. `integration-tests/run-validate-tests.sh` prints both totals on every
run and its header comment records the last measured pair with the date and
commit; do not restate them here, because the Windows figure can only be
re-measured on Windows and this page is edited far more often than that happens.
It also means a resource quietly disappearing shows up as a rise in skips, not
as a failure.

The `user:` and `group:` fixtures each assert that a deliberately absent account
reports `exists: false` without erroring. Those two cases exist because that
exact behaviour regressed once.

## If you run `syver serve` on Windows

The `registry:` and `user:`/`group:` improvements above mean a failing check now
reports *why* it failed, not just that it did. On an unreachable-domain-controller
or access-denied case that error text reaches the HTTP response, and
[`serve`](cli.md#serve) is unauthenticated. That is the correct trade: a check
that could not run must not be reported as one that passed. But it is worth
knowing before pointing `serve` at a spec that names sensitive registry paths on
a host whose port is broadly reachable.

### The suite reaches the public internet

Two fixtures depend on an external service rather than on the host under test:
`addr` opens a TCP connection to `google.com:443`, and `http` fetches
`https://google.com`. They exercise real code paths, but a failure in either may
mean the network was slow rather than that syver is wrong.

If a Windows run fails on one of those and passes on an immediate re-run, that is
what you are looking at. `addr`'s budget was raised from one second to five on
2026-09-04 after exactly that happened. Treat a repeatable failure as real and a
one-off as suspect, and check the rest of the run before assuming a regression.

## Behaviour changes in this release

Windows specs that passed before may now fail. That is the point: they were not
being checked.

* `package: <name>: {installed: false}` now errors. Previously it passed for
  every name, because Windows fell through to the RPM backend and a missing
  `rpm` was read as "not installed".
* `registry: <key>: {exists: false}` now errors for a key that exists but cannot
  be read. A genuinely absent key still reports `exists: false`.
* `service: <name>: {enabled: false}` / `{running: false}` now error for a
  service that does not exist. Detection does not depend on English-language
  Windows output.
* `syver add service <name>` fails for a service that does not exist, rather
  than writing a plausible `enabled: false` block.
* `syver add file <path>` omits `mode`, `owner` and `group` rather than writing
  a fabricated `"-1"`. It still exits 0.
* `user:`, `group:` and `interface:` now distinguish "the lookup ran and found
  nothing" from "the lookup could not run". The first is unchanged; the second
  now errors. On a domain-joined host, an unreachable domain controller is the
  case that changes.

Re-run your Windows specs after upgrading. A spec that passed before may have
been checking less than it claimed.
