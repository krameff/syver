# ksyver

ksyver is a wrapper for syver that aims to bring the simplicity of testing
with syver to containers running in pods in Kubernetes.

`kgoss` is the previous name of this script and is kept as a compatibility copy,
under the [compatibility policy](https://syver.readthedocs.io/en/latest/goss-vs-syver/#compatibility-policy):
maintained, but new features arrive in `ksyver` only. It takes the same commands, variables and spec files as
`ksyver`; the difference is inside the pod, where `GOSS_CONTAINER_PATH` defaults
to `/tmp/goss` rather than `/tmp/syver` and the copied binary is named `goss`.
New scripts and documentation should use `ksyver`.

ksyver is a script which when invoked copies and runs syver (the binary) within a
Linux container. syver itself is only supported on Linux, but since it need only
run in the target container, the ksyver script can be used from any
bash-compatible shell, including Terminal on Mac and git-bash on Windows. On
Windows, [winpty][] is used for interactive connections to the pod under test.

[winpty]: https://github.com/rprichard/winpty

## Install

ksyver needs two files on the machine you run it from: the `ksyver` script, in
a directory on your `PATH`, and a **Linux** syver binary for your pod's
architecture, which ksyver copies into the pod. Release binaries are named
`syver-linux-<arch>` (`amd64`, `arm64` or `s390x`); see
[Releases](https://github.com/krameff/syver/releases).

```shell
dest_dir=${HOME}/bin
arch=amd64   # the pod's architecture, not necessarily this machine's

curl -fsSL -o "${dest_dir}/ksyver" \
  https://raw.githubusercontent.com/krameff/syver/main/extras/ksyver/ksyver
chmod a+rx "${dest_dir}/ksyver"

curl -fsSL -o "${dest_dir}/syver" \
  "https://github.com/krameff/syver/releases/latest/download/syver-linux-${arch}"
chmod a+rx "${dest_dir}/syver"
```

ksyver finds the binary as `syver` on your `PATH`, or as `~/syver` or
`~/bin/syver`; otherwise set `GOSS_PATH` to it. On Windows, put it in your home
directory, e.g. `C:\Users\<username>`, or set `GOSS_PATH`.

## Use

`ksyver [run|edit] -i <image_url> [-p | -c "command to run" | -a "args to pass"] [-d "directory to include"]* [-e "k=v"]*`
(or `kgoss ...`, the compat shim)

If none of `-p|-c|-a` are specified the container is run with its configured entry point.

`-d` and `-e` can be specified multiple (or zero) times to add additional
directories and env vars.

By default ksyver and kgoss copy one spec file from the current working
directory and nothing else: the first of `syver.yaml`, `syver.yml`, `goss.yaml`
and `goss.yml`, staged in the pod as `goss.yaml`, plus a wait file found the
same way. You may need other files like scripts and configurations copied
as well. Specify `-d <path_to_dir>` for each additional directory you'd like
to recursively copy. These will be copied as directories next to `goss.yaml`
in the target container's `GOSS_CONTAINER_PATH`.

To find the spec in another directory, specify that directory's path in `GOSS_FILES_PATH`.

### Run

The `run` command is used to validate a container. It uses the first of
`syver.yaml`, `syver.yml`, `goss.yaml` and `goss.yml` found in
`GOSS_FILES_PATH`, the current directory by default.

If a wait file exists there (`syver_wait.yaml`, `syver_wait.yml`,
`goss_wait.yaml` or `goss_wait.yml`), syver regularly checks whether the
conditions in it are met. Only then does syver start the actual check. This is used, for example, to wait
until a certain port is open before executing the tests.

**Example:**

`kgoss run -e JENKINS_OPTS="--httpPort=8080 --httpsPort=-1" -e JAVA_OPTS="-Xmx1048m" -i jenkins:alpine`

`kgoss run` will do the following:

* Run the container with the start commands specified by `-c`, `-a`, or `-p`.
* Run `goss` with `$GOSS_WAIT_OPTS` if `./goss_wait.yaml` file exists in the current dir.
* Run `goss` with `$GOSS_OPTS` using `./goss.yaml` from `GOSS_FILES_PATH`.

### Edit

Edit will launch a container, install goss, and drop the user into an
interactive shell. Once the user quits the interactive shell, the spec and wait
file are copied back to the files they were read from, so edits to a
`syver.yaml` land in `syver.yaml`. On a new project, the `syver.yaml` that
`syver add` creates is copied out under that name, into `GOSS_FILES_PATH`. This
allows the user to leverage the `goss add|autoadd` commands to write tests as
they would on a regular machine.

**Example:**

`kgoss edit -e JENKINS_OPTS="--httpPort=8080 --httpsPort=-1" -e JAVA_OPTS="-Xmx1048m" -i jenkins:alpine`

## Environment variables

The following environment variables effect the behavior of kgoss.

Every `GOSS_*` variable below also has a `SYVER_*` twin — `SYVER_FILES_PATH`,
`SYVER_OPTS`, and so on. The `SYVER_*` name wins when it is set to a non-empty
value; otherwise the `GOSS_*` name is used, so existing setups keep working
unchanged. An exported-but-empty `SYVER_*` is treated as unset and never
shadows a real `GOSS_*`. This matches the dual-prefix scheme the `syver`
binary itself uses for its own environment variables.

Variable | Description | Default
-------- | ----------- | -------
GOSS\_PATH | Local location of a compatible syver (or legacy goss) binary to use in container | `$(which syver)`, falls back to `$(which goss)`
GOSS\_FILES\_PATH | Location of the goss yaml files | `.`
GOSS\_KUBECTL\_BIN | Kubenetes client tool to use | `$(which kubectl)`
GOSS\_KUBECTL\_OPTS | Options to inject more options such as "--namespace=default" | ""
GOSS\_OPTS | Options to use for the goss test run. | `--color --format documentation`
GOSS\_WAIT\_OPTS | Options to use for the goss wait run, when `./goss_wait.yaml` exists. | `-r 30s -s 1s > /dev/null`
GOSS\_VARS | Variables file relative to `GOSS_FILES_PATH` to copy and use | ""
GOSS\_CONTAINER\_PATH | Path within container to put goss binary and YAML files | `/tmp/goss`
