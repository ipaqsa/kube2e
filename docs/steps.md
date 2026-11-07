# Steps

Each step runs one or more typed [actions](actions.md). Set any combination of
actions; absent fields are skipped. Actions within a step execute in a **fixed
order**:

```
ensure → patch → wait → assert → logs → exec → delete
```

The first failure aborts the step — unless the step is marked `optional: true`,
in which case the failure is logged as a warning and skipped.

## Step reference

```yaml
name: <string>           # required — shown in log output
description: <string>
optional: <bool>         # when true, failures are warned and skipped
ensure:                  # optional — create or update the object
  object: <string>       # required (flat, no target nesting)
  ...
patch:                   # optional — apply JSON patches to the live object
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
    object: <string>     # a templated Pod/Deployment/ReplicaSet/StatefulSet …
    # — or — select existing pods by label (no template):
    kind: <string>       # Pod, Deployment, ReplicaSet, or StatefulSet
    labelSelector: <string>
  ...
exec:                    # optional — run a command inside a pod
  target:
    object: <string>     # … or kind + labelSelector, as with logs
  ...
delete:                  # optional — remove the object
  target:
    object: <string>     # required
  ...
```

> **Note:** `ensure` takes `object` directly (flat). Every other action nests it
> under `target`. Only `logs` and `exec` accept `kind` + `labelSelector` as an
> alternative to a templated `object`; the two forms are mutually exclusive.

## `delay`

Every action accepts an optional `delay: <duration>` field that sleeps before
the action executes.

## `retry`

Every action **except `wait` and `logs`** accepts an optional `retry` block:

```yaml
retry:
  attempts: <int>         # total executions (1 = no retry)
  backoff: <duration>     # sleep between attempts (e.g. 5s)
```

`wait` and `logs` poll on their own schedule — tune their `interval` and
`timeout` fields instead of using `retry`.

## `timeout`

Every action accepts an optional `timeout: <duration>`. It bounds the **whole
action**: every `retry` attempt, the `backoff` between attempts, and kube2e's
internal retries on transient API errors all share one deadline. `delay` runs
before the action and does not count against it.

| Action | Default | Notes |
|--------|---------|-------|
| `ensure`, `patch`, `assert` | none | No deadline unless set. |
| `wait` | `2m` | Polling stops at the deadline. |
| `exec` | `30s` | Covers all attempts together, not one command. |
| `delete` | `2m` with `wait: true`, none otherwise | Covers the delete retries and the post-delete wait. |
| `logs` | `2m` | `any`/`all`: deadline. `none`: observation window — see [`logs`](actions.md#logs). |

When an action's timeout expires, the action fails with `action timeout <d>
exceeded: context deadline exceeded …`. That fails the step (or is skipped if
the step is `optional`) and therefore the case. Cleanup still runs and the
**next case starts as usual**.

## Cancellation

When the run itself is canceled (Ctrl-C) or its own deadline expires, the
in-flight action stops immediately: a `backoff` or `delay` sleep ends at once
and no further attempt starts. Cleanup of the current case still runs on a
separate context bounded to 30s, then the suite stops — the remaining cases are
not run. An `optional` step does not swallow a cancellation.

For the full per-action field reference, see [Actions](actions.md).
