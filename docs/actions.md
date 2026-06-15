# Actions

Actions are the individual Kubernetes operations inside a [step](steps.md). A
step may set any combination of them, and they always run in this fixed order:

```
ensure → patch → wait → assert → logs → exec → delete
```

The first failure aborts the step unless the step is `optional: true`. See
[Steps](steps.md) for the shared `delay` and `retry` fields.

## `ensure`

Create or update the object using Server-Side Apply (field manager `kube2e`).
Requires Kubernetes v1.22+. The object is cached for automatic cleanup. See
[Server-Side Apply & cleanup](server-side-apply.md).

```yaml
ensure:
  object: <string>     # required — key in the case objects map (flat, no target)
  values:              # template render inputs; name is injected automatically
    <key>: <value>
  retry:
    attempts: <int>
    backoff: <duration>
```

## `patch`

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

## `wait`

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

## `assert`

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

## `logs`

Poll object logs until they contain the expected string, or until the timeout
expires. Does not support retry — tune `interval` and `timeout` instead.

Supported object kinds: **Pod**, **Deployment**, **ReplicaSet**, **StatefulSet**.
For workload types, a running pod is resolved via `spec.selector.matchLabels` on
each poll tick; if no pod is ready yet, the tick is silently skipped.

The target is specified one of two ways (mutually exclusive):

- `object` — a key in the case `objects` map (a templated Pod or workload).
- `kind` + `labelSelector` — select existing pods (or a workload's pods) by
  label, without a template. Useful for resources created outside the suite
  (operators, Helm). For workload kinds, the first match's pods are used.

```yaml
logs:
  target:
    object: <string>          # key in the case objects map …
    # — or —
    kind: <Pod|Deployment|ReplicaSet|StatefulSet>
    labelSelector: <string>   # e.g. "app=nginx"
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

Returns `ErrLogsNotContain` when the timeout elapses without the condition being
met.

## `exec`

Run a command inside the resolved pod. Succeeds when the command exits with
code zero; any non-zero exit or transport error is treated as a failure (and
retried if `retry` is configured).

Supported object kinds: **Pod**, **Deployment**, **ReplicaSet**, **StatefulSet**.
For workload types, the first Running pod is resolved via
`spec.selector.matchLabels`.

Like `logs`, the target is either a templated `object` or a `kind` +
`labelSelector` pair selecting an existing pod (the first Running match):

```yaml
exec:
  target:
    object: <string>          # key in the case objects map …
    # — or —
    kind: <Pod|Deployment|ReplicaSet|StatefulSet>
    labelSelector: <string>   # e.g. "app=nginx"
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

Use `["sh", "-c", "..."]` to run shell expressions. On failure, the command's
stderr is included in the returned error.

## `delete`

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

Resources created with `ensure` are cleaned up automatically after each case;
use `delete` only when a case needs to assert deletion behavior mid-run. See
[Server-Side Apply & cleanup](server-side-apply.md).
