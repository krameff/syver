# Syver - Quick and Easy server validation

<!-- markdownlint-disable no-inline-html -->
<p align="center">
  <img src="images/syver-logo.svg" alt="Syver - by Krameff Solutions Ltd" width="200">
</p>
<!-- markdownlint-enable no-inline-html -->

--8<-- "README.md:intro"

## Supported resources

* package - add new package
* pkg-baseline - record every installed package, to check the host still has exactly that set (Linux)
* file - add new file
* addr - add new remote address:port - ex: google.com:80
* port - add new listening [protocol]:port - ex: 80 or udp:123
* service - add new service
* user - add new user
* group - add new group
* command - add new command
* dns - add new dns
* process - add new process name
* registry - add new Windows registry key or value (Windows only)
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
* structured - JSON like `json`, plus a `summary` object and a human-readable `summary-line` on every result
* tap - TAP style
* junit - JUnit style
* nagios - Nagios/Sensu compatible output /w exit code 2 for failures.
* prometheus - Prometheus compatible output.
* silent - No output. Avoids exposing system information (e.g. when serving tests as a healthcheck endpoint).

## Community Contributions

These are third-party integrations written for upstream goss. They are listed
because they still work, but **each one invokes a binary named `goss`**, and
`install.sh` installs `syver`, `dsyver` and `dgoss`, but no `goss`. To use any of
them, put a `goss` on your `PATH` pointing at syver:

```bash
sudo ln -s "$(command -v syver)" /usr/local/bin/goss
```

The gossfiles these tools generate and consume need no changes; only the
binary name differs. None of them are maintained by this project.

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

`syver` works well on Linux, but support on Windows & macOS is alpha. See [platform support](https://github.com/krameff/syver/blob/main/docs/platforms.md).

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
* Windows Service Control Manager (alpha, with six Windows-only attributes --
  see [windows](https://github.com/krameff/syver/blob/main/docs/windows.md))

Port:

* Port state is read from `/proc/net/{tcp,udp,tcp6,udp6}` on Linux, where it is
  fully supported. For a socket on `[::]`, Linux sock_diag says whether it
  also accepts IPv4; if it does, it counts for `tcp:N` / `udp:N` with `ip`
  `0.0.0.0`. It is not implemented on macOS or Windows: measured on
  Windows Server 2025, every assertion returns "not implemented yet". See
  [platform support](https://github.com/krameff/syver/blob/main/docs/platforms.md),
  which is the authoritative per-resource matrix.
* On Linux, if one of those files exists but contains a line syver can't parse
  (an unexpected IP/port/uid encoding, typically from a non-standard procfs,
  e.g. inside certain containers or network namespaces), the affected `port`
  resource now fails with an explicit `Error:` block in the output instead of
  silently reporting the port as not listening. To investigate: note which
  protocol failed from the resource id (`tcp`, `tcp6`, `udp`, `udp6`), then
  inspect the corresponding file directly, e.g. `cat /proc/net/tcp6`, and
  compare its columns against a working host. A missing or unreadable file is
  *not* an error case -- it's treated as "no ports" for that protocol.

## Documentation

* [goss vs Syver](goss-vs-syver.md): what this project is, and exactly how it
  differs from the upstream it continues
* [Installation](installation.md): install syver, dgoss, and the other wrappers
* [Quickstart](quickstart.md): write and run your first gossfile
* [Container image](container_image.md): run syver from the published container image
* [Command reference](cli.md): CLI flags for `validate`, `serve`, `add`, and friends
* [The gossfile](gossfile.md): full resource and matcher reference, including
  [discovery](gossfile.md#discovery) and
  [test dependencies (`depends-on`)](gossfile.md#test-dependencies)
* [Platforms](platforms.md): per-platform support notes and caveats
* [Windows](windows.md): opt-in alpha support, covering what works, what does
  not, and what reports honestly rather than guessing
* Containers:
    * [Testing container images](containers/testing-images.md): a walkthrough of checking an image with `dsyver`
    * [Docker](containers/docker.md), [Docker Compose](containers/docker-compose.md) and
      [Kubernetes](containers/kubernetes.md): the `dsyver`, `dcsyver` and `ksyver` wrapper references
    * [Checking an agent sandbox](containers/agent-sandboxes.md): verifying a coding agent's container before it starts
    * [Docker Sandboxes](containers/docker-sandboxes.md): the `sbxsyver` wrapper for `sbx` microVMs
* [Contributing](contributing.md): development setup and contribution guidelines
* [Testing](testing.md): running the test suite locally, and how those checks
  map to CI
* [Changelog](changelog.md): release history
* [License](license.md)
