## Summary

<!-- What changed and why? -->

## Behavior before and after

<!-- Describe user-visible behavior before and after this change. Include CLI/YAML/log compatibility notes when relevant. -->

## Testing

<!-- List commands run locally. Mark anything intentionally skipped with a reason. -->

- [ ] `go test ./...`
- [ ] `go test -race -count=1 ./...`
- [ ] `golangci-lint run`
- [ ] `CGO_ENABLED=0 go build -trimpath -o /dev/null ./cmd/kube2e`
- [ ] `go run ./cmd/kube2e run ./examples --dry-run`

## Kubernetes impact

<!-- Note API-server behavior, Server-Side Apply, cleanup, waits/assertions, RBAC, namespace/resource scope, or cluster-backed testing. Write "N/A" if not applicable. -->

## Documentation

- [ ] README, `case.yaml`, examples, or CLI help updated when user-facing behavior changed.
- [ ] No documentation update needed.

## Checklist

- [ ] Change is focused and does not mix unrelated refactors.
- [ ] Existing case files and command lines remain compatible, or migration steps are documented.
- [ ] Tests are black-box or behavior-focused where practical.
- [ ] Commits include a DCO `Signed-off-by` trailer.
