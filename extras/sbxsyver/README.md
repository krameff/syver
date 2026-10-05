# sbxsyver

sbxsyver runs syver inside a [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/)
sandbox, the microVMs that the `sbx` CLI creates for coding agents. It copies
the syver binary and your spec into the sandbox with `sbx cp`, runs them with
`sbx exec`, and removes them again. Your workspace is not touched.

It checks an existing sandbox; creating, starting and removing sandboxes stays
with `sbx`. A stopped sandbox is started by `sbx exec` itself.

A ready-made profile for checking that a sandbox is what an agent should be
given lives in the repository under
[`examples/agent-sandbox-sbx/`](https://github.com/krameff/syver/tree/main/examples/agent-sandbox-sbx).

If your agent runs in an ordinary container started with `docker run` rather
than in an sbx sandbox, use `dsyver` and the plain-container profile instead:
[checking an agent sandbox](https://syver.readthedocs.io/en/latest/containers/agent-sandboxes/).
The two profiles check different things, because an sbx sandbox gives the
agent sudo and its own Docker daemon by design.

## Install

Copy `sbxsyver` to a directory on your `PATH`:

```sh
curl -fsSLo ~/bin/sbxsyver https://raw.githubusercontent.com/krameff/syver/main/extras/sbxsyver/sbxsyver
chmod +x ~/bin/sbxsyver
```

It needs `sbx`, signed in, and a **Linux** syver binary for the host's CPU
architecture. A sandbox is always a Linux VM, so on macOS or Windows the syver
you run locally is the wrong build: download the `linux` release for your
architecture and point `SYVER_PATH` at it. sbxsyver refuses to guess on a
non-Linux host.

## Usage

```sh
sbxsyver run <sandbox> [sbx exec flags]
```

`<sandbox>` is a name from `sbx ls`. Anything after it is passed to `sbx exec`
before the sandbox name, so `sbxsyver run my-sandbox -u root` runs the checks as
root. By default they run as the sandbox's own user, which is what the agent
sees.

The exit status is syver's: 0 when every check passes, 1 when any fails.

### Environment variables

Each `SYVER_*` name has a `GOSS_*` equivalent, as in the other wrappers. An
exported but empty `SYVER_*` is treated as unset.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SYVER_PATH` | `syver` on `PATH` (Linux hosts only) | The syver binary to copy in |
| `SYVER_FILES_PATH` | `.` | Directory holding the spec and vars |
| `SYVER_FILE` | first of `syver.yaml`, `syver.yml`, `goss.yaml`, `goss.yml` | Spec file name |
| `SYVER_VARS` | unset | Vars file name, relative to `SYVER_FILES_PATH` |
| `SYVER_OPTS` | `--color --format documentation` | Options for `syver validate` |
| `SYVER_WAIT_OPTS` | `-r 30s -s 1s > /dev/null` | Options for the wait spec |
| `SYVER_TEMP_DIR` | `/tmp` | Where the files are staged on the host |
| `SBX_BIN` | `sbx` | The sbx CLI |

If a wait spec (`syver_wait.yaml`, `syver_wait.yml`, `goss_wait.yaml` or
`goss_wait.yml`) is present, it is run first and must pass before the main spec.

## Checking an agent sandbox

```sh
sbx create --name my-sandbox claude ~/src/my-project
cd examples/agent-sandbox-sbx    # after editing vars.yaml
SYVER_VARS=vars.yaml sbxsyver run my-sandbox
```

### What the profile checks

An sbx sandbox is not an ordinary container, and a profile written for one gets
it wrong. Inside the VM the agent has sudo and its own Docker daemon by design,
and sbx sets credential variables to placeholders that its host-side proxy
swaps for the real keys. So this profile does NOT check for a non-root user, an
absent Docker socket or absent credential variables. It checks what sbx
promises instead:

| Section | Asserts | Catches |
| --- | --- | --- |
| Sandbox | `SANDBOX_ID` is set | running the profile somewhere other than sbx |
| Toolchain | each command in `tools` runs | a missing or broken tool, before the agent hits it |
| Workspace | `WORKSPACE_DIR` exists and is writable | a missing or read-only workspace |
| Docker | the daemon reports the sandbox's own name | a host daemon reaching the sandbox |
| Proxied credentials | each variable in `proxied_env` is unset or the sbx placeholder | a real key passed in with `sbx create -e` |
| Other credentials | each variable in `absent_env` is unset | a token sbx does not manage, passed in with `-e` |
| Credential files | no SSH keys, AWS, `gh` or git credentials in `/home/agent` | credentials baked into a custom template |
| Host credential directories | `~/.ssh`, `~/.aws`, `~/.config/gh`, `~/.kube` under `host_home` are absent | a credential directory passed to `sbx create` as an extra workspace, which mounts it at its host path |
| Egress | hosts in `egress_allow` are reachable, hosts in `egress_deny` are not | a network policy rule that allows more than intended |

A host the network policy denies is refused at connect, so `reachable: false`
tests the policy itself. The defaults in `vars.yaml` match the `balanced`
policy; adjust both lists to yours.

A correctly configured sandbox passes every check. Against one created with a
token and an API key passed in with `-e`, the host's `~/.aws` mounted as an
extra workspace and a sandbox rule allowing `example.com`, the profile fails
exactly those four checks and exits 1. Neither value it was given appears in
the output: the variable checks print nothing.

### What it cannot see

* **Check the sandbox before the agent has used it.** The agent has sudo inside
  the VM, so once it has run it could change anything syver reads, including
  syver itself. A pass is evidence about a sandbox the agent has not touched
  yet.
* **The placeholder formats are sbx's, not syver's.** The profile accepts
  `proxy-managed` and `gho_sbxproxymanaged...`, the values sbx v0.46.0 sets. If
  a later sbx changes them, the proxied-credential checks fail on a safe
  sandbox, and the patterns in `syver.yaml` need updating.
* **The policy is checked by example.** The egress checks prove the hosts you
  list are allowed or denied. They do not read the policy, so a rule allowing a
  host you did not list goes unnoticed.
