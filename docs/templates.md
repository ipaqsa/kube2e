# Templates

Templates are Go templates with [Sprig](https://masterminds.github.io/sprig/)
helper functions. They are loaded from `templates/` at suite start and rendered
per step with the values defined in the case file. The `templates/` directory
is optional — cases that need no rendered objects can omit it entirely.

The object name is injected automatically from the `objects` map key — never
put `name` in step values. Use Sprig's `default` for every optional spec field
so templates render safely when only the name is provided (e.g. for `wait` or
`delete` steps that carry no values).

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .name }}
spec:
  replicas: {{ .replicas | default 1 }}
  selector:
    matchLabels:
      app: {{ .name }}
  template:
    metadata:
      labels:
        app: {{ .name }}
    spec:
      containers:
        - name: app
          image: {{ .image | default "nginx:1.25-alpine" }}
```

## How values flow

1. The `objects` map in the [case file](suites.md) binds a key to a template
   base-filename (without `.yaml`).
2. An action references the object by that key.
3. The key is injected as `.name`, and the action's `values` are merged in as
   the remaining template inputs.
4. The rendered manifest is applied to the cluster.

Because `.name` is always supplied, a template used by a value-less step (such
as `wait` or `delete`) still renders — provided every other field has a `default`.
