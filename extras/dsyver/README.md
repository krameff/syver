# dsyver

dsyver is a convenience wrapper around syver that aims to bring the simplicity of syver to containers.

`dgoss` is the previous name of this script and is kept as a thin forwarding
shim for one major version -- it behaves identically to `dsyver`. New scripts
and documentation should use `dsyver`. Everything below applies equally to
both names unless noted otherwise; examples use `dgoss` in a few places for
historical continuity, but `dsyver` works the same way.

## Examples and Tutorials

**Start here:** [testing container images](https://syver.readthedocs.io/en/latest/containers/testing-images/)
is a step-by-step walkthrough -- what dsyver does, a worked example against a
real image, what a failure looks like, and how to wire it into CI. This file is
the reference for every flag and environment variable.

The three links below predate the rename and describe `dgoss`, the original
upstream tool. They remain conceptually accurate, since `dsyver` behaves
identically, but the commands and branding are goss's.

* [blog tutorial](https://medium.com/@aelsabbahy/tutorial-how-to-test-your-docker-image-in-half-a-second-bbd13e06a4a9) -
Introduction to dgoss tutorial
* [video tutorial](https://youtu.be/PEHz5EnZ-FM) - Same as above, but in video format
* [dgoss-examples](https://github.com/aelsabbahy/dgoss-examples) - Repo containing examples of using dgoss to validate
container images

## Installation

### Linux

Follow the syver [installation instructions](https://github.com/krameff/syver#installation)

### Mac OSX

Since syver runs on the target container, dgoss can be used on a Mac OSX system by doing the following:

```shell
# Install dgoss
curl -L https://raw.githubusercontent.com/krameff/syver/main/extras/dsyver/dgoss -o /usr/local/bin/dgoss
chmod +rx /usr/local/bin/dgoss

# Download the desired syver version to your preferred location (e.g. v0.7.0).
# Release assets are raw, uncompressed binaries -- there is no tarball to extract.
curl -L "https://github.com/krameff/syver/releases/download/v0.7.0/syver-darwin-arm64" \
  -o ~/Downloads/syver
chmod +rx ~/Downloads/syver
# Point SYVER_PATH at it (GOSS_PATH is still honoured)
export SYVER_PATH=~/Downloads/syver

# Set SYVER_TEMP_DIR to the tmp directory in your home, since /tmp is private on Mac OSX
# (DGOSS_TEMP_DIR is still honoured)
export SYVER_TEMP_DIR=~/tmp

# Use dgoss
dgoss edit ...
dgoss run ...
```

## Usage

`dsyver [run|edit] <docker_run_params>` (or `dgoss [run|edit] <docker_run_params>`, the compat shim)

### Run

Run is used to validate a container.
It expects a `./syver.yaml` file to exist in the directory it was invoked from.
`syver.yml`, `goss.yaml` and `goss.yml` are also accepted, in that order of
preference, so an existing goss-named spec keeps working untouched.

If the file `./syver_wait.yaml` exists in the current directory, syver regularly
checks whether the conditions in the file are met. Only then does syver start the
actual check with the file `./syver.yaml`. This is used, for example, to wait
until a certain port is open before executing the tests. The wait file is probed
the same way: `syver_wait.yaml`, `syver_wait.yml`, `goss_wait.yaml`,
`goss_wait.yml`.

In most cases one can just substitute the runtime command (`docker`, `nerdctl` or `podman`)
for the dgoss command, for example:

**run:**

`docker run -e JENKINS_OPTS="--httpPort=8080 --httpsPort=-1" -e JAVA_OPTS="-Xmx1048m" jenkins:alpine`

**test:**

`dgoss run -e JENKINS_OPTS="--httpPort=8080 --httpsPort=-1" -e JAVA_OPTS="-Xmx1048m" jenkins:alpine`

`dgoss run` will do the following:

* Run the container with the flags you specified.
* Stream the containers log output into the container as `/syver/docker_output.log`
    * This allows writing tests or waits against the container output
* (optional) Run `goss` with `$GOSS_WAIT_OPTS` if `./goss_wait.yaml` file exists in the current dir
* Run `goss` with `$GOSS_OPTS` using `./goss.yaml`

### Edit

Edit will launch a container, install syver, and drop the user into an interactive shell.
Once the user quits the interactive shell, any `syver.yaml` or `syver_wait.yaml`
are copied out into the current directory. The goss-named equivalents are picked
up too.
This allows the user to leverage the `syver add|autoadd` commands to write tests as they would on a regular machine.

**Example:**

`dgoss edit -e JENKINS_OPTS="--httpPort=8080 --httpsPort=-1" -e JAVA_OPTS="-Xmx1048m" jenkins:alpine`

### Environment vars and defaults

The following environment variables can be set to change the behavior of `dsyver`.

**The `SYVER_*` names below are the current ones. Every `GOSS_*` equivalent still
works and always will** — `GOSS_FILES_PATH`, `GOSS_OPTS` and the rest are read
whenever the `SYVER_*` name is unset, so an existing setup keeps working with no
change at all. Where both are set, the `SYVER_*` name wins, and an
exported-but-empty `SYVER_*` is treated as unset so it can never shadow a real
`GOSS_*`. This matches the dual-prefix scheme the `syver` binary uses for its own
environment variables.

`SYVER_TEMP_DIR` is paired with `DGOSS_TEMP_DIR` rather than a `GOSS_*` name,
because that variable's legacy name carries the script prefix and there has never
been a `GOSS_TEMP_DIR`. The contract is identical to every other pair.

#### DEBUG

Enables debug output of `dsyver`.

When running in debug mode, the tmp dir with the container output will not be cleaned up.

Note: Debug output of `dsyver` is from the `dsyver` shell script and not debug output of `syver`
(`dsyver run -e SYVER_LOGLEVEL=DEBUG nginx:alpine`).

**Default:** empty

**Example:**

`DEBUG=true dsyver run nginx:alpine`

#### SYVER_PATH

Location of the syver (or legacy goss) binary to use. (Default: `$(which syver)`, falls back to `$(which goss)`)

Goss-named equivalent: `GOSS_PATH`.

#### SYVER_FILE

Name of the spec file to use. When unset, the first of `syver.yaml`,
`syver.yml`, `goss.yaml`, `goss.yml` that exists is used, in that order --
the same probe the binary itself performs. Setting this pins one filename
and skips the probe.

#### SYVER_OPTS

Options to use for the syver test run. (Default: `--color --format documentation`)

#### SYVER_WAIT_OPTS

Options to use for the syver wait run, when `./syver_wait.yaml` exists. (Default: `-r 30s -s 1s > /dev/null`)

#### SYVER_SLEEP

Time to sleep after running container (and optionally `syver_wait.yaml`) and before running tests. (Default: `0.2`)

#### SYVER_FILES_PATH

Location of the spec files. (Default: `.`)

#### SYVER_ADDITIONAL_COPY_PATH

Colon-separated list of additional directories to copy to container.

By default `dsyver` copies `syver.yaml` from the current working directory and
nothing else. You may need other files like scripts and configurations copied
as well. Specify `SYVER_ADDITIONAL_COPY_PATH` similar to `$PATH` as colon separated
list of directories for each additional directory you'd like to recursively copy.
These will be copied as directories next to the spec in the temporary
directory `DGOSS_TEMP_DIR`. (Default: `''`)

#### SYVER_VARS

The name of the variables file relative to `SYVER_FILES_PATH` to copy into the
container and use for validation (i.e. `dsyver run`) and copy out of the
container when writing tests (i.e. `dsyver edit`). If set, the
`--vars` flag is passed to `syver validate` commands inside the container.
If unset (or empty), the `--vars` flag is omitted, which is the normal behavior.
(Default: `''`).

#### SYVER_FILES_STRATEGY

Strategy used for copying goss files into the container. If set to `'mount'` a volume with goss files is mounted
and log output is streamed into the container as `/syver/docker_output.log` file. Other strategy is `'cp'` which uses
`'docker cp'` command to copy goss files into container. With the `'cp'` strategy you lose the ability to write
tests or waits against the container output. The `'cp'` strategy is required especially when container daemon is
not on the local machine. It is not supported with rootless `nerdctl`, which cannot copy into a container before it
starts, and the wrapper stops with an error in that case rather than failing partway.
(Default `'mount'`)

#### CONTAINER_LOG_OUTPUT

Location of the file that contains tested container logs. Logs are retained only if the variable is set to a non-empty
string. (Default `''`)

#### SYVER_TEMP_DIR

Location of the temporary directory used by `dsyver` and by the `dgoss` shim.
(Default `'$(mktemp -d /tmp/tmp.XXXXXXXXXX)'`)

Legacy equivalent: `DGOSS_TEMP_DIR`, still honoured. Note the legacy name is
`DGOSS_`, not `GOSS_` -- it is named after the script rather than the product,
and `GOSS_TEMP_DIR` has never existed.

#### CONTAINER_RUNTIME

Container runtime to use - `docker`, `nerdctl` or `podman`. Defaults to `docker`. Note that `podman` requires a run command
to keep the container running. This defaults to `sleep infinity` in case only an image is passed to `dsyver` commands.

With `nerdctl`, the SELinux relabel requested on the mounted directory (`:z`) takes effect only when `nerdctl` itself runs
with `--selinux-enabled`. If a container on an SELinux-enforcing host cannot read the mounted files, use
`SYVER_FILES_STRATEGY=cp` with rootful `nerdctl`.
