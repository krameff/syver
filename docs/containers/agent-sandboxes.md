# Checking an agent sandbox

A coding agent that runs in a container is only as safe as that container. It
should have its toolchain and its workspace, and it should NOT have the
operator's SSH keys, cloud credentials, Docker socket or open network access.
When one of those is wrong nothing fails at start-up: the agent either fails
minutes into a task, or succeeds at something it should never have been able
to do.

Syver can check all of it before the agent starts, using
[`dsyver`](docker.md) to run a spec inside the container. A ready-made profile
lives in the repository under
[`examples/agent-sandbox/`](https://github.com/krameff/syver/tree/main/examples/agent-sandbox).

## What the profile checks

| Section | Asserts | Catches |
| --- | --- | --- |
| Toolchain | each command in `tools` runs | a missing or broken tool, before the agent hits it |
| Workspace | the workspace exists and is writable | a mount at the wrong path, or owned by the wrong user |
| Identity | the agent's user is not uid 0 | a lost `USER` line or a stray `--user 0` |
| Credential files | `~/.ssh` keys, `~/.aws/credentials`, `~/.docker/config.json`, `gh` and `kube` config, `.git-credentials`, `.netrc` are absent | a home directory mounted from the host |
| Credential variables | each name in `absent_env` is unset | a token passed through with `-e` or an `env_file` |
| Runtime socket | `/var/run/docker.sock` and the podman socket are absent | a mounted socket, which lets the agent start a privileged container on the host |
| Egress | hosts in `egress_allow` are reachable, hosts in `egress_deny` are not | a missing allow rule, and a firewall that is not actually filtering |
| Hardening | an empty capability bounding set, `NoNewPrivs`, a read-only root filesystem, a memory limit | a `docker run` that dropped `--cap-drop=ALL`, `--security-opt no-new-privileges`, `--read-only` or `--memory` |

## Running it

Copy `syver.yaml` and `vars.yaml` next to where you start the sandbox, edit
`vars.yaml`, and run `dsyver` with **the same arguments the sandbox is started
with**:

```sh
SYVER_VARS=vars.yaml dsyver run \
  --cap-drop=ALL --security-opt no-new-privileges --read-only --memory 2g \
  -v "$PWD:/workspace" my-agent-image
```

The arguments matter. Most of what this profile checks is a property of how the
container was started rather than of the image, so checking the image with
different flags checks a different sandbox.

A sandbox that passes reports every check and exits 0. Against a container run
as root, with the Docker socket and an AWS credentials file mounted, a token in
the environment and no other flags, the same profile fails eight checks. The
failure summary, abridged to one line per check:

```console
Addr: tcp://example.com:443: reachable:
Command: env GITHUB_TOKEN is not set: exit-status:
Command: not running as root: stdout:
File: /home/agent/.aws/credentials: exists:
File: /proc/mounts: contents:
File: /proc/self/status: contents:
File: /sys/fs/cgroup/memory.max: contents:
File: /var/run/docker.sock: exists:
Count: 32, Failed: 8, Skipped: 0
```

and exits 1, so it can gate whatever starts the agent.

## Writing credential checks safely

Assert that a secret is **absent**, never what its value is. A spec that says
a variable must equal a token has put the token in a file that gets committed.

The variable checks in the profile only test that the name is set, with
`env | grep -q '^NAME='`. `grep -q` prints nothing, so even when the check fails
the value does not appear in syver's output.

## What it cannot see

* **Egress denial depends on a real filter.** The deny checks prove that the
  listed hosts are unreachable; they do not make them unreachable. On a
  container with ordinary network access every deny check fails, which is the
  point: it tells you the filtering you thought was there is not.
* **The environment is the container's, not the agent's.** `dsyver` checks the
  environment the container was started with. A variable the agent's own
  launcher exports after start-up is not visible to it.
* **Some container settings are only visible from outside.** Capabilities, the
  read-only root and the memory limit are read through `/proc` and cgroup files,
  which works for the settings above. For anything else, such as the network
  mode or the full mount list, run syver on the host with a `command:` check on
  `docker inspect`. Syver renders a spec as a Go template before reading it, so
  the `--format` argument is wrapped in `` {{` `}} `` to pass it through
  untouched:

  ```yaml
  command:
    sandbox network mode:
      exec: docker inspect --format '{{`{{.HostConfig.NetworkMode}}`}}' my-sandbox
      exit-status: 0
      stdout:
        - "/^sandbox-net$/"
  ```

* **The memory check needs cgroup v2.** On a cgroup v1 host
  `/sys/fs/cgroup/memory.max` does not exist and that check fails. Delete it
  there: cgroup v1 reports "no limit" as a very large number rather than `max`,
  so the same pattern pointed at the v1 file would pass whether or not a limit
  was set.

## Reference

Every `dsyver` flag and environment variable: [the dsyver reference](docker.md).
The resources used here, `command`, `file` and `addr`, are documented in
[the gossfile reference](../gossfile.md).
