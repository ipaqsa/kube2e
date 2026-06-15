# Test suites & case files

A **test suite** is a directory containing a `cases/` directory and an optional
`templates/` directory. The directory name becomes the suite name — there is no
suite descriptor file.

## Directory layout

```
<work-dir>/
└── <suite-name>/           # directory name becomes the suite name
    ├── templates/          # optional — Go templates rendered into Kubernetes objects
    │   └── *.yaml
    └── cases/              # one YAML file per test case
        └── *.yaml
```

Templates are optional and shared across all cases in the same suite. Cases
execute in **alphabetical filename order**. All resources applied during a case
are deleted once it finishes. The case's `namespace`, if one was specified, is
created when absent but **never deleted** — kube2e will not remove a namespace it
may not own (e.g. a pre-existing user namespace).

## Case file contract

- `version: v1` is required and validated at parse time.
- The suite name is the directory name (no `test.yaml` descriptor).
- `namespace` is per-case, not per-suite.
- The `objects` map binds a resource key to a template filename (without
  `.yaml`).
- Templates are loaded from `<suite-dir>/templates/`; the directory is optional.
- Cases execute in alphabetical filename order.

## Case file reference (`cases/<name>.yaml`)

```yaml
version: v1              # required — must match the supported schema version
name: <string>           # required — shown in log output
description: <string>
tags:                    # optional — filter with --tags
  - <string>
namespace: <string>      # optional — created before the case if absent; never deleted by kube2e
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

See [Steps](steps.md) for the step schema and [Actions](actions.md) for the
per-action fields.
