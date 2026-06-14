# kube2e documentation

Reference documentation for writing and running kube2e test suites. For
installation, CLI flags, and the quickstart, see the
[project README](../README.md).

## Execution model

kube2e executes declarative YAML tests in a four-level hierarchy:

```
Test (suite)   → a directory with cases/ and optional templates/
  └─ Case       → one YAML file; owns a namespace, objects, hooks, and steps
       └─ Step   → an ordered group of typed actions
            └─ Action → a single Kubernetes operation
```

A run discovers every suite under the work directory, parses each case
(`version: v1`), renders templated objects, and executes steps sequentially.
All resources applied during a case are cleaned up after it finishes.

## Contents

- [Test suites & case files](suites.md) — suite layout, the case file contract,
  hooks, and `objects` binding
- [Steps](steps.md) — step structure, the fixed action order, and the `delay`,
  `retry`, and `optional` fields
- [Actions](actions.md) — full reference for `ensure`, `patch`, `wait`,
  `assert`, `logs`, `exec`, and `delete`
- [Templates](templates.md) — Go templates, Sprig helpers, and automatic name
  injection
- [Server-Side Apply & cleanup](server-side-apply.md) — field manager,
  conflicts, and per-case resource cleanup

The full annotated case file is [`case.yaml`](../case.yaml).
