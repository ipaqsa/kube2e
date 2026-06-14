# Server-Side Apply & cleanup

kube2e uses [Server-Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
(SSA) for all `ensure` operations, with the field manager `kube2e`. SSA requires
Kubernetes **v1.22+**.

## How objects are applied

- The [`ensure`](actions.md#ensure) action renders the object from its template
  and applies it via SSA.
- Each applied object is recorded in a thread-safe store for later cleanup.
- [`patch`](actions.md#patch) applies RFC 6902 JSON Patch operations to the
  rendered object and then re-ensures it through the same SSA path.

## Field-manager conflicts

Because kube2e applies with the field manager `kube2e`, fields owned by another
field manager — a controller, a separate `kubectl apply`, or an operator — can
produce **apply conflicts**, which fail the test.

Author templates so kube2e owns the fields it sets, and avoid taking ownership
of fields a controller manages (for example, do not set `spec.replicas` on a
Deployment that an HPA scales).

## Per-case cleanup

After each case finishes — whether it passed or failed — kube2e deletes every
object recorded in the applied cache. If the case declared a `namespace`, the
namespace is deleted after the case's resources are removed.

This cleanup is deterministic and automatic: you do not write teardown steps for
objects created with `ensure`. Use the [`delete`](actions.md#delete) action only
when a case needs to assert deletion behavior mid-run.
