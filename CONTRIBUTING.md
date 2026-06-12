# Contributing

Thanks for helping improve `kube2e`. This guide explains how to make changes
that are easy to review, easy to test, and consistent with the project.

## Before you start

- Use Go from `go.mod` or newer compatible tooling. The repository currently
  targets Go 1.26.
- Install `golangci-lint`; CI runs it on every pull request.
- Have access to a Kubernetes cluster only when your change needs live
  end-to-end validation. Most parser, template, action, and CLI changes should
  be covered by unit tests or dry-run checks first.
- Keep changes focused. Separate unrelated refactors, behavior changes,
  examples, and documentation updates when possible.

## Contribution workflow

For small fixes, documentation updates, and tests, opening a pull request
directly is fine. For larger changes, open an issue first and describe:

- the user-visible problem or workflow being improved;
- the proposed behavior and any alternatives considered;
- compatibility risks for existing case files, templates, CLI flags, and logs;
- how the change will be tested.

Mature Kubernetes projects optimize for reviewable, incremental changes. Prefer
a small pull request that solves one problem completely over a broad pull
request that mixes refactoring, behavior changes, and formatting churn.

Bug reports should include the `kube2e` version or commit, Kubernetes version,
the command that was run, relevant case/template snippets, expected behavior,
actual behavior, and any logs needed to reproduce the issue.

## Repository layout

```text
cmd/kube2e/           CLI entry point
pkg/command/          Cobra commands and flag wiring
pkg/engine/           Public RunTests entry point
internal/engine/      Test execution engine: test -> case -> step -> action
internal/template/    Go template loading and rendering
internal/kube/        Kubernetes client and Server-Side Apply integration
internal/image/       Test image build and remote image extraction
internal/tools/       Shared helpers: filter, logs, patch, safe, workerpool
examples/             Working kube2e suites
case.yaml             Full case-file template
```

The CLI discovers suites as immediate child directories that contain a `cases/`
directory. Keep example suites under `examples/<suite>/cases` and optional
Kubernetes object templates under `examples/<suite>/templates`.

## Development commands

Use the Makefile targets when they match the task:

```bash
make build       # build bin/kube2e
make test        # run go test -v ./...
make lint        # run golangci-lint run
make fmt         # run go fmt ./...
make vet         # run go vet ./...
```

Equivalent direct commands are fine for focused work:

```bash
go test ./internal/tools/patch
go test -run TestName ./internal/tools/filter
golangci-lint run ./pkg/command/...
go build -trimpath -o bin/kube2e ./cmd/kube2e
```

For broad behavior changes, run at least:

```bash
go test -race -count=1 ./...
golangci-lint run
CGO_ENABLED=0 go build -trimpath -o /dev/null ./cmd/kube2e
```

These match the main CI checks.

## Code style

- Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
  unless this repository has a more specific local convention.
- Prefer simple, explicit Go over clever abstractions.
- Prefer explicit control flow and typed data over reflection, stringly typed
  plumbing, global state, or hidden side effects.
- Keep interfaces small and define them near the consumer.
- Pass `context.Context` as the first argument for request-scoped work; never
  store contexts in structs.
- Return errors instead of panicking. Wrap errors with `%w` when callers may
  need `errors.Is` or `errors.As`.
- Avoid logging and returning the same error unless the surrounding code already
  does that intentionally.
- Keep goroutine ownership explicit. Do not add fire-and-forget goroutines.
- Add or update tests for externally visible behavior, bug fixes, and edge
  cases.
- Run `gofmt` or `make fmt` before sending changes.

Follow the existing package boundaries:

- `pkg/command` should translate flags and environment values into typed
  execution options.
- `pkg/engine` should stay the small public API boundary.
- `internal/engine` should own test, case, step, and action execution.
- `internal/kube`, `internal/template`, `internal/image`, and `internal/tools`
  should keep implementation details out of the public API.

## Compatibility and behavior

Treat the following as user-facing compatibility surfaces:

- case file schema fields and defaults;
- suite discovery rules;
- template rendering inputs, especially injected object names;
- CLI flags, environment variables, and exit behavior;
- log format fields for `--log-format json`;
- container image layout produced by `kube2e tests publish`.

Do not break existing case files or command lines without a clear migration
path. If a breaking change is unavoidable, document the old behavior, new
behavior, and migration steps in `README.md` and the pull request.

Prefer additive changes: new optional fields, new flags with conservative
defaults, and behavior that preserves existing YAML semantics.

## Kubernetes-specific guidance

`kube2e` exists to test Kubernetes resources, so changes should respect common
Kubernetes project practices:

- Use contexts and deadlines for API work. Avoid unbounded waits.
- Keep namespace and resource cleanup deterministic and idempotent.
- Treat not-found deletes as successful cleanup unless the caller explicitly
  needs a hard failure.
- Preserve Server-Side Apply behavior and field-manager ownership unless the
  change is specifically about apply semantics.
- Avoid relying on resource ordering unless Kubernetes or `kube2e` explicitly
  guarantees it.
- Make waits and assertions observable through clear errors that include the
  object and condition involved.
- Do not require cluster-wide permissions for tests or examples unless the
  feature genuinely needs them.

## Tests

Prefer black-box tests that exercise behavior through public package APIs,
command entry points, or realistic suite/case files. Tests should describe what
`kube2e` does from a user's point of view, not lock onto private helper
implementation details.

When testing engine behavior, model inputs as suite directories with `cases/`
and `templates/` whenever practical. This keeps coverage aligned with the real
discovery and execution model:

```text
testdata/
└── suites/
    └── <suite-name>/
        ├── cases/
        │   └── <case-name>.yaml
        └── templates/
            └── <template-name>.yaml
```

Use table-driven tests when they make suite/case scenarios easier to compare,
but keep each case behavior-focused and readable.

Existing focused tests are useful examples for low-level packages where the
package itself is the public behavior under test:

- `internal/tools/filter/jq_test.go`
- `internal/tools/patch/patch_test.go`

Use the narrowest useful test tier:

- Unit tests for pure parsing, filtering, patching, template rendering, and CLI
  option handling.
- Black-box suite/case tests for discovery, schema behavior, hooks, action
  ordering, and dry-run behavior.
- Kubernetes-backed tests only for behavior that depends on the API server,
  Server-Side Apply, waits, deletion, or cleanup.

Use `go test -run <TestName> ./path/...` while debugging, then run the package
or repository test command before opening a pull request.

Avoid sleeps in tests. Prefer contexts, deadlines, fake inputs, `t.TempDir`,
polling with bounded deadlines, or direct function-level assertions.

## Testing kube2e suites

Use dry-run mode for syntax, discovery, and rendering validation when a live
cluster is not required:

```bash
go run ./cmd/kube2e run ./examples --dry-run
```

Use a real cluster only for behavior that depends on Kubernetes API responses,
Server-Side Apply, wait conditions, deletion, or cleanup:

```bash
go run ./cmd/kube2e run ./examples --kubeconfig ~/.kube/config
```

When adding or changing example suites:

- Keep each suite in `examples/<suite-name>/`.
- Put case files under `cases/` and shared object templates under `templates/`.
- Use `case.yaml` as the field reference.
- Prefer deterministic resource names and namespaces.
- Add tags when they make focused test selection useful.
- Update `README.md` if the example demonstrates a new feature or behavior.

Examples should be safe to read and adapt. Keep them minimal, deterministic,
and scoped to namespaced resources unless cluster-scoped behavior is the point
of the example.

## Documentation

Update documentation in the same change when behavior, flags, examples, or
directory layout changes. At minimum, check:

- `README.md` for user-facing behavior and examples.
- `case.yaml` for case schema fields and action semantics.
- Cobra command examples under `pkg/command/...` for CLI help output.

Keep docs concrete. Prefer working commands and short explanations over broad
project marketing copy.

## Dependencies

Dependency changes should be intentional and reviewable:

- Prefer standard library or existing dependencies before adding new ones.
- Explain why a new dependency is needed in the pull request.
- Keep `go.mod` and `go.sum` changes limited to the dependency being added,
  updated, or removed.
- Run `go mod tidy` after dependency changes.
- Be careful with Kubernetes library updates; they can change API behavior,
  transitive dependencies, and supported cluster-version assumptions.

## Pull requests

Before opening a pull request:

1. Rebase or merge the latest `main`.
2. Run the focused tests for changed packages.
3. Run `golangci-lint run`, or `make lint`.
4. Run `go test -race -count=1 ./...` for changes that touch shared behavior,
   concurrency, public APIs, serialization, Kubernetes actions, or suite
   discovery.
5. Update documentation and examples for user-facing behavior changes.
6. Include a short summary of the behavior change and the verification commands
   you ran.

CI also checks that commits include a DCO `Signed-off-by` trailer. Create signed
commits with:

```bash
git commit -s
```

or add the trailer manually:

```text
Signed-off-by: Your Name <you@example.com>
```

Good pull request descriptions include:

- what changed and why;
- user-visible behavior before and after the change;
- tests run locally;
- known follow-up work, if any.

Review comments should be resolved by updating the code, tests, or docs. If you
disagree with feedback, explain the tradeoff with concrete behavior or
maintenance impact.

## Security

Do not open public issues for vulnerabilities or credential leaks. Report them
privately to the maintainers instead. If the issue involves Kubernetes access,
include the minimal permissions, resource types, and command path needed to
reproduce the problem.

## Releases

Release images are built from tags that start with `v`. The release workflow
publishes multi-platform images to GitHub Container Registry. Do not change
release workflow behavior in the same pull request as unrelated code changes.
