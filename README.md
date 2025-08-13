# kube2e

kube2e is a command‑line tool for running end‑to‑end tests against a Kubernetes cluster. Tests are described declaratively in YAML and executed step by step using Kubernetes clients.

## Overview

* **Tests** live in directories that contain a `test.yaml` file describing metadata, optional CRDs, and a set of cases.
* **CRDs** from a test's `crds/` directory are ensured before any cases run.
* **Cases** provide `case.yaml`, template manifests, and ordered step files defining actions to perform.
* **Steps** run a sequence of actions such as ensuring, deleting, waiting for, or patching Kubernetes objects rendered from templates.
* **Cleanup** clears all resources applied during a case once it finishes.

## Building

This project uses Go modules. Build the CLI with:

```bash
go build -o kube2e ./cmd
```

## Usage

Build the binary and run tests against a cluster:

```bash
go build -o kube2e ./cmd
./kube2e test <path-to-tests> --kubeconfig <path-to-kubeconfig> \
  [--log info] [--test name] [--case name]
```

Flags:

* `--kubeconfig` – path to the kubeconfig file.
* `--log` – log level (`debug`, `info`, `warn`, `error`).
* `--test` – optional test directory to run.
* `--case` – optional case within the selected test.

### Examples

```bash
# Run every test in ./tests using your default kubeconfig
./kube2e test ./tests

# Run only the "network" test and its "smoke" case
./kube2e test ./tests --test network --case smoke
```

## Tests directory structure

A directory passed to `kube2e test` contains one or more test subdirectories:

```text
tests/
├── <test-name>/
│   ├── test.yaml          # Test metadata and defaults
│   ├── crds/              # Optional CRD manifests ensured before cases
│   └── cases/
│       └── <case-name>/
│           ├── case.yaml  # Case metadata
│           ├── templates/ # Go templates rendered for objects
│           └── steps/
│               ├── 001_<name>.yaml
│               └── 002_<name>.yaml
└── ...
```

Step files are prefixed with an increasing number so they execute in order. All
resources created during a case are deleted once that case finishes.

## Specification

### Test (`test.yaml`)

Defines metadata for a suite of cases and defaults applied to all objects.
Any CRDs in the sibling `crds/` directory are ensured before the cases run.

**Schema**

```yaml
name: <string>                     # required test name
description: <string>              # optional human description
namespace: <string>                # default namespace for objects
labels:                            # labels added to every object
  <key>: <value>
annotations:                       # annotations added to every object
  <key>: <value>
ignored:                           # case names to skip
  - <string>
```

**Example**

```yaml
name: sample-test
description: Demonstrate kube2e
namespace: demo
labels:
  suite: demo
ignored:
  - flaky-case
```

### Case (`case.yaml`)

Each case provides its own description and relies on templates and step files
within the case directory.

**Schema**

```yaml
name: <string>         # required case name
description: <string>  # human description
```

**Example**

```yaml
name: basic
description: Deploy nginx and verify rollout
```

### Step (`steps/NNN_<name>.yaml`)

Steps contain ordered actions. File names are prefixed numerically to determine
execution order.

**Schema**

```yaml
name: <string>
description: <string>
actions:
  - command: <Ensure|Delete|Wait|Patch>
    object:
      template: <string>            # file inside templates/
      values: {<key>: <value>}      # optional template values
    condition: <jq expression>      # Wait only
    deletion: <bool>                # Wait for object deletion
    interval: <duration>            # Wait polling interval
    timeout: <duration>             # Wait timeout
    patches:                        # Patch only – JSONPatch operations
      - op: <add|replace|remove|move|copy|test>
        path: <json-pointer>
        value: <any>
```

**Example**

```yaml
name: deploy and wait
actions:
  - command: Ensure
    object:
      template: deployment.yaml
      values:
        name: nginx
  - command: Wait
    object:
      template: deployment.yaml
      values:
        name: nginx
    condition: status.readyReplicas == 1
```

### Actions

Actions operate on rendered Kubernetes objects:

* `Ensure` – create or update the rendered object using Server‑Side Apply (SSA).
* `Delete` – remove the rendered object.
* `Wait` – watch the object until a JQ condition passes or it is deleted.
* `Patch` – apply JSON patches to the rendered object before ensuring it. Patch
  also uses SSA.

#### Server‑Side Apply considerations

`Ensure` and `Patch` rely on Kubernetes Server‑Side Apply and use the field
manager `kube2e`. SSA requires a cluster that supports the feature (Kubernetes
v1.22 or newer). If another controller already owns a field, the apply will
fail with a conflict and `kube2e` will stop. Forced applies are not currently
supported.

## Project structure

```text
cmd/                # CLI entry point
internal/command    # Cobra commands and flags
internal/engine     # Test execution engine (tests, cases, steps, actions)
internal/manager    # Helper managers for tests and templates
internal/service    # Kubernetes and CRD services
internal/tools      # Utility packages (filters, logging, patches)
```

See `LICENSE` for licensing information.
