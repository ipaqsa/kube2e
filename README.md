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
- Kubernetes actions for apply, delete, wait, patch, and value extraction

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
| `--tests`           | `KUBE2E_TESTS`           | all     | Comma-separated suite names to run                 |
| `--remote`          | `KUBE2E_REMOTE`          | —       | Container image that contains test suites          |
| `--remote-user`     | `KUBE2E_REMOTE_USER`     | —       | Registry username for `--remote`                   |
| `--remote-password` | `KUBE2E_REMOTE_PASSWORD` | —       | Registry password for `--remote`                   |
| `-v, --verbose`     | `KUBE2E_VERBOSE`         | false   | Show `warn`-level messages (default: `info`+`error` only) |

```bash
# Run all suites under ./examples/tests
kube2e run ./examples/tests

# Run only the nginx and job suites
kube2e run ./examples/tests --tests nginx,job

# Run with warning messages visible
kube2e run ./examples/tests -v

# Run test suites packaged in a container image
kube2e run ./tests --remote ghcr.io/example/kube2e-tests:v0.1.0

# Run test suites from the root of a container image
kube2e run . --remote ghcr.io/example/kube2e-tests:v0.1.0

# Run a specific suite against a staging cluster
kube2e run ./examples/tests --tests nginx -v --kubeconfig ~/.kube/staging.yaml
```

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
    ├── test.yaml        # Suite descriptor (name, namespace, labels, ignored cases)
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
labels:
  <key>: <value>         # merged onto every applied object
annotations:
  <key>: <value>
ignored:                 # case basenames (without .yaml) to skip
  - <string>
```

### Case file (`cases/<name>.yaml`)

```yaml
name: <string>           # required
description: <string>
labels:
  <key>: <value>         # additive over test-level labels
annotations:
  <key>: <value>
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

Each step references one object from the case `objects` map and runs one or
more actions against it.

```yaml
name: <string>           # required — shown in log output
description: <string>
optional: <bool>         # when true, failures are warned and skipped
labels:
  <key>: <value>         # applied to the object for this step only
annotations:
  <key>: <value>
object: <string>         # key in the case objects map (= Kubernetes object name)
actions:
  - <action>
```

### Actions

All actions operate on the object identified by the step's `object` key.
Every action accepts an optional `delay: <duration>` field that sleeps before
execution. Only `Ensure` uses the step's `values` to build the full rendered
spec; the other actions use the object's name and GVK to operate on live
cluster state.

#### `Ensure`

Create or update the object using Server-Side Apply (field manager `kube2e`).
Requires Kubernetes v1.22+. The object is cached for automatic cleanup.

```yaml
- command: Ensure
  values:              # template render inputs; name is injected automatically
    <key>: <value>
```

#### `Delete`

Remove the object. Not-found responses are treated as success.

```yaml
- command: Delete
```

#### `Wait`

Poll the live object until all JQ conditions return `true`, or until the object
disappears (`deletion: true`), or until the timeout expires.

```yaml
- command: Wait
  conditions:
    - <jq expression>    # must evaluate to boolean — all must pass
  deletion: <bool>       # succeed when object no longer exists
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

#### `Patch`

Apply RFC 6902 JSON Patch operations to the rendered object, then re-ensure it.

```yaml
- command: Patch
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
```

#### `Value`

Fetch the current object from the cluster, evaluate a JQ expression against it,
and store the string result for use in subsequent step templates.

```yaml
- command: Value
  valueKey: <string>     # key to store the result under
  valuePath: <jq>        # must evaluate to a string
```

Stored values are available in templates as `{{ .Values.<key> }}`.

```yaml
# Examples
valuePath: .spec.clusterIP
valuePath: .data.version
valuePath: .spec.containers[0].image | split(":")[1]
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
| `configmap`  | Ensure + Value extraction + Patch              |
| `job`        | Ensure + Wait for completion + ignored case    |

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
