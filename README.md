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

kube2e is a command-line tool for end-to-end testing of Kubernetes controllers,
operators, Helm charts, and GitOps delivery. You describe tests as declarative
YAML and run them against a live cluster — no Go code required.

kube2e validates resources and workflows against an **existing** cluster. CRDs
and operators under test are expected to be installed beforehand; kube2e does not
manage their lifecycle.

## Table of contents

- [Overview](#overview)
- [Features](#features)
- [Quickstart](#quickstart)
- [Example](#example)
- [Installation](#installation)
- [Building](#building)
- [Usage](#usage)
- [Suite layout](#suite-layout)
- [Documentation](#documentation)
- [Examples](#examples)
- [Project structure](#project-structure)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Overview

A run discovers test suites under a working directory and executes them against
the configured cluster. Tests follow a four-level hierarchy:

```
Test (suite)   a directory with cases/ and an optional templates/
  Case         one YAML file: a namespace, objects, hooks, and ordered steps
    Step       a group of typed actions run in a fixed order
      Action   a single Kubernetes operation
```

Because a suite is just a directory, it can be packaged into an OCI image and run
unchanged on any cluster — locally, in CI, or in staging — without cloning a
repository:

```
test suite on disk
        │  kube2e tests publish
        ▼
ghcr.io/example/tests:v1     (stored in any OCI registry)
        │  kube2e run --remote
        ▼
executed on any cluster
```

This lets test suites ship alongside the product they validate, and lets you run
vendor- or third-party-provided suites directly from a registry.

## Features

- Declarative YAML test suites — no Go code to write or compile.
- Seven actions per step, run in a fixed order
  (`ensure` → `patch` → `wait` → `assert` → `logs` → `exec` → `delete`).
- Go templates with [Sprig](https://masterminds.github.io/sprig/) helpers for
  object manifests.
- `beforeEach` / `afterEach` hooks at the case level.
- Target pods for `logs` and `exec` by templated object or by
  `kind` + `labelSelector`.
- Server-Side Apply with automatic, deterministic per-case cleanup.
- Tag filtering, parallel suite execution, and `--dry-run` validation.
- OCI packaging (`tests publish`) and remote execution (`run --remote`).
- Machine-readable YAML reports and structured JSON logs for CI.

## Quickstart

Install the CLI:

```bash
go install github.com/ipaqsa/kube2e/cmd/kube2e@latest
```

Clone the repository and validate the bundled examples without touching a
cluster:

```bash
git clone https://github.com/ipaqsa/kube2e.git
cd kube2e
kube2e run ./examples --dry-run
```

Run only the cases tagged `smoke`:

```bash
kube2e run ./examples --dry-run --tags smoke
```

Run against a real cluster:

```bash
kube2e run ./examples --kubeconfig ~/.kube/config
```

Scaffold a new suite of your own:

```bash
kube2e tests add smoke
kube2e run . --dry-run
```

## Example

A suite is a directory with an optional `templates/` and a `cases/` directory:

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

Validate it without cluster writes, then run it:

```bash
kube2e run ./tests --dry-run
kube2e run ./tests --kubeconfig ~/.kube/config
```

## Installation

### go install

```bash
go install github.com/ipaqsa/kube2e/cmd/kube2e@latest
```

### Container image

```bash
docker pull ghcr.io/ipaqsa/kube2e:latest
docker run --rm ghcr.io/ipaqsa/kube2e:latest version
```

The image contains the `kube2e` binary only; mount your suites and kubeconfig to
run them.

### Prebuilt binary

Download the binary for your platform from the latest release. Builds are
published for `linux-amd64`, `linux-arm64`, and `darwin-arm64`, each with its own
`.sha256` checksum:

```bash
# Choose your platform: linux-amd64, linux-arm64, or darwin-arm64
PLATFORM=linux-amd64
BASE=https://github.com/ipaqsa/kube2e/releases/latest/download

curl -fsSL -O "$BASE/kube2e-$PLATFORM"
curl -fsSL -O "$BASE/kube2e-$PLATFORM.sha256"
sha256sum -c "kube2e-$PLATFORM.sha256"        # macOS: shasum -a 256 -c

chmod +x "kube2e-$PLATFORM"
sudo mv "kube2e-$PLATFORM" /usr/local/bin/kube2e
```

### From source

```bash
git clone https://github.com/ipaqsa/kube2e.git
cd kube2e
make install
```

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
| `--kubeconfig`      | `KUBE2E_KUBECONFIG`      | —        | Kubeconfig path; falls back to `$KUBECONFIG` then `~/.kube/config`, then in-cluster |
| `--tags`            | `KUBE2E_TAGS`            | all      | Comma-separated tags; only matching cases run      |
| `-n, --parallel`    | `KUBE2E_PARALLEL`        | 1        | Number of suites to run concurrently               |
| `--remote`          | `KUBE2E_REMOTE`          | —        | OCI image that contains test suites                |
| `--remote-user`     | `KUBE2E_REMOTE_USER`     | —        | Registry username for `--remote`                   |
| `--remote-password` | `KUBE2E_REMOTE_PASSWORD` | —        | Registry password for `--remote`                   |
| `--dry-run`         | `KUBE2E_DRY_RUN`         | false    | Parse, validate, and render without applying anything |
| `--report-file`     | `KUBE2E_REPORT_FILE`     | —        | Write a YAML execution report after the run        |
| `--log-format`      | `KUBE2E_LOG_FORMAT`      | `text`   | Log format: `text` (colored) or `json`             |
| `-v, --verbose`     | `KUBE2E_VERBOSE`         | false    | Include `debug` and `warn` messages (default: `info` + `error`) |

```bash
# Run all suites under ./examples
kube2e run ./examples

# Run only cases tagged "smoke" or "job"
kube2e run ./examples --tags smoke,job

# Run 4 suites in parallel
kube2e run ./examples -n 4

# Run suites packaged in an OCI image
kube2e run . --remote ghcr.io/example/kube2e-tests:v0.1.0

# Validate without touching the cluster
kube2e run ./examples --dry-run

# Save a YAML report and emit JSON logs (useful in CI)
kube2e run ./examples --report-file report.yaml --log-format json
```

Suites run independently: a failing suite does not abort the others, and the
process exits non-zero if any case fails. When `--tags` is unset, every case
runs; otherwise a case runs only if it carries at least one matching tag.

`<dir>` is required in both local and remote modes. With `--remote`, kube2e pulls
the image, extracts its filesystem to a temporary directory, and discovers suites
from `<dir>` within it — use `.` for the image root. Private registries accept
`--remote-user` / `--remote-password`; when the username is omitted, the default
Docker credential keychain is used.

The `--report-file` report contains aggregate totals and nested test, case, step,
hook, and action results with their state and failure reason. For remote runs it
records the image reference and registry username, but never the password.

### Run with Docker

The published image holds only the `kube2e` binary (entrypoint `kube2e`), so the
suites and kubeconfig must be mounted in. Validate suites without a cluster:

```bash
docker run --rm \
  -v "$PWD/examples:/work:ro" \
  ghcr.io/ipaqsa/kube2e:latest run /work --dry-run
```

Run against a cluster by also mounting your kubeconfig and pointing `--kubeconfig`
at it:

```bash
docker run --rm \
  -v "$PWD/examples:/work:ro" \
  -v "$HOME/.kube/config:/kubeconfig:ro" \
  ghcr.io/ipaqsa/kube2e:latest run /work --kubeconfig /kubeconfig
```

To run suites packaged in an image, no mount is needed — point `--remote` at the
suite image and use `.` for its root:

```bash
docker run --rm \
  -v "$HOME/.kube/config:/kubeconfig:ro" \
  ghcr.io/ipaqsa/kube2e:latest \
  run . --remote ghcr.io/example/kube2e-tests:v0.1.0 --kubeconfig /kubeconfig
```

> **Networking:** the container must reach the cluster's API server. For a
> cluster whose kubeconfig points at `127.0.0.1` (kind, minikube, Docker
> Desktop), add `--network host` (Linux) or rewrite the server address to
> `host.docker.internal`.

### Scaffold a new suite

```bash
kube2e tests add <name> [flags]
```

| Flag        | Default | Description                             |
|-------------|---------|-----------------------------------------|
| `-C, --dir` | `.`     | Parent directory to create the suite in |

Creates `<name>/cases/` and `<name>/templates/` with a starter ConfigMap template
and a starter case. The case's optional fields (tags, namespace, the other
actions, hooks, retry, delay, timeout) are written as comments — uncomment what
you need. The uncommented fields form a minimal, runnable case.

```bash
# Create ./nginx with a starter case and template
kube2e tests add nginx

# Create the suite under ./examples
kube2e tests add nginx --dir ./examples

# Validate it — pass the parent directory, not the suite itself
kube2e run . --dry-run
```

### Publish a suite image

```bash
kube2e tests publish <dir> --remote <image> [flags]
```

| Flag                | Env var                                | Default | Description                      |
|---------------------|----------------------------------------|---------|----------------------------------|
| `--remote`          | `KUBE2E_TESTS_PUBLISH_REMOTE`          | —       | Image reference to push          |
| `--remote-user`     | `KUBE2E_TESTS_PUBLISH_REMOTE_USER`     | —       | Registry username for `--remote` |
| `--remote-password` | `KUBE2E_TESTS_PUBLISH_REMOTE_PASSWORD` | —       | Registry password for `--remote` |
| `-v, --verbose`     | `KUBE2E_VERBOSE`                       | false   | Include `debug` and `warn` messages |

```bash
# Publish every suite under ./examples
kube2e tests publish ./examples --remote ghcr.io/example/kube2e-tests:v0.1.0

# Publish with explicit registry credentials
kube2e tests publish ./examples \
  --remote ghcr.io/example/kube2e-tests:v0.1.0 \
  --remote-user "$USER" \
  --remote-password "$TOKEN"
```

Only immediate child directories that contain a `cases/` subdirectory are
included, and they are written at the image root. An image built from
`./examples` is run with:

```bash
kube2e run . --remote ghcr.io/example/kube2e-tests:v0.1.0
```

## Suite layout

```
<work-dir>/
└── <suite-name>/          # directory name becomes the suite name
    ├── templates/         # optional — Go templates rendered into objects
    │   └── *.yaml
    └── cases/             # one YAML file per test case
        └── *.yaml
```

There is no suite descriptor file — the suite name is the directory name.
Templates are optional and shared by every case in the suite. Cases execute in
alphabetical filename order, and all resources applied during a case are deleted
when it finishes.

A case's `namespace`, if set, is created when absent but **never deleted** —
kube2e will not remove a namespace it may not own (such as a pre-existing user
namespace). See [Test suites & case files](docs/suites.md) for the full contract.

## Documentation

Reference documentation for authoring suites lives in [`docs/`](docs/):

- [Test suites & case files](docs/suites.md) — suite layout, the case file
  contract, hooks, and `objects` binding.
- [Steps](docs/steps.md) — step structure, the fixed action order, and the
  `delay`, `retry`, and `optional` fields.
- [Actions](docs/actions.md) — `ensure`, `patch`, `wait`, `assert`, `logs`,
  `exec`, and `delete`.
- [Templates](docs/templates.md) — Go templates, Sprig helpers, and automatic
  name injection.
- [Server-Side Apply & cleanup](docs/server-side-apply.md) — field manager,
  conflicts, and per-case resource cleanup.

The fully annotated case file is [`case.yaml`](case.yaml).

## Examples

The [`examples/`](examples/) directory contains four runnable suites. All
validate with `--dry-run` (no cluster required) and run end-to-end against a
cluster.

| Suite       | Cases                          | Demonstrates                                                        |
|-------------|--------------------------------|--------------------------------------------------------------------|
| `configmap` | `labels`, `lifecycle`, `patch-ops` | `ensure`, `assert`, and the full set of RFC 6902 `patch` operations |
| `job`       | `complete`, `report`           | `beforeEach` / `afterEach` hooks, `optional` steps, multi-step cases |
| `nginx`     | `rollout`, `scale`, `selector` | rollout `wait`, replica `patch`, and `logs` / `exec` by object or label selector |
| `pod`       | `output`, `probe`, `silent`    | `logs` match policies (`any` / `none`), `exec`, and `delete` with wait |
| `webapp`    | `deploy`                       | a multi-object app (ConfigMap + Deployment + Service): `match: all` logs, both selector forms, and a retried `assert` |

## Project structure

```
cmd/kube2e/           CLI entry point
pkg/command/          Cobra commands and flag wiring
pkg/engine/           Public RunTests entry point
internal/engine/      Test execution engine (test → case → step → action)
internal/template/    Go template loading and rendering
internal/kube/        Kubernetes client (SSA, wait, logs, exec)
internal/image/       OCI image build and pull
internal/scaffold/    Starter suite generation (tests add)
internal/tools/       filter, logs, patch, safe, workerpool
internal/errors/      Sentinel errors
internal/version/     Build-time version info
examples/             Runnable test suites
```

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for local development, testing, style,
DCO, and pull request guidance. New behavior should be covered with black-box
tests where practical.

## Security

Do not open public issues for vulnerabilities or credential leaks. Report
security issues privately to the maintainers with the affected command path,
required Kubernetes permissions, and reproduction details.

## License

kube2e is released under the [MIT License](LICENSE).
