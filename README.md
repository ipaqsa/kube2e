# kube2e

<p align="center">
  <img src="./icon.png" alt="kube2e icon" width="160">
</p>

YAML-driven end-to-end testing for Kubernetes. `kube2e` discovers suites on
disk or inside a container image, renders declarative scenarios, and runs them
against a live cluster.

CRDs must be provisioned in the cluster before running tests — kube2e does not
manage CRD lifecycle.

## Highlights

- Local or remote suite sources
- Declarative test, case, step, and action files
- Go templates with Sprig helpers
- Kubernetes actions: ensure, patch, wait, assert, delete
- Tag-based filtering with `--tags`
- Parallel suite execution with `-n`

## Install

### From source

```bash
git clone https://github.com/ipaqsa/kube2e.git
cd kube2e
make install
```

### With `go install`

```bash
go install github.com/ipaqsa/kube2e/cmd/kube2e@latest
```

### With the container image

```bash
docker pull ghcr.io/ipaqsa/kube2e:latest
docker run --rm ghcr.io/ipaqsa/kube2e:latest version
```

### From a release

Download the prebuilt binary for your platform from the
[GitHub Releases page](https://github.com/ipaqsa/kube2e/releases) and place it
on your `PATH`.

## Building

```bash
make build          # produces bin/kube2e
go build -o bin/kube2e ./cmd/kube2e
```

## Usage

### Run tests

```bash
kube2e run <dir> [flags]
```

| Flag                | Env var                  | Default | Description                                        |
|---------------------|--------------------------|---------|----------------------------------------------------|
| `--kubeconfig`      | `KUBE2E_KUBECONFIG`      | —       | Kubeconfig path; falls back to `$KUBECONFIG` then `~/.kube/config` |
| `--tags`            | `KUBE2E_TAGS`            | all     | Comma-separated tags; only matching suites/cases run |
| `-n, --parallel`    | `KUBE2E_PARALLEL`        | 1       | Number of test suites to run concurrently          |
| `--remote`          | `KUBE2E_REMOTE`          | —       | Container image that contains test suites          |
| `--remote-user`     | `KUBE2E_REMOTE_USER`     | —       | Registry username for `--remote`                   |
| `--remote-password` | `KUBE2E_REMOTE_PASSWORD` | —       | Registry password for `--remote`                   |
| `--dry-run`         | `KUBE2E_DRY_RUN`         | false   | Parse and validate tests without applying any resources |
| `-v, --verbose`     | `KUBE2E_VERBOSE`         | false   | Show `warn`-level messages (default: `info`+`error` only) |

```bash
# Run all suites under ./examples/tests
kube2e run ./examples/tests

# Run only suites and cases tagged "smoke" or "aws"
kube2e run ./examples/tests --tags smoke,aws

# Run 4 test suites in parallel
kube2e run ./examples/tests -n 4

# Run with warning messages visible
kube2e run ./examples/tests -v

# Run test suites packaged in a container image
kube2e run ./tests --remote ghcr.io/example/kube2e-tests:v0.1.0

# Run tagged tests against a staging cluster
kube2e run ./examples/tests --tags smoke --kubeconfig ~/.kube/staging.yaml

# Validate test files without touching the cluster
kube2e run ./examples/tests --dry-run
```

**Tag filtering rules:**

1. If `--tags` is not set, everything runs.
2. If `--tags a,b` is set:
   - A suite is **skipped entirely** when its `tags:` is non-empty and contains neither `a` nor `b`.
   - A suite with no `tags:` (or with a matching tag) has its cases checked individually.
   - A case runs only if it has at least one matching tag.
   - **Exception**: when the suite itself has a matching tag, *all* its cases run regardless of case-level tags.

`<dir>` is required in both modes. For local runs it points at a filesystem
directory. When `--remote` is set, kube2e pulls the image, extracts its
filesystem to a temporary directory, and discovers suites from `<dir>` inside
that extracted filesystem. Use `.` to discover suites from the image root.
Private registries can be accessed with `--remote-user` and `--remote-password`;
when the username is omitted, kube2e uses the default Docker credential
keychain.

### Build a tests image

```bash
kube2e tests build <dir> --remote <image> [flags]
```

| Flag                | Env var                              | Default | Description                              |
|---------------------|--------------------------------------|---------|------------------------------------------|
| `--remote`          | `KUBE2E_TESTS_BUILD_REMOTE`          | —       | Image reference to push                  |
| `--remote-user`     | `KUBE2E_TESTS_BUILD_REMOTE_USER`     | —       | Registry username for `--remote`         |
| `--remote-password` | `KUBE2E_TESTS_BUILD_REMOTE_PASSWORD` | —       | Registry password for `--remote`         |
| `-v, --verbose`     | `KUBE2E_VERBOSE`                     | false   | Show `warn`-level messages               |

```bash
# Package all test suites under ./examples/tests and push them
kube2e tests build ./examples/tests --remote ghcr.io/example/kube2e-tests:v0.1.0

# Push with explicit registry credentials
kube2e tests build ./examples/tests \
  --remote ghcr.io/example/kube2e-tests:v0.1.0 \
  --remote-user "$USER" \
  --remote-password "$TOKEN"
```

Only immediate child directories that contain `test.yaml` are included in the
image. The suites are written at the image root, so an image built from
`./examples/tests` can be run with:

```bash
kube2e run . --remote ghcr.io/example/kube2e-tests:v0.1.0
```

## Directory structure

```
<work-dir>/
└── <suite-name>/
    ├── test.yaml        # Suite descriptor (name, namespace)
    ├── templates/       # Go templates rendered into Kubernetes objects
    │   └── *.yaml
    └── cases/           # One YAML file per test case
        └── *.yaml
```

Templates are shared across all cases in the same suite. Cases are executed in
filesystem order. All resources applied during a case are deleted once it
finishes.

## Reference

### `test.yaml`

```yaml
name: <string>           # required — shown in log output
description: <string>
namespace: <string>      # default namespace for all objects in this suite
tags:                    # optional — filter with --tags; matching suite runs all its cases
  - <string>
```

### Case file (`cases/<name>.yaml`)

```yaml
name: <string>           # required
description: <string>
tags:                    # optional — filter with --tags when parent suite didn't already match
  - <string>
objects:                 # resource name → template base-filename (without .yaml)
  <name>: <template>    # the key is injected as the Kubernetes object name
hooks:
  before:
    - <step>             # executed before each item in steps
  after:
    - <step>             # executed after each item in steps
steps:
  - <step>
```

The `objects` map is the single place where you bind a resource name to its
template. Steps reference entries by key; the key is used as the Kubernetes
`metadata.name` in every render — you never put `name` in step values.

`hooks.before` and `hooks.after` are optional case-level step lists. They use
the same schema as normal steps and run before or after every step in `steps`.
After hooks run even when a before hook or the main step fails.

### Step

Each step runs one or more typed actions. Set any combination; absent fields
are skipped. Actions execute in a fixed order: **ensure → patch → wait → assert → delete**,
and the first failure aborts the step (unless `optional: true`).

```yaml
name: <string>           # required — shown in log output
description: <string>
optional: <bool>         # when true, failures are warned and skipped
ensure:                  # optional — create or update the object
  object: <string>       # required (flat, no target nesting)
  ...
patch:                   # optional — apply JSON patches then re-ensure
  target:
    object: <string>     # required
  ...
wait:                    # optional — poll until conditions pass
  target:
    object: <string>     # required
  ...
assert:                  # optional — one-shot condition check
  target:
    object: <string>     # required
  ...
delete:                  # optional — remove the object
  target:
    object: <string>     # required
  ...
```

Every action also accepts an optional `delay: <duration>` field that sleeps
before execution. Every action except `wait` accepts an optional `retry` block:

```yaml
retry:
  attempts: <int>         # total executions (1 = no retry)
  backoff: <duration>     # sleep between attempts (e.g. 5s)
```

### Actions

#### `ensure`

Create or update the object using Server-Side Apply (field manager `kube2e`).
Requires Kubernetes v1.22+. The object is cached for automatic cleanup.

```yaml
ensure:
  object: <string>     # required — key in the case objects map (flat, no target)
  values:              # template render inputs; name is injected automatically
    <key>: <value>
  retry:
    attempts: <int>
    backoff: <duration>
```

#### `patch`

Apply RFC 6902 JSON Patch operations to the rendered object, then re-ensure it.

```yaml
patch:
  target:
    object: <string>   # required — key in the case objects map
  patches:
    - op: add            # add | replace | remove | move | copy | test
      path: /metadata/labels/env
      value: production
    - op: replace
      path: /spec/replicas
      value: 3
    - op: remove
      path: /metadata/annotations/tmp
    - op: move
      from: /data/src
      path: /data/dst
  retry:
    attempts: <int>
    backoff: <duration>
```

#### `wait`

Poll the live object until all JQ conditions return `true`, or until the
timeout expires. Does not support retry (use `assert` for one-shot checks).

```yaml
wait:
  target:
    object: <string>   # required — key in the case objects map
  conditions:
    - <jq expression>    # must evaluate to boolean — all must pass
  interval: <duration>   # poll interval  (default: 2s)
  timeout: <duration>    # hard deadline  (default: 2m)
```

**Condition examples**

```yaml
# Deployment fully rolled out
- .status.readyReplicas == .spec.replicas

# Pod ready
- any(.status.conditions[]?; .type=="Ready" and .status=="True")

# Job completed
- (.status.succeeded // 0) >= 1

# Custom condition True
- any(.status.conditions[]?; .type=="Available" and .status=="True")
```

#### `assert`

Fetch the object once and check that all JQ conditions are true. Unlike `wait`,
there is no poll loop — use `retry` for repeated checks with backoff.

```yaml
assert:
  target:
    object: <string>   # required — key in the case objects map
  conditions:
    - <jq expression>
  retry:
    attempts: <int>
    backoff: <duration>
```

#### `delete`

Remove the object. Not-found responses are treated as success. Set `wait: true`
to block until the object disappears from the cluster.

```yaml
delete:
  target:
    object: <string>   # required — key in the case objects map
  wait: <bool>           # block until object is gone (default: false)
  interval: <duration>   # poll interval for the post-delete wait  (default: 2s)
  timeout: <duration>    # hard deadline for the post-delete wait  (default: 2m)
  retry:
    attempts: <int>
    backoff: <duration>
```

### Template system

Templates are Go templates with [Sprig](https://masterminds.github.io/sprig/)
helper functions. They are loaded from `templates/` at test start and rendered
per step with the values defined in the case file.

The object name is injected automatically from the `objects` map key — never
put `name` in step values. Use Sprig's `default` for every optional spec field
so templates render safely when only the name is provided (e.g. for Wait or
Delete steps that carry no values).

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .name }}
spec:
  replicas: {{ .replicas | default 1 }}
  selector:
    matchLabels:
      app: {{ .name }}
  template:
    metadata:
      labels:
        app: {{ .name }}
    spec:
      containers:
        - name: app
          image: {{ .image | default "nginx:1.25-alpine" }}
```

## Examples

See [`examples/tests/`](examples/tests/) for three working suites:

| Suite        | Demonstrates                                  |
|--------------|-----------------------------------------------|
| `nginx`      | Ensure + Wait with readiness condition         |
| `configmap`  | Ensure + Patch                                 |
| `job`        | Ensure + Wait for completion                   |

Reference templates with every field documented are in
[`examples/templates/`](examples/templates/).

## Project structure

```
cmd/kube2e/           CLI entry point
pkg/command/          Cobra commands and flag wiring
pkg/engine/           Public RunTests entry point
internal/engine/      Test execution engine (test → case → step → action)
internal/template/    Go template loading and rendering
internal/kube/        Kubernetes client (SSA, Wait, Filter)
internal/tools/       filter, logs, patch, safe, workerpool
internal/errors/      Sentinel errors
internal/version/     Build-time version info
```
