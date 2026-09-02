# Command Line Interface

## Usage

```console
NAME:
   syver - Quick and Easy server validation

USAGE:
   syver [global options] [command [command options]]

COMMANDS:
   validate, v  Validate system
   serve, s     Serve a health endpoint
   render, r    render gossfile after imports
   autoadd, aa  automatically add all matching resource to the test suite
   add, a       add a resource to the test suite
   help, h      Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --log-level string, --loglevel string, -L string, -l string  Goss log verbosity level (default: "INFO") [$SYVER_LOGLEVEL, $GOSS_LOGLEVEL]
   --syverfile string, --gossfile string, -g string              Syver file to read from / write to [$SYVER_FILE, $GOSS_FILE]
   --vars string [ --vars string ]                               json/yaml file containing variables for template. Can be specified multiple times. Later files override overlapping keys. [$SYVER_VARS, $GOSS_VARS]
   --vars-inline string                                          json/yaml string containing variables for template (overwrites vars) [$SYVER_VARS_INLINE, $GOSS_VARS_INLINE]
   --package string                                              Package type to use [apk, dpkg, pacman, rpm]
   --help, -h                                                    show help
   --version, -v                                                 print the version
```

!!! note
    Most flags can be set by using environment variables, see `--help` for more info.
    `SYVER_*` variables are checked first; the legacy `GOSS_*` variables are
    still honored as a fallback for one major version.

## Global options

`--syverfile/--gossfile/-g <syverfile>`
:   The file to use when reading/writing tests. This flag controls the
    **CLI file path** (which file syver opens); it is unrelated to the
    **YAML import key** inside that file. Note the two use the same two
    names for different purposes: at the CLI level `--syverfile` is the
    primary flag and `--gossfile`/`-g` are aliases of it; inside the
    YAML/JSON content itself, the reverse is true: `gossfile:` is the
    canonical, emitted import key, and `syverfile:` is an input-only
    alias for it, folded in at decode time and never written back out by
    `render`. Use `--syverfile -`, `--gossfile -`, or `-g -` to read from
    `STDIN`. When unset, the first of `syver.yaml`, `syver.yml`,
    `goss.yaml`, `goss.yml` found in the current directory is used; if none
    exist, `add`/`autoadd` create `./syver.yaml`.

    Valid formats:
    * `yaml` *(default)*
    * `json`

`--vars <varfile>`
:   Files to read variables from when rendering gossfile [templates](gossfile.md#templates).
    Can be specified multiple times. Later files override overlapping keys.

    Valid formats:

    * `yaml` *(default)*
    * `json`

`--package <type>`
:   The package type to check for.

    Valid options are:

    * `apk`
    * `dpkg`
    * `pacman`
    * `rpm`

## Commands

Commands are the actions syver can run.
* [add](#add): add a single test for a resource
* [autoadd](#autoadd): automatically add multiple tests for a resource
* [render](#render): renders and outputs the gossfile, importing all included gossfiles
* [serve](#serve): serves the gossfile validation as an HTTP endpoint on a specified address and port,
    so you can use your gossfile as a health report for the host
* [validate](#validate): runs the syver test suite on your server

### `add`

!!! abstract "Add system resource to test suite"
    ```console
    syver add [--exclude-attr <pattern>] <test> [<test>]
    syver a [--exclude-attr <pattern>] <test> [<test>]
    ```

This will add a test for a resource. Non existent resources will add a test to ensure they do not exist on the system.
A sub-command *resource type* has to be provided when running `add`.

`--exclude-attr`
:   Ignore **non-required** attribute(s) matching the provided glob when adding a new resource,
    may be specified multiple times.

!!! example
    ```console
    syver add file /etc/passwd
    syver a user nobody
    syver add --exclude-attr home --exclude-attr shell user nobody
    syver a --exclude-attr '*' user nobody
    ```

#### Resources types

| Type                                       | Description                                                                                                                     |
|--------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------|
| [`addr`](gossfile.md#addr)                 | Verify if a remote `address:port` is reachable                                                                                  |
| [`command`](gossfile.md#command)           | Run a [command](gossfile.md#command) and validate the exit status and/or output                                                 |
| [`dns`](gossfile.md#dns)                   | Resolves a [dns](gossfile.md#dns) name and validates the addresses                                                              |
| [`file`](gossfile.md#file)                 | Validate a [file](gossfile.md#file) existence, permissions, stats (size, etc) and contents                                      |
| [`syver`](gossfile.md#gossfile)            | Includes the contents of another [gossfile](gossfile.md) (alias: `goss`)                                                        |
| [`group`](gossfile.md#group)               | can validate the existence and values of a [group](gossfile.md#group) on the system                                             |
| [`http`](gossfile.md#http)                 | Validate the HTTP response code, headers, and content of a URI                                                                  |
| [`interface`](gossfile.md#interface)       | Validate the existence and values (es. the addresses) of a network interface                                                    |
| [`kernel-param`](gossfile.md#kernel-param) | Validate kernel parameters (sysctl values)                                                                                      |
| [`mount`](gossfile.md#mount)               | Validate the existence and options relative to a [mount](gossfile.md#mount) point                                               |
| [`package`](gossfile.md#package)           | Validate the status of a [package](gossfile.md#package) using the package manager specified on the commandline with `--package` |
| [`port`](gossfile.md#port)                 | Validate the status of a local [port](gossfile.md#port), for example `80` or `udp:123`                                          |
| [`process`](gossfile.md#process)           | Validate the status of a [process](gossfile.md#process)                                                                         |
| [`registry`](gossfile.md#registry)         | Validate a Windows [registry](gossfile.md#registry) key or value. Windows only                                                  |
| [`service`](gossfile.md#service)           | Validate if a [service](gossfile.md#service) is running and/or enabled at boot                                                  |
| [`user`](gossfile.md#user)                 | Validate the existence and values of a [user](gossfile.md#user) on the system                                                   |

### `autoadd`

!!! abstract "Auto add all matching resources to test suite"
    ```console
    syver autoadd [arguments...]
    syver aa [arguments...]
    ```

Automatically [adds](#add) all **existing** resources matching the provided argument.

Will automatically add the following matching resources:
* `file` - only if argument contains `/`
* `group`
* `package`
* `port`
* `process` - Also adding any ports it's listening to (if run as root)
* `service`
* `user`

Will **NOT** automatically add:
* `addr`
* `command` - for safety
* `dns`
* `http`
* `interface`
* `kernel-param`
* `mount`

!!! example
    ```console
    syver autoadd sshd
    ```

    Generates the following `goss.yaml`

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

### `render`

!!! abstract "Render gossfile after importing all referenced gossfiles"
    ```
    syver render
    syver r
    ```

This command allows you to keep your tests separated and render a single, valid, gossfile,
by including them with the `gossfile` directive.

`--debug`
:   This prints the rendered golang template prior to printing the parsed JSON/YAML gossfile.

!!! example
    ```console
    $ cat goss_httpd_package.yaml
    package:
      httpd:
        installed: true
        versions:
        - 2.2.15

    $ cat goss_httpd_service.yaml
    service:
      httpd:
        enabled: true
        running: true

    $ cat goss_nginx_service-NO.yaml
    service:
      nginx:
        enabled: false
        running: false

    $ cat goss.yaml
    gossfile:
      goss_httpd_package.yaml: {}
      goss_httpd_service.yaml: {}
      goss_nginx_service-NO.yaml: {}

    $ syver -g goss.yaml render
    package:
      httpd:
        installed: true
        versions:
        - 2.2.15
    service:
      httpd:
        enabled: true
        running: true
      nginx:
        enabled: false
        running: false
    ```

### `serve`

!!! abstract "Serve a health endpoint"
    ```console
    syver serve [<opts>...]
    syver s [<opts>...]
    ```

`serve` exposes the syver test suite as a health endpoint on your server.
The end-point will return the stest results in the format requested and an http status of 200 or 503.

`serve` will look for a test suite in the same order as [validate](#validate)

!!! warning "The endpoint is unauthenticated"

    `serve` has no authentication, authorization or TLS of its own. Anyone who
    can reach the listen address can run the suite and read its full output --
    including the verbatim stdout and stderr of any [`command`](gossfile.md#command)
    resource, which may carry file paths, versions, or other host detail.

    Bind it to a trusted network or an address reachable only by your health
    checker, put a reverse proxy in front of it if you need TLS or auth, and
    keep secrets out of command output.

`--cache <duration>`, `-c <duration>`
:   Time to cache the results (default: 5s)

`--endpoint <endpoint>`, `-e <endpoint>`
:   Endpoint to expose (default: `/healthz`)

`--format <format>`, `-f <format>`
:   Output format, same as [validate](#validate)

`--listen-addr [ip]:port`, `-l [ip]:port`
:   Address to listen on (default: `:8080`)

`--loglevel <level>`, `-L <level>`
:   Syver logging verbosity level (default: `INFO`).
    Lower levels of tracing include all upper levels traces also (ie. `INFO` include `WARN` and `ERROR`).
    `level` can be one of:
    - `ERROR` - Critical errors that halt syver or significantly affect its functionality, requiring immediate intervention.
    - `WARN` - Non-critical issues, such as overwritten keys, an unrecognised top-level key, or deprecated features.
    - `INFO` - General operational messages, useful for tasks where a more structured output is needed (e.g. syver serve).
    - `DEBUG` - Information useful for the syver user to debug.
    - `TRACE` - Detailed internal system activities useful for syver developers to debug.

`--max-concurrent <num>`
:   Max number of tests to run concurrently

!!! example
    ```console
    $ syver serve &
    $ curl http://localhost:8080/healthz
    # JSON endpoint
    $ syver serve --format json &
    $ curl localhost:8080/healthz
    # rspecish output format in response via content negotiation
    syver serve --format json &
    curl -H "Accept: application/vnd.goss-rspecish" localhost:8080/healthz
    ```

The `application/vnd.goss-{output format}` media type can be used in the `Accept` request header
to determine the response's content-type.
You can also `Accept: application/json` to get back `application/json`.

!!! warning "`serve` is unauthenticated, and the response describes your host"

    There is no authentication, authorisation or TLS on this endpoint. Anyone
    who can reach it gets your full compliance posture: every resource the spec
    names, and whether each one passed.

    Failing checks also carry the underlying error text, which can be
    host-specific. A registry key that exists but cannot be read reports the
    access-denied error rather than simply "absent"; a user or group lookup that
    could not complete reports why. That distinction is deliberate and is what
    stops a check that never ran being reported as a check that passed, but it
    does mean the response says slightly more than pass or fail.

    Treat the endpoint, not the error text, as the thing to control. Bind it to
    an interface you trust, put it behind whatever fronts your other internal
    endpoints, and do not point `serve` at a spec naming paths you would not
    show to anyone who can reach the port.

### `validate`

!!! abstract "Validate the system"
    ```console
    syver validate [<opts>...]
    syver v [<opts>...]
    ```

`validate` runs the syver test suite on your server. Prints an rspec-like (by default) output of test results.
Exits with status 0 on success, non-0 otherwise.

`--format <format>`, `-f <format>`
:   Output format. Can be one of:
    - `documentation` - Verbose test results
    - `json` - Detailed test result on a single line (See `pretty` format option)
    - `structured` - Like `json`, but each result also carries a human-readable
      `summary-line` and `summary-line-compact`, and the document gains a
      `summary` object (`test-count`, `failed-count`, `total-duration`) plus a
      top-level `summary-line`. Useful when the same output has to be both
      machine-parsed and read by a person. Supports the `pretty` and `sort`
      format options
    - `junit`
    - `nagios` - Nagios/Sensu compatible output /w exit code 2 for failures
    - `rspecish` **(default)** - Similar to rspec output
    - `tap`
    - `prometheus` - Prometheus compatible output.
    - `silent` - No output. Avoids exposing system information (e.g. when serving tests as a healthcheck endpoint)
    - `discovery` - JSON vars output from [discovery tests](gossfile.md#discovery); always exits 0 on successful execution

`--format-options`, `-o`
:   Output format option:
    - `perfdata` - Outputs Nagios "performance data". Applies to `nagios` output
    - `verbose`  - Gives verbose output. Applies to `nagios` and `prometheus` output
    - `pretty`   - Pretty printing for the `json` output
    - `sort`     - Sorts the results

`--loglevel <level>`, `-L <level>`
:   Syver logging verbosity level (default: `INFO`).
    Lower levels of tracing include all upper levels also (ie. `INFO` includes `WARN` and `ERROR`).
    `level` can be one of:
    - `ERROR` - Critical errors that halt syver or significantly affect its functionality.
    - `WARN` - Non-critical issues, such as overwritten keys, an unrecognised top-level key, or deprecated features.
    - `INFO` - General operational messages.
    - `DEBUG` - Information useful for the syver user to debug.
    - `TRACE` - Detailed internal system activities useful for syver developers to debug.

`--max-concurrent <num>`
:   Max number of tests to run concurrently

`--discover <gossfile>`
:   Gossfile containing `discovery:` tests to run before the main `-g` gossfile. Results are
    injected as `.Discovered` for template rendering. When the main gossfile also has an inline
    `discovery:` section, the `--discover` file wins. Environment variables:
    `SYVER_DISCOVER`, `GOSS_DISCOVER` (checked in that order).

    See [discovery](gossfile.md#discovery).

`--color`/`--no-color`
:   Force color or disable color

`--retry-timeout <timeout>`, `-r <timeout>`
:   Retry on failure so long as elapsed + sleep time is less than this

    *default: `0`*

`--sleep <duration>`, `-s <duration>`
:   Time to sleep between retries

    *default: `1s`*

An unrecognised top-level key in the gossfile (a typo, or a key syver
doesn't know yet) also produces a `WARN` line here, naming the file, the
line, and a suggestion if the key is close to a real one:

```console
[WARN] syver.yaml:4: unknown top-level key "prot" -- ignored (did you mean "port"?)
```

It doesn't fail the run or change `Count`/`Failed` -- the key is still
skipped, syver just tells you it happened. See
[Unknown top-level keys](gossfile.md#unknown-top-level-keys) for the full
picture, including the two cases that are deliberately exempt.

!!! example

    ```console
    $ syver validate --format documentation
    File: /etc/hosts: exists: matches expectation: [true]
    DNS: localhost: resolvable: matches expectation: [true]
    [...]
    Total Duration: 0.002s
    Count: 10, Failed: 2, Skipped: 0

    $ syver validate -g goss.yml --discover discovery.yaml --format documentation
    File: /etc/hosts: exists: matches expectation: true
    File: /etc/hosts: contents: matches expectation: ["localhost"]
    [...]
    Total Duration: 0.000s
    Count: 2, Failed: 0, Skipped: 0

    $ syver --vars <(syver validate -g discovery.yaml --format discovery) \
        validate -g goss.yml --format documentation
    File: /etc/hosts: exists: matches expectation: true
    File: /etc/hosts: contents: matches expectation: ["localhost"]
    [...]
    Total Duration: 0.000s
    Count: 2, Failed: 0, Skipped: 0

    $ curl -s https://static/or/dynamic/goss.json | syver validate
    ...F.F
    [...]
    Total Duration: 0.002s
    Count: 6, Failed: 2, Skipped: 0

    $ syver render | ssh remote-host 'syver -g - validate'
    ......

    Total Duration: 0.002s
    Count: 6, Failed: 0, Skipped: 0

    $ syver validate --format nagios -o verbose -o perfdata
    GOSS CRITICAL - Count: 76, Failed: 1, Skipped: 0, Duration: 1.009s|total=76 failed=1 skipped=0 duration=1.009s
    Fail 1 - DNS: localhost: addrs: doesn't match, expect: [["127.0.0.1","::1"]] found: [["127.0.0.1"]]
    $ echo $?
    2
    ```
