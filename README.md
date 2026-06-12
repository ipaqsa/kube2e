# kube2e

<p align="center">
  <img src="./icon.png" alt="kube2e icon" width="160">
</p>

[![CI](https://github.com/ipaqsa/kube2e/actions/workflows/ci.yml/badge.svg)](https://github.com/ipaqsa/kube2e/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/ipaqsa/kube2e/main/.github/badges/coverage.json)](https://github.com/ipaqsa/kube2e/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ipaqsa/kube2e.svg)](https://pkg.go.dev/github.com/ipaqsa/kube2e)
[![Go Report Card](https://goreportcard.com/badge/github.com/ipaqsa/kube2e)](https://goreportcard.com/report/github.com/ipaqsa/kube2e)
[![License](https://img.shields.io/github/license/ipaqsa/kube2e)](LICENSE)
[![Image](https://img.shields.io/badge/image-ghcr.io%2Fipaqsa%2Fkube2e-blue)](https://github.com/ipaqsa/kube2e/pkgs/container/kube2e)

YAML-driven end-to-end testing for Kubernetes controllers, operators, charts,
and GitOps delivery workflows. Write declarative test suites, package them into
an OCI image alongside your product, and run the same image against any cluster —
local, CI, staging, or production.

kube2e validates resources and workflows against an existing cluster.
CRDs and operators are expected to be installed before test execution.

## Why kube2e?

**OCI-native test distribution** is the core advantage. Test suites are regular
directories that can be published to any container registry and executed directly
from the image on any cluster:

```
test suite on disk
        │
        ▼
kube2e tests publish
        │
        ▼
ghcr.io/example/tests:v1        ← stored in any OCI registry
        │
        ▼
kube2e run --remote             ← run on any cluster, anywhere
```

This means:

- Tests ship with the product, not as a separate artifact.
- CI pulls the same image that was validated in dev — no "works on my machine" drift.
- Any environment with `kube2e` and a kubeconfig can run the suite without
  cloning a repository.
- You can also run suites from an OCI image you didn't build — useful for
  running vendor-provided or third-party test suites against your cluster.

Beyond image distribution:

- Write tests as declarative YAML with no Go code required.
- Keep suites simple: one directory with `cases/` and optional `templates/`.
- Render Kubernetes objects with Go templates and Sprig helpers.
- Exercise real cluster behavior with `ensure`, `patch`, `wait`, `assert`,
  `logs`, `exec`, and `delete` actions.
- Use Server-Side Apply with deterministic per-case cleanup.
- Filter cases with tags and run suites in parallel.
- Validate suites with `--dry-run` before touching a cluster.

## Quickstart 🚀

Install the CLI:

```bash
go install github.com/ipaqsa/kube2e/cmd/kube2e@latest
```

Run the bundled examples directly from the published image — no clone needed:

```bash
kube2e run . --remote ghcr.io/ipaqsa/kube2e:latest --dry-run
```

Or clone and run locally:

```bash
git clone https://github.com/ipaqsa/kube2e.git
cd kube2e
kube2e run ./examples --dry-run
```

Run only smoke-tagged cases:

```bash
kube2e run ./examples --dry-run --tags smoke
```

Run against a real cluster:

```bash
kube2e run ./examples --kubeconfig ~/.kube/config
```

## Minimal example 🧩

Create a suite with one template and one case:

```text
tests/
└── configmap/
    ├── templates/
    │   └── configmap.yaml
    └── cases/
        └── lifecycle.yaml
```

`tests/configmap/templates/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .name }}
data:
  env: {{ .env | default "development" | quote }}
```

`tests/configmap/cases/lifecycle.yaml`:

```yaml
version: v1
name: lifecycle
tags:
  - smoke
namespace: kube2e-configmap

objects:
  app-config: configmap

steps:
  - name: create-and-check
    ensure:
      object: app-config
      values:
        env: production
    assert:
      target:
        object: app-config
      conditions:
        - .data.env == "production"
```

Validate it without cluster writes:

```bash
kube2e run ./tests --dry-run
```

Run it against a cluster:

```bash
kube2e run ./tests --kubeconfig ~/.kube/config
```

## When to use kube2e ✅

- Smoke-test controllers and operators against a real Kubernetes API server.
- Validate rendered Kubernetes resources from charts or GitOps pipelines.
- Assert rollout, status, cleanup, and patch behavior with real cluster state.
- Ship test suites as OCI images so any environment can run them with a single
  `kube2e run --remote` command.
- Run vendor-provided or third-party test suites from an OCI registry without
  cloning a repository.

## When not to use kube2e ⚠️

- CRD installation, upgrades, and conversion testing; install CRDs before the
  suite runs.
- Load, soak, or performance testing.
- Replacing unit tests, controller-runtime `envtest`, or package-level Go tests.
- Hiding missing RBAC, broken cluster setup, or unavailable dependencies.

## Install 📦

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

| Flag                | Env var                  | Default  | Description                                        |
|---------------------|--------------------------|----------|----------------------------------------------------|
| `--kubeconfig`      | `KUBE2E_KUBECONFIG`      | —        | Kubeconfig path; falls back to `$KUBECONFIG` then `~/.kube/config` |
| `--tags`            | `KUBE2E_TAGS`            | all      | Comma-separated tags; only matching cases run      |
| `-n, --parallel`    | `KUBE2E_PARALLEL`        | 1        | Number of test suites to run concurrently          |
| `--remote`          | `KUBE2E_REMOTE`          | —        | Container image that contains test suites          |
| `--remote-user`     | `KUBE2E_REMOTE_USER`     | —        | Registry username for `--remote`                   |
| `--remote-password` | `KUBE2E_REMOTE_PASSWORD` | —        | Registry password for `--remote`                   |
| `--dry-run`         | `KUBE2E_DRY_RUN`         | false    | Parse and validate tests without applying any resources |
| `--report-file`     | `KUBE2E_REPORT_FILE`     | —        | Write a YAML execution report after the run finishes |
| `--log-format`      | `KUBE2E_LOG_FORMAT`      | `text`   | Log output format: `text` (colored) or `json`      |
| `-v, --verbose`     | `KUBE2E_VERBOSE`         | false    | Show `debug` and `warn` messages (default: `info`+`error` only) |

```bash
# Run all suites under ./examples
kube2e run ./examples

# Run only cases tagged "smoke" or "job"
kube2e run ./examples --tags smoke,job

# Run 4 test suites in parallel
kube2e run ./examples -n 4

# Run with verbose output (debug + warn messages)
kube2e run ./examples -v

# Run test suites packaged in a container image
kube2e run ./tests --remote ghcr.io/example/kube2e-tests:v0.1.0

# Run tagged tests against a staging cluster
kube2e run ./examples --tags smoke --kubeconfig ~/.kube/staging.yaml

# Validate test files without touching the cluster
kube2e run ./examples --dry-run

# Save a YAML report after all suites finish
kube2e run ./examples --report-file report.yaml

# Emit structured JSON logs (useful in CI)
kube2e run ./examples --log-format json
```

**Tag filtering:** when `--tags` is not set, all cases run. When `--tags a,b`
is set, a case runs only if it has at least one matching tag.

**Report files:** when `--report-file <path>` is set, kube2e writes one YAML
report after all scheduled work finishes. The report contains aggregate totals
and nested test, case, step, hook, and action results with their state and
failure reason.
For remote runs, the report also includes the image reference and registry
username, but never includes the registry password.

`<dir>` is required in both local and remote modes. When `--remote` is set,
kube2e pulls the image, extracts its filesystem to a temporary directory, and
discovers suites from `<dir>` inside that extracted filesystem. Use `.` to
discover suites from the image root. Private registries can be accessed with
`--remote-user` and `--remote-password`; when the username is omitted, kube2e
uses the default Docker credential keychain.

### Publish a tests image

```bash
kube2e tests publish <dir> --remote <image> [flags]
```

| Flag                | Env var                                | Default | Description                              |
|---------------------|----------------------------------------|---------|------------------------------------------|
| `--remote`          | `KUBE2E_TESTS_PUBLISH_REMOTE`          | —       | Image reference to push                  |
| `--remote-user`     | `KUBE2E_TESTS_PUBLISH_REMOTE_USER`     | —       | Registry username for `--remote`         |
| `--remote-password` | `KUBE2E_TESTS_PUBLISH_REMOTE_PASSWORD` | —       | Registry password for `--remote`         |
| `-v, --verbose`     | `KUBE2E_VERBOSE`                       | false   | Show `debug` and `warn` messages         |

```bash
# Publish all test suites under ./examples
kube2e tests publish ./examples --remote ghcr.io/example/kube2e-tests:v0.1.0

# Publish with explicit registry credentials
kube2e tests publish ./examples \
  --remote ghcr.io/example/kube2e-tests:v0.1.0 \
  --remote-user "$USER" \
  --remote-password "$TOKEN"
```

Only immediate child directories that contain a `cases/` subdirectory are
included in the image. The suites are written at the image root, so an image
built from `./examples` can be run with:

```bash
kube2e run . --remote ghcr.io/example/kube2e-tests:v0.1.0
```

## Directory structure

```
<work-dir>/
└── <suite-name>/           # directory name becomes the suite name
    ├── templates/           # optional — Go templates rendered into Kubernetes objects
    │   └── *.yaml
    └── cases/              # one YAML file per test case
        └── *.yaml
```

No suite descriptor file is required. The suite name is taken from the directory
name. Templates are optional and shared across all cases in the same suite.
Cases execute in alphabetical filename order. All resources applied during a
case are deleted once it finishes, along with the case's namespace if one was
specified.

## Reference

### Case file (`cases/<name>.yaml`)

```yaml
version: v1              # required — must match the supported schema version
name: <string>           # required — shown in log output
description: <string>
tags:                    # optional — filter with --tags
  - <string>
namespace: <string>      # optional — created before the case, deleted after cleanup
objects:                 # resource name → template base-filename (without .yaml)
  <name>: <template>     # the key is injected as the Kubernetes object name
hooks:
  beforeEach:
    - <step>             # executed before each item in steps
  afterEach:
    - <step>             # executed after each item in steps; runs even on step failure
steps:
  - <step>
```

The `objects` map is the single place where you bind a resource name to its
template. Steps reference entries by key; the key is used as the Kubernetes
`metadata.name` in every render — you never put `name` in step values.

`hooks.beforeEach` and `hooks.afterEach` are optional case-level step lists.
They use the same schema as normal steps and run before or after every step in
`steps`. After hooks run even when a before hook or the main step fails.

### Step

Each step runs one or more typed actions. Set any combination; absent fields
are skipped. Actions execute in a fixed order:
**ensure → patch → wait → assert → logs → exec → delete**,
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
logs:                    # optional — poll logs until output contains a string
  target:
    object: <string>     # required — Pod, Deployment, ReplicaSet, or StatefulSet
  ...
exec:                    # optional — run a command inside a pod
  target:
    object: <string>     # required — Pod, Deployment, ReplicaSet, or StatefulSet
  ...
delete:                  # optional — remove the object
  target:
    object: <string>     # required
  ...
```

Every action also accepts an optional `delay: <duration>` field that sleeps
before execution. Every action except `wait` and `logs` accepts an optional `retry` block:

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

#### `logs`

Poll object logs until they contain the expected string, or until the timeout
expires. Does not support retry — tune `interval` and `timeout` instead.

Supported object kinds: **Pod**, **Deployment**, **ReplicaSet**, **StatefulSet**.
For workload types, a running pod is resolved via `spec.selector.matchLabels` on
each poll tick; if no pod is ready yet, the tick is silently skipped.

```yaml
logs:
  target:
    object: <string>    # required — key in the case objects map
  contains: <string>    # required — substring to search for in the log output
  container: <string>   # optional — container name; omit for single-container pods
  match: <any|all|none> # optional — match policy across pods (default: any)
  interval: <duration>  # poll interval  (default: 2s)
  timeout: <duration>   # hard deadline  (default: 2m)
```

Fresh logs (last 200 lines) are streamed on every tick. Transient errors (pod
not yet scheduled, container still initializing) are retried silently.

**Match policies:**
- `any` (default) — succeed when at least one pod's logs contain the string
- `all` — succeed when every pod's logs contain the string
- `none` — succeed when no pod's logs contain the string

Returns `ErrLogsNotContain` when the timeout elapses without the condition being met.

#### `exec`

Run a command inside the resolved pod. Succeeds when the command exits with
code zero; any non-zero exit or transport error is treated as a failure (and
retried if `retry` is configured).

Supported object kinds: **Pod**, **Deployment**, **ReplicaSet**, **StatefulSet**.
For workload types, the first Running pod is resolved via
`spec.selector.matchLabels`.

```yaml
exec:
  target:
    object: <string>     # required — key in the case objects map
  command:               # required — passed directly to the container; no shell wrapping
    - sh
    - -c
    - "nginx -t"
  container: <string>    # optional — container name; omit for single-container pods
  retry:
    attempts: <int>
    backoff: <duration>
  timeout: <duration>    # hard deadline for the command (default: 30s)
```

Use `["sh", "-c", "..."]` to run shell expressions. stdout and stderr are
captured and emitted at debug level.

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
helper functions. They are loaded from `templates/` at suite start and rendered
per step with the values defined in the case file. The `templates/` directory
is optional — cases that need no rendered objects can omit it entirely.

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

## Examples 🧪

See [`examples/`](examples/) for four working suites:

| Suite       | Cases                      | Demonstrates                                      | Tags                                |
|-------------|----------------------------|---------------------------------------------------|-------------------------------------|
| `configmap` | `lifecycle`, `labels`      | ensure, assert, patch + assert, multi-case suites | `smoke`, `configmap`, `patch`, `labels` |
| `nginx`     | `rollout`, `scale`         | ensure, wait, assert replicas, logs (Deployment), exec (config check), patch + scale | `smoke`, `deployment`, `wait` |
| `job`       | `complete`, `report`       | beforeEach / afterEach hooks, multi-step cases    | `smoke`, `job`, `hooks`, `cleanup`  |
| `pod`       | `output`, `silent`, `probe` | logs `match: any`, logs `match: none`, exec into a running pod | `smoke`, `pod`, `logs`, `exec` |

The full case file template with every field documented is
[`case.yaml`](case.yaml).

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

## Contributing 🤝

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for local development, testing, style,
DCO, and pull request guidance. New behavior should be covered with black-box
suite/case tests where practical.

## Security 🔒

Do not open public issues for vulnerabilities or credential leaks. Report
security issues privately to the maintainers with the affected command path,
required Kubernetes permissions, and reproduction details.

## License

`kube2e` is released under the [MIT License](LICENSE).
