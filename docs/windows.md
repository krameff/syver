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

## What works

`file:` (existence, contents, size), `command:`, `http:`, `dns:`, `addr:`,
`process:`, `service:`, `registry:`, and `exists` on `user:`, `group:` and
`interface:`.

`registry:` is Windows-only, and is the resource most worth using here. It
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

**`process:` `status`.** Returns an empty list with no error when the per-process
lookup fails, so an assertion can pass having learned nothing.

### Not implemented yet

**`package:`.** Windows has no package-manager backend, so every assertion
errors with `could not detect Package type on this system, please use --package
flag to explicitly set it`. It does not silently answer "not installed", which
it did before this release. A backend over the Add/Remove Programs registry
hives is planned.

**`mount:`.** Reports a mountpoint-not-found error rather than an
unimplemented one. Loud, but it blames the wrong thing.

## What is actually tested

Measured 2026-09-02 on Windows Server 2025 (go1.27.0, gcc 16.1.0 MinGW-Builds).
Re-derive rather than trust these numbers, by counting `skip: true` entries under
`integration-tests/syver/windows/tests/`.

A passing Windows run covers less than it looks like, and the honest position is
that four fixtures assert nothing at all.

| Fixture | Live entries | Notes |
| --- | --- | --- |
| `command` | 4 of 4 | |
| `gossfile` | 13 of 13 | aggregate of the others |
| `user` | 3 of 3 | includes an absent-account case |
| `group` | 3 of 3 | includes an absent-account case |
| `registry` | 3 of 4 | |
| `addr` | 2 of 2 | |
| `process` | 2 of 2 | |
| `dns`, `file`, `http`, `service` | 1 of 1 each | |
| `interface` | **0 of 1** | asserts nothing |
| `mount` | **0 of 1** | asserts nothing |
| `package` | **0 of 1** | asserts nothing |
| `port` | **0 of 1** | asserts nothing |
| `kernel-param` | **not run at all** | excluded by filename; nothing to assert |

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
