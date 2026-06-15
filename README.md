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

## Table of contents

- [Why kube2e?](#why-kube2e)
- [Quickstart](#quickstart-)
- [Minimal example](#minimal-example-)
- [When to use kube2e](#when-to-use-kube2e-)
- [When not to use kube2e](#when-not-to-use-kube2e-)
- [Install](#install-)
- [Building](#building)
- [Usage](#usage)
- [Directory structure](#directory-structure)
- [Documentation](#documentation)
- [Examples](#examples-)
- [Project structure](#project-structure)
- [Contributing](#contributing-)
- [Security](#security-)
- [License](#license)

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
- Target pods for `logs` and `exec` by templated object or by `kind` +
  `labelSelector` (handy for resources created outside the suite).
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

### Scaffold a new suite

```bash
kube2e tests add <name> [flags]
```

| Flag           | Default | Description                              |
|----------------|---------|------------------------------------------|
| `-C, --dir`    | `.`     | Parent directory to create the suite in  |

Creates `<name>/cases/` and `<name>/templates/` with a starter ConfigMap
template and a starter case. The case's optional fields (tags, namespace,
patch/wait/logs/exec/delete actions, hooks, retry, delay, timeout) are written
as comments — uncomment the ones you need. The uncommented fields form a
minimal, runnable case.

```bash
# Create ./nginx with a starter case and template
kube2e tests add nginx

# Create the suite under ./examples
kube2e tests add nginx --dir ./examples

# Validate the scaffolded suite without touching a cluster.
# Pass the parent directory that contains the suite, not the suite itself.
kube2e run . --dry-run
```

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
case are deleted once it finishes. The case's `namespace`, if one was specified,
is created when absent but **never deleted** — kube2e will not remove a namespace
it may not own (e.g. a pre-existing user namespace). Clean it up yourself if
needed.

See [Test suites & case files](docs/suites.md) for the full case file contract.

## Documentation

Reference documentation for authoring suites lives in [`docs/`](docs/):

- [Test suites & case files](docs/suites.md) — suite layout, the case file
  contract, hooks, and `objects` binding
- [Steps](docs/steps.md) — step structure, the fixed action order, and the
  `delay`, `retry`, and `optional` fields
- [Actions](docs/actions.md) — `ensure`, `patch`, `wait`, `assert`, `logs`,
  `exec`, and `delete`
- [Templates](docs/templates.md) — Go templates, Sprig helpers, and automatic
  name injection
- [Server-Side Apply & cleanup](docs/server-side-apply.md) — field manager,
  conflicts, and per-case resource cleanup

The full annotated case file is [`case.yaml`](case.yaml).

## Examples 🧪

See [`examples/`](examples/) for four working suites:

| Suite       | Cases                      | Demonstrates                                      | Tags                                |
|-------------|----------------------------|---------------------------------------------------|-------------------------------------|
| `configmap` | `lifecycle`, `labels`      | ensure, assert, patch + assert, multi-case suites | `smoke`, `configmap`, `patch`, `labels` |
| `nginx`     | `rollout`, `scale`         | ensure, wait, assert replicas, logs (Deployment), exec (config check), patch + scale | `smoke`, `deployment`, `wait` |
| `job`       | `complete`, `report`       | beforeEach / afterEach hooks, multi-step cases    | `smoke`, `job`, `hooks`, `cleanup`  |
| `pod`       | `output`, `silent`, `probe` | logs `match: any`, logs `match: none`, exec into a running pod | `smoke`, `pod`, `logs`, `exec` |

## Project structure

```
cmd/kube2e/           CLI entry point
pkg/command/          Cobra commands and flag wiring
pkg/engine/           Public RunTests entry point
internal/engine/      Test execution engine (test → case → step → action)
internal/template/    Go template loading and rendering
internal/kube/        Kubernetes client (SSA, Wait, Logs, Exec)
internal/image/       OCI image build and pull
internal/tools/       filter, logs, patch, safe, workerpool
internal/errors/      Sentinel errors
internal/version/     Build-time version info
examples/             Working test suites (run with --dry-run, no cluster needed)
  configmap/          ensure, assert, patch
  nginx/              wait, assert, logs, exec (Deployment)
  job/                beforeEach/afterEach hooks
  pod/                logs match policies, exec (Pod)
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
