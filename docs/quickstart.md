# Quick start

This page takes you from nothing to a working spec in a few minutes: install
syver, generate tests from a running system, validate them, and serve them as a
health endpoint.

To watch the same workflow catch a real change, see
[Catching drift with an auto-generated spec](https://krameff.com/videos/syver/autoadd-drift.mp4),
one of the [demo videos](https://krameff.com/syver.html#demos).

## Install

On Linux or macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/install.sh | sh
```

[Installation](installation.md) covers release binaries, building from source,
Windows, and the container wrappers.

## Generate a spec with `autoadd`

`autoadd` looks for packages, services, processes, users and groups matching a
name, and writes a test for each one it finds. This example uses `cron` on an
Ubuntu host; pick any service your machine runs.

```console
$ syver autoadd cron
Adding Package to './syver.yaml': cron
Adding Process to './syver.yaml': cron
Adding Service to './syver.yaml': cron
```

With no spec file in the current directory, syver creates `syver.yaml`:

```yaml
package:
  cron:
    installed: true
    versions:
    - 3.0pl1-184ubuntu2
service:
  cron:
    enabled: true
    running: true
process:
  cron:
    running: true
    status:
    - sleep
    user:
    - root
```

Run it as root to also pick up the ports a process listens on. `add` writes a
single test of a given type, for example a file:

```console
$ syver add file /etc/hosts
Adding File to 'syver.yaml': /etc/hosts
```

[`add` and `autoadd`](cli.md#add) list every resource type and what each one
detects. The spec is plain YAML, so edit it freely afterwards: the
[gossfile reference](gossfile.md) documents every attribute and
[matcher](gossfile.md#matchers).

## Validate

```console
$ syver validate
............

Total Duration: 0.043s
Count: 12, Failed: 0, Skipped: 0
```

Each dot is a passing test. `--format documentation` prints one line per test
instead. When something no longer matches, syver says what it expected and what
it found, and exits non-zero. Here the spec expects `/etc/hosts` to be mode
`0600`:

```console
$ syver validate
.F..........

Failures/Skipped:

File: /etc/hosts: mode:
Expected
    "0644"
to equal
    "0600"
diff
    --- test
    +++ actual
    @@ -1 +1 @@
    -0600
    +0644

Total Duration: 0.030s
Count: 12, Failed: 1, Skipped: 0
$ echo $?
1
```

To wait for a system to settle, for example while a service starts, retry until
the suite passes or the timeout runs out:

```console
syver validate --retry-timeout 30s --sleep 1s
```

## Serve a health endpoint

`serve` runs the suite when it is asked, caches the result for 5 seconds by
default, and answers on `/healthz` with HTTP 200 when every test passes and 503
when any fails:

```console
$ syver serve &
$ curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/healthz
200
$ curl -s -H "Accept: application/json" localhost:8080/healthz
{"results":[...
```

The endpoint has no authentication, so bind it to an address only your health
checker can reach; see [`serve`](cli.md#serve) for the options.

## Next steps

* [Templates](gossfile.md#templates) and `--vars` let one spec cover many hosts.
* [Discovery](gossfile.md#discovery) and [test dependencies](gossfile.md#test-dependencies)
  check prerequisites before the main suite runs.
* [Testing container images](containers/testing-images.md) runs a spec inside a
  Docker image with `dsyver`.
* The [command reference](cli.md) has every flag and its
  [environment variables](cli.md#environment-variables).
