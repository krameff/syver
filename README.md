# Syver - Quick and Easy server validation

<!-- markdownlint-disable no-inline-html -->
<p align="center">
  <img src="images/syver-logo.svg" alt="Syver - by Krameff Solutions Ltd" width="200">
</p>
<!-- markdownlint-enable no-inline-html -->

> **Note:** This is a fork of the original [goss-org/goss](https://github.com/goss-org/goss) project created and
> maintained by [@aelsabbahy](https://github.com/aelsabbahy). All original work remains under the Apache 2.0 license
> and full credit goes to the original author for building such a solid foundation. This fork exists to enable newer
> features and fixes to be developed and released. We are deeply grateful for the effort and care that went into the
> original project and aim to continue it in the same spirit.

[![Documentation](https://img.shields.io/badge/docs-docs-blue)](docs/index.md)

<!-- --8<-- [start:intro] -->

## Syver in 45 seconds

<!-- markdownlint-disable line-length no-inline-html-->
<a href="https://asciinema.org/a/4suhr8p42qcn6r7crfzt6cc3e?autoplay=1" target="_blank"><img src="https://cloud.githubusercontent.com/assets/6783261/17330426/ce7ad066-5894-11e6-84ea-29fd4207af58.gif" alt="asciicast"></a>
<!-- markdownlint-enable line-length no-inline-html -->

**Note:** For testing containers see the [dgoss](extras/dsyver/README.md) wrapper.
Also, user submitted wrapper scripts for Kubernetes [kgoss](extras/ksyver/README.md)
and Docker Compose [dcgoss](extras/dcsyver/README.md).

**Note:** For some Docker/Kubernetes healthcheck, health endpoint, and
container ordering examples, see the
[Docker/Kubernetes simplified health checks blog post][kubernetes-simplified-health-checks].

## Introduction

### What is Syver?

Syver is a YAML based [serverspec](http://serverspec.org/) alternative tool for validating a server's configuration.
It eases the process of writing tests by allowing the user to generate tests from the current system state.
Once the test suite is written they can be executed, waited-on, or served as a health endpoint.

### Why use Syver?

* Syver is EASY! - [Syver in 45 seconds](#syver-in-45-seconds)
* Syver is FAST! - small-medium test suites are near instantaneous
* Syver is SMALL! - <10MB single self-contained binary

### Coming from goss?

Syver is the renamed continuation of the `krameff/goss` fork. **Your gossfiles do not
need to change.** `gossfile:`, `goss.yaml`, `GOSS_*` env vars, the `dgoss`/`dcgoss`/`kgoss`
wrappers and the `-g` flag all keep working.

See [goss vs Syver](docs/goss-vs-syver.md) for the full side-by-side comparison, including
the single intentional breaking change.

## Installation

**Note:** For macOS and Windows, see [platform support](docs/platforms.md).

Build from source or install release binaries — see [installation](docs/installation.md).

This will install syver and [dgoss](extras/dsyver/README.md).

<!-- --8<-- [end:intro] -->
<!-- --8<-- [start:install] -->

### Manual installation

Download pre-built binaries and install wrappers as described in [installation](docs/installation.md).

### Build it yourself

```bash
make build
```

Alternatively, you can build it with [goreleaser](https://goreleaser.com/). To
build a binary, use `goreleaser build`, and to only build for the same OS and
architecture as the machine you're building on, include the `--single-target`
flag. The `--clean` flag will clean up any existing builds, and `--snapshot`
will allow you to build against something other than a tag.

Here's an example:

```console
$ goreleaser build --clean --single-target --snapshot
  • skipping validate...
  • cleaning distribution directory
  • loading environment variables
  • getting and validating git state
  • ignoring errors because this is a snapshot error=git doesn't contain any tags - either add a tag or use --snapshot
  • using tags previous= current=v0.0.0
  • pipe skipped or partially skipped reason=disabled during snapshot mode
  • parsing tag
  • setting defaults
  • partial
  • snapshotting
  • building snapshot... version=0.0.1-next
  • running before hooks
  • running hook=go mod tidy
  • ensuring distribution directory
  • setting up metadata
  • writing release metadata
  • loading go mod information
  • build prerequisites
  • building binaries
  • partial build match=target=linux_arm64_v8.0
  • building paths=cmd/syver binaries=syver target=linux_arm64_v8.0
  • took: 31s
  • writing artifacts metadata
  • build succeeded after 31s
  • thanks for using GoReleaser!
$ tree dist
dist
├── artifacts.json
├── binaries_linux_arm64_v8.0
│   └── syver <- your binary
├── config.yaml
└── metadata.json

2 directories, 4 files
```

<!-- --8<-- [end:install] -->

## Full Documentation

[Full documentation](docs/index.md)

## Using the container image

[Using the Syver container image](docs/container_image.md)

## Quick start

<!-- --8<-- [start:quickstart] -->

### Writing a simple sshd test

An initial set of tests can be derived from the system state by using the [add](docs/cli.md#add)
or [autoadd](docs/cli.md#autoadd) commands.

Let's write a simple sshd test using autoadd.

```txt
# Running it as root will allow it to also detect ports
$ sudo syver autoadd sshd
```

Generated `goss.yaml`:

```yaml
port:
  tcp:22:
    listening: true
    ip:
    - 0.0.0.0
    pid:
    - 1234
  tcp6:22:
    listening: true
    ip:
    - '::'
    pid:
    - 1234
service:
  sshd:
    enabled: true
    running: true
user:
  sshd:
    exists: true
    uid: 74
    gid: 74
    groups:
    - sshd
    home: /var/empty/sshd
    shell: /sbin/nologin
group:
  sshd:
    exists: true
    gid: 74
process:
  sshd:
    running: true
    status:
    - sleep
    user:
    - root
```

Now that we have a test suite, we can:

* Run it once

```console
$ syver validate
...............

Total Duration: 0.021s # <- yeah, it's that fast..
Count: 15, Failed: 0
```

* Edit it to use [templates](docs/gossfile.md#templates), and run with a vars file

```console
syver --vars vars.yaml validate
```

* keep running it until the system enters a valid state or we timeout

```console
syver validate --retry-timeout 30s --sleep 1s
```

* serve the tests as a health endpoint

```console
$ syver serve &
$ curl localhost:8080/healthz

# JSON endpoint
$ syver serve --format json &
$ curl localhost:8080/healthz

# rspecish response via content negotiation
$ syver serve --format json &
$ curl -H "Accept: application/vnd.goss-rspecish" localhost:8080/healthz
```

### Manually editing Syver files

Syver files can be manually edited to improve readability and expressiveness of tests.

A [Json draft 7 schema](https://github.com/json-schema-org/json-schema-spec/blob/draft-07/schema.json) in
[docs/schema.yaml](docs/schema.yaml) makes it easier to edit simple goss.yaml files in IDEs,
providing usual coding assistance such as inline documentation, completion and static analysis.
See #793 for screenshots.

For example, to configure the Json schema in JetBrains intellij IDEA,
follow [documented instructions](https://www.jetbrains.com/help/idea/json.html#ws_json_schema_add_custom),
with arguments such as:
* `schema url=docs/schema.yaml` (path from the repository root)
* `schema version=Json schema version 7`
* `file path pattern=*/goss.yaml`

In addition, Syver files can also be further manually edited (without yet full json support) to use:

* [Matchers and patterns](docs/gossfile.md#matchers)
* [Advanced Matchers](docs/gossfile.md#advanced-matchers)
* [Templates](docs/gossfile.md#templates)
* `title` and `meta` (arbitrary data) attributes are persisted when adding other resources with `syver add`

Some examples:

```yaml+jinja
user:
  sshd:
    title: UID must be between 50-100, GID doesn't matter. home is flexible
    meta:
      desc: Ensure sshd is enabled and running since it's needed for system management
      sev: 5
    exists: true
    uid:
      # Validate that UID is between 50 and 100
      and:
        gt: 50
        lt: 100
    home:
      # Home can be any of the following
      or:
      - /var/empty/sshd
      - /var/run/sshd

package:
  kernel:
    installed: true
    versions:
      # Must have 3 kernels and none of them can be 4.4.0
      and:
      - have-len: 3
      - not:
          contain-element: 4.4.0

  # Loaded from --vars YAML/JSON file
  {{.Vars.package}}:
    installed: true

{{if eq .Env.OS "centos"}}
  # This test is only when $OS environment variable is set to "centos"
  libselinux:
    installed: true
{{end}}
```

Goss.yaml files with templates can still be validated through the Json schema after being rendered
using the `syver render` command. See example below

```console
$ cd docs
$ syver --vars ./vars.yaml render > rendered_goss.yaml
# proceed with json schema validation of rendered_goss.yaml in your favorite IDE
# or in one of the Json schema validator listed in https://json-schema.org/implementations.html
# The following example is for a Linux AMD64 host
$ curl -LO https://github.com/neilpa/yajsv/releases/download/v1.4.1/yajsv.linux.amd64
$ chmod a+x yajsv.linux.amd64
$ sudo mv yajsv.linux.amd64 /usr/sbin/yajsv

$ yajsv -s goss-json-schema.yaml rendered_goss.yaml

rendered_goss.yaml: fail: process.chrome: skip is required
rendered_goss.yaml: fail: service.sshd: skip is required
1 of 1 failed validation
rendered_goss.yaml: fail: process.chrome: skip is required
rendered_goss.yaml: fail: service.sshd: skip is required
```

Full list of available Json schema validators can be found in <https://json-schema.org/implementations.html#validator-command%20line>

### Discovery and test dependencies

Run lightweight discovery checks before the main suite and use the results in templates, or declare
`depends-on` to skip dependents when a prerequisite fails. See
[Discovery](docs/gossfile.md#discovery) and
[Test dependencies](docs/gossfile.md#test-dependencies).

```bash
# Pre-run discovery, then validate main gossfile (preferred)
syver validate -g goss.yml --discover discovery.yaml

# Or inline discovery: in the same gossfile
syver validate -g goss-inline.yml

# Export discovery results for external tooling (unchanged)
syver validate -g discovery.yaml --format discovery
```

Fixtures: [`integration-tests/goss/examples/discovery/`](integration-tests/goss/examples/discovery/)

<!-- --8<-- [end:quickstart] -->
<!-- --8<-- [start:about] -->

## Supported resources

* package - add new package
* file - add new file
* addr - add new remote address:port - ex: google.com:80
* port - add new listening [protocol]:port - ex: 80 or udp:123
* service - add new service
* user - add new user
* group - add new group
* command - add new command
* dns - add new dns
* process - add new process name
* kernel-param - add new kernel-param
* mount - add new mount
* interface - add new network interface
* http - add new network http url with proxy support
* syver (alias: goss) - add new syver file, it will be imported from this one
* matching - test for matches in supplied content

## Supported output formats

* rspecish - **(default)** Similar to rspec output
* documentation - Verbose test results
* json - JSON, detailed test result
* tap - TAP style
* junit - JUnit style
* nagios - Nagios/Sensu compatible output /w exit code 2 for failures.
* prometheus - Prometheus compatible output.
* silent - No output. Avoids exposing system information (e.g. when serving tests as a healthcheck endpoint).

## Community Contributions

<!-- markdownlint-disable line-length -->
* [goss-ansible](https://github.com/indusbox/goss-ansible) - Ansible module for Goss.
* [degoss](https://github.com/naftulikay/ansible-role-degoss) - Ansible role for installing, running, and removing Goss in a single go.
* [ansible-goss-install](https://github.com/marcinpraczko/ansible-goss-install) - Ansible role for installing Goss (option for install as user or root)
* [kitchen-goss](https://github.com/ahelal/kitchen-goss) - A test-kitchen verifier plugin for Goss.
* [goss-fpm-files](https://github.com/deanwilson/unixdaemon-fpm-cookery-recipes) - Might be useful for building goss system packages.
* [packer-provisioner-goss](https://github.com/YaleUniversity/packer-provisioner-goss) - A packer plugin to run Goss as a provision step.
* [gossboss](https://github.com/mdb/gossboss) - Collect and view aggregated Goss test results from multiple remote Goss servers.
<!-- markdownlint-enable line-length -->

## Limitations

`syver` works well on Linux, but support on Windows & macOS is alpha. See [platform support](docs/platforms.md).

The following tests have limitations.

Package:

* rpm
* deb
* Alpine apk
* pacman

Service:

* systemd
* sysV init
* OpenRC init
* Upstart

Port:

* Port state is only implemented on Linux, where it's read from
  `/proc/net/{tcp,udp,tcp6,udp6}`. It is not implemented on macOS or Windows --
  see [platform support](docs/platforms.md).
* On Linux, if one of those files exists but contains a line syver can't parse
  (an unexpected IP/port/uid encoding, typically from a non-standard procfs,
  e.g. inside certain containers or network namespaces), the affected `port`
  resource now fails with an explicit `Error:` block in the output instead of
  silently reporting the port as not listening. To investigate: note which
  protocol failed from the resource id (`tcp`, `tcp6`, `udp`, `udp6`), then
  inspect the corresponding file directly, e.g. `cat /proc/net/tcp6`, and
  compare its columns against a working host. A missing or unreadable file is
  *not* an error case -- it's treated as "no ports" for that protocol.

[kubernetes-simplified-health-checks]: https://medium.com/@aelsabbahy/docker-1-12-kubernetes-simplified-health-checks-and-container-ordering-with-goss-fa8debbe676c

<!-- --8<-- [end:about] -->
