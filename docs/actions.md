# Actions

Actions are the individual Kubernetes operations inside a [step](steps.md). A
step may set any combination of them, and they always run in this fixed order:

```
ensure → patch → wait → assert → logs → exec → delete
```

The first failure aborts the step unless the step is `optional: true`. See
[Steps](steps.md) for the shared `delay`, `retry`, and `timeout` fields and
for how cancellation is handled.

## `ensure`

Create or update the object using Server-Side Apply (field manager `kube2e`).
Ownership conflicts fail by default; `run --force-conflicts` lets kube2e take
ownership of conflicting fields. Requires Kubernetes v1.22+. The object is
cached for automatic cleanup. See [Server-Side Apply & cleanup](server-side-apply.md).

```yaml
ensure:
  object: <string>     # required — key in the case objects map (flat, no target)
  values:              # template render inputs; name is injected automatically
    <key>: <value>
  retry:
    attempts: <int>
    backoff: <duration>
  timeout: <duration>  # deadline for the whole action, retries included (default: none)
```

## `patch`

Apply RFC 6902 JSON Patch operations to the **live** object on the cluster. The
action fetches the current object, applies the patches (creating missing parent
paths on `add`), and writes the result back. Because it operates on live state,
`patch` preserves fields set by earlier steps — it does not re-render the
template.

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
  timeout: <duration>  # deadline for the whole action, retries included (default: none)
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
  timeout: <duration>  # deadline for the whole action, retries included (default: none)
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
  timeout: <duration>   # deadline, or watch window for match: none (default: 2m)
```

Fresh logs (last 400 lines) are streamed on every tick. For `any` and `all`,
transient errors (pod not yet scheduled, container still initializing) are
retried silently. For `none` they are not — see below.

**Match policies:**

- `any` (default) — succeed as soon as at least one pod's logs contain the
  string. Times out with `ErrLogsNotContain`.
- `all` — succeed as soon as every resolved pod's logs contain the string. A pod
  whose logs cannot be read counts as not matching, so an unreadable pod can
  never make the check pass. Times out with `ErrLogsNotContain`.
- `none` — watch the logs for the **whole** `timeout` and fail with
  `ErrLogsContainForbidden` the moment the string appears in any pod. Succeeds
  only once the window has elapsed with every tick reading every pod's logs
  without a match. If the target's pods cannot be resolved (no pods, workload
  missing, RBAC denies listing) or any pod's logs cannot be read (container not
  started, `pods/log` forbidden), the step fails immediately with
  `ErrLogsUnavailable`: unread logs prove nothing about absence. Make sure the
  pods are running before a `none` step, e.g. with a preceding `wait`.

Because `none` is an absence assertion, it cannot pass early: the step always
takes the full `timeout`. Keep that window short (seconds, not the 2m default)
and set it to the span the string would realistically appear in. Only the
window elapsing passes: canceling the run mid-window fails the step, and so does
a read error even if it is itself a timeout.

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
  timeout: <duration>    # deadline for all attempts together, backoff included (default: 30s)
```

`timeout` is shared by every attempt: with `retry`, size it for all of them
(attempts × command time + backoff), not for a single command.

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
  timeout: <duration>    # deadline for delete retries + post-delete wait (default: 2m with wait, none without)
  retry:
    attempts: <int>
    backoff: <duration>
```

Resources created with `ensure` are cleaned up automatically after each case;
use `delete` only when a case needs to assert deletion behavior mid-run. See
[Server-Side Apply & cleanup](server-side-apply.md).
