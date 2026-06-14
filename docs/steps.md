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

> **Note:** `ensure` takes `object` directly (flat). Every other action nests it
> under `target`.

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

For the full per-action field reference, see [Actions](actions.md).
