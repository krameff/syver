# Agent sandbox profile

A syver spec for checking a Docker container that a coding agent runs in. It
asserts that the agent has what it needs and, just as important, that the
operator's credentials and the host's escape routes are NOT in reach.

| File | What it is |
| --- | --- |
| `syver.yaml` | The profile. Sections are independent; delete any that do not apply |
| `vars.yaml` | Everything that varies between sandboxes: paths, tools, hosts, variable names |

Run it with the same arguments you start the sandbox with:

```sh
SYVER_VARS=vars.yaml dsyver run \
  --cap-drop=ALL --security-opt no-new-privileges --read-only --memory 2g \
  -v "$PWD:/workspace" my-agent-image
```

The walkthrough, including what each section catches and what it cannot see, is
in [docs/containers/agent-sandboxes.md](../../docs/containers/agent-sandboxes.md).
