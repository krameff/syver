# Testing container images

`dsyver` runs syver **inside** a container so you can assert what is actually in
an image, from the image's own point of view. This page is the walkthrough; the
[dsyver reference](docker.md) documents every flag and environment variable.

## The problem it solves

An image builds successfully and is still wrong. The base image bumped a major
version and dropped a package. A `COPY` landed a config file one directory
higher than intended. The service runs as `root` because a `USER` line was lost
in a rebase. None of that fails the build, and none of it is visible in
`docker images`.

The usual answer is to start the container, curl something, and hope that
exercises the parts that matter. That tests one path slowly and says nothing
about the other ninety nine.

`dsyver` inverts it: you declare what the image should contain, and it checks
the whole list in a fraction of a second.

## What it actually does

1. Starts your container with the exact arguments you would have passed to
   `docker run`.
2. Puts the syver binary and your spec inside it, by bind mount or `docker cp`.
3. Runs the checks **from within the container**, so paths, users, packages and
   ports are seen as the application sees them.
4. Stops and removes the container, and exits non-zero if anything failed.

That last point is what makes it a CI gate rather than a report.

## Walkthrough

Every command and every output below was run against `nginx:alpine`.

### 1. Get a syver binary the container can execute

This is the step that trips people up, and the error is unhelpful when it goes
wrong.

The binary runs **inside** your container, so it has to work with that image's C
library. A binary built with cgo enabled is dynamically linked against glibc and
will not run on an Alpine (musl) image. The symptom is not "wrong architecture",
it is:

```console
sh: /syver/syver: not found
```

The file is plainly there. The *loader* is missing.

Released syver binaries are statically linked and work everywhere. If you are
building your own, build it static:

```sh
CGO_ENABLED=0 go build -o /tmp/syver ./cmd/syver
export SYVER_PATH=/tmp/syver
```

### 2. Write a spec

Create `syver.yaml` next to where you will run `dsyver`:

```yaml
file:
  /etc/nginx/nginx.conf:
    exists: true
    filetype: file
user:
  nginx:
    exists: true
command:
  nginx -v:
    exit-status: 0
    stderr:
      - "/nginx version: nginx\\/1\\./"
```

You do not have to write it by hand. `dsyver edit <image>` starts the container,
installs syver, and drops you into a shell, where `syver add` and `syver autoadd`
generate assertions from what is really there. When you exit the shell, the spec
is copied back out to your working directory.

### 3. Run it

```sh
dsyver run nginx:alpine
```

```console
INFO: Starting docker container
INFO: Container ID: 2ab55154
INFO: Running Tests
User: nginx: exists: matches expectation: true
File: /etc/nginx/nginx.conf: exists: matches expectation: true
File: /etc/nginx/nginx.conf: filetype: matches expectation: "file"
Command: nginx -v: exit-status: matches expectation: 0
Command: nginx -v: stderr: matches expectation: ["/nginx version: nginx\\/1\\./"]

Total Duration: 0.005s
Count: 5, Failed: 0, Skipped: 0
INFO: Stopping container
```

Five checks, five milliseconds, exit code 0.

### 4. See what a failure looks like

Add an assertion for a file the image does not have:

```yaml
file:
  /etc/nginx/conf.d/tls.conf:
    exists: true
```

```console
Failures/Skipped:

File: /etc/nginx/conf.d/tls.conf: exists:
Expected
    false
to equal
    true

Total Duration: 0.000s
Count: 3, Failed: 1, Skipped: 0
```

`dsyver` exits **1**. It names the assertion, what it expected and what it found,
so the failure is actionable without reading the container's logs.

### 5. Put it in CI

`dsyver run` is a single command with a meaningful exit code, so it goes
straight after the build:

```sh
docker build -t myapp:ci .
dsyver run myapp:ci        # non-zero fails the job
```

Run it against the image you are about to push, not against a rebuild, so what
you tested is what you ship.

## Waiting for something to be ready

If `./syver_wait.yaml` exists, `dsyver` runs it first and retries until it passes
before starting the real checks. Use it for anything that is not ready the
instant the container starts:

```yaml
port:
  tcp:80:
    listening: true
```

This is the difference between a flaky image test and a reliable one. A check
that fails because you looked too early is worse than no check, because it
teaches people to re-run the job.

## Other runtimes

`dsyver` is not docker-only. Set `CONTAINER_RUNTIME` to `podman` or `nerdctl`:

```sh
CONTAINER_RUNTIME=podman dsyver run nginx:alpine
```

`podman` needs a command to keep the container alive; when you pass only an
image, `dsyver` supplies `sleep infinity` for you.

If the container daemon is not on your machine, the bind mount cannot work.
Switch to `SYVER_FILES_STRATEGY=cp`, which copies the files in instead. You lose
the ability to assert against the container's log output, which is streamed in
as `/syver/docker_output.log` under the default `mount` strategy.

## What it does not do

`dsyver` validates an image **from the inside**. Container-level configuration --
dropped capabilities, a read-only root filesystem, the network mode, memory
limits, the mount list -- is a property of how the container was *started*, not
of what is in it. Some of it is visible from inside through `/proc` and the
cgroup files, but if that is what you want to assert, do it from the host with
`command:` and `docker inspect`.

## Reference

Every flag, environment variable and default:
[the dsyver reference](docker.md), which is `extras/dsyver/README.md` rendered here.

For running syver inside an image you publish, rather than testing one, see
[the container image page](../container_image.md).
