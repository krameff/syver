# ksyver

ksyver is a wrapper for syver that aims to bring the simplicity of testing
with syver to containers running in pods in Kubernetes.

`kgoss` is the previous name of this script and is kept as a thin forwarding
shim for one major version -- it behaves identically to `ksyver`. New scripts
and documentation should use `ksyver`.

ksyver is a script which when invoked copies and runs syver (the binary) within a
Linux container. syver itself is only supported on Linux, but since it need only
run in the target container, the ksyver script can be used from any
bash-compatible shell, including Terminal on Mac and git-bash on Windows. On
Windows, [winpty][] is used for interactive connections to the pod under test.

[winpty]: https://github.com/rprichard/winpty

## Install

Installing kgoss requires copying the kgoss file to a directory in your PATH
and copying the goss file to your home folder (or a path set as `GOSS_PATH`),
as follows.

### Manual / UI

You can manually install kgoss and goss by going through the Web UI, getting
the files and putting them in the right path. To get each of them:

* **kgoss**: Run `curl -sSLO
  https://raw.githubusercontent.com/krameff/syver/main/extras/kgoss/kgoss`.
* **goss**: Download a release archive such as `goss_0.5.0_linux_x86_64.tar.gz`
  from <https://github.com/krameff/syver/releases>, extract it, and rename the
  binary `goss`. Place it in your HOME directory, e.g. `C:\Users\<username>` on
  Windows; or set the environment variable `GOSS_PATH` to its path. Or run
  `curl -fsSL https://raw.githubusercontent.com/krameff/syver/main/install.sh | sh`.

### Automatic / CLI

To install from the command line or automatically, use the following commands.
[jq][] is required to parse the API response and find the release asset's
download URL.

[jq]: https://stedolan.github.io/jq

First get a GitHub personal access token for accessing the GitHub API from
<https://github.com/settings/tokens>. Input it in the first
line below. Set `dest_dir` to a directory in your `PATH` env var.

```shell
token=<personal_access_token>
username=$(whoami)
dest_dir=${HOME}/bin

host=raw.githubusercontent.com
repo=krameff/syver
# for private repos, replace:
# host=github.yourcompany.com
# repo=org-name/goss

## install kgoss
curl -sSL -u "${username}:${token}" -H 'Accept: application/vnd.github.v3.raw' -o "${dest_dir}/kgoss" \
  https://${host}/api/v3/repos/${repo}/contents/extras/kgoss/kgoss
chmod a+rx "${dest_dir}/kgoss"

## install goss
if [[ ! $(which jq) ]]; then echo "jq is required, get from https://stedolan.github.io/jq"; fi
version=v0.5.0
arch=x86_64
asset="goss_${version#v}_linux_${arch}.tar.gz"
host=github.com
# for private repos, leave `host` blank or same as above:
# host=github.yourcompany.com
dl_url=$(curl -sSL -u "${username}:${token}" https://${host}/api/v3/repos/${repo}/releases \
  | jq -r ".[] | select (.tag_name == \"${version}\") | .assets[] | select (.name == \"${asset}\") | .url")
tmpdir=$(mktemp -d)
curl -sSL -u "${username}:${token}" -H 'Accept: application/octet-stream' -o "${tmpdir}/${asset}" "${dl_url}"
tar xzf "${tmpdir}/${asset}" -C "${tmpdir}"
mv "${tmpdir}/goss" "${dest_dir}/goss"
chmod a+rx "${dest_dir}/goss"

# If `goss` is not in your path, export a GOSS_PATH variable:
export GOSS_PATH=${dest_dir}/goss

# Now you can use kgoss as described below:
# kgoss edit ...
# kgoss run ...
```

## Use

`ksyver [run|edit] -i <image_url> [-p | -c "command to run" | -a "args to pass"] [-d "directory to include"]* [-e "k=v"]*`
(or `kgoss ...`, the compat shim)

If none of `-p|-c|-a` are specified the container is run with its configured entry point.

`-d` and `-e` can be specified multiple (or zero) times to add additional
directories and env vars.

By default kgoss copies `goss.yaml` from the current working directory and
nothing else. You may need other files like scripts and configurations copied
as well. Specify `-d <path_to_dir>` for each additional directory you'd like
to recursively copy. These will be copied as directories next to `goss.yaml`
in the target container's `GOSS_CONTAINER_PATH`.

To find `goss.yaml` in another directory specify that directory's path in `GOSS_FILES_PATH`.

### Run

The `run` command is used to validate a container. It expects a
`./goss.yaml` file to exist in the directory it was invoked from.

If the file `./goss_wait.yaml` exists in the current directory, goss regularly
checks whether the conditions in the file are met. Only then does goss start the
actual check with the file `./goss.yaml`. This is used, for example, to wait
until a certain port is open before executing the tests.

**Example:**

`kgoss run -e JENKINS_OPTS="--httpPort=8080 --httpsPort=-1" -e JAVA_OPTS="-Xmx1048m" -i jenkins:alpine`

`kgoss run` will do the following:

* Run the container with the start commands specified by `-c`, `-a`, or `-p`.
* Run `goss` with `$GOSS_WAIT_OPTS` if `./goss_wait.yaml` file exists in the current dir.
* Run `goss` with `$GOSS_OPTS` using `./goss.yaml` from `GOSS_FILES_PATH`.

### Edit

Edit will launch a container, install goss, and drop the user into an
interactive shell. Once the user quits the interactive shell, any `goss.yaml`
or `goss_wait.yaml` are copied out into the current directory. This allows the
user to leverage the `goss add|autoadd` commands to write tests as they would
on a regular machine.

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
