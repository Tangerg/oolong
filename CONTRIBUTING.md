# Contributing

Focused issues and pull requests are welcome. Before changing behaviour or an
exported API, read the module and ring boundaries in the
[README](./README.md#what-it-is) and the reasoning behind them in
[DESIGN.md](./DESIGN.md).

## Requirements

- Go 1.26 or newer for the complete workspace. `core`, `internal` and `ptytest`
  also test their declared Go 1.25 floor without the workspace; higher modules
  still depend on existing Oolong tags whose own directives require 1.26.
- `golangci-lint` v2 (CI pins v2.12.2), `gofumpt` (v0.11.0), `govulncheck`
  (v1.6.0).
- Node.js 22 or newer when changing Markdown.
- Tests written with the standard `testing` package.

This is a workspace of several modules. `go.work` is committed and a checkout
does not build without it.

## Development workflow

While iterating:

```sh
gofumpt -w .
for m in core components markdown highlight internal ptytest examples; do (cd "$m" && go test ./...); done
```

Before opening a pull request, the whole gate CI runs:

```sh
test -z "$(gofumpt -l .)"
for m in core components markdown highlight internal ptytest examples; do (cd "$m" && \
  go vet ./... && go test -race -count=1 ./... && \
  golangci-lint run ./... && govulncheck ./...) || break; done
go work sync && git diff --quiet -- go.work
npx --yes markdownlint-cli2
```

CI additionally copies the repository, turns `go.work` off, and checks every module
with local replacements for its declared Oolong dependencies. It runs both tests and
`go mod tidy -diff` in that disposable graph. The replacements are never committed:
they let a coordinated change be checked before the lower module has a tag, while a
missing `require` still fails.

Fuzz targets run in CI on every push. Run one locally with:

```sh
cd core && go test ./input/ -run '^$' -fuzz FuzzParserDoesNotCareHowBytesArrive -fuzztime=30s
```

## Boundaries

A **module** boundary is where dependencies differ, and nowhere else. It costs
version skew and buys an independent dependency set:

- `core` carries the whole third-party list and everything the engine is.
- `components` imports nothing outside `core` and the standard library.
- `markdown` is where a parser is allowed to be and `highlight` is where a lexer
  per language is, and nothing imports either: that is what the two modules above
  buy by refusing them. They may not import each other, which is what the seam
  between them is for.
- `ptytest` depends on neither and nothing depends on it.
- Anything wanting a heavy dependency — markdown, syntax highlighting — becomes
  a module of its own so that neither of the first two hears about it.

A **ring** boundary is inside a module, where the compiler cannot see it:

- Foundations (`ansi`, `grid`, `layout`, algorithms) know no decoded protocol,
  derived model, OS adapter or runtime.
- `input` and `text` derive from foundations; `term` adapts protocols to the OS;
  `program` composes those layers. Those directions are one-way in code and docs.
- `core/program` must never know the widgets exist. The module graph does not
  catch this — `core` could require `components` and Go would allow it.
- `components/headless` must not depend on `components/kit`, or walking away
  from one appearance would mean walking away from the behaviour too.

`internal/arch` enforces both from a direct dependency DAG. It fails if the graph is
incomplete or cyclic, and computes every permitted transitive edge rather than
maintaining a forbidden matrix. Adding a module or package means declaring its one
node and immediate lower dependencies.

## Public API changes

Any exported change must include:

- A comment that defines behaviour and edge cases, not just restates the name.
- A test in the external package (`foo_test`) showing it from a caller's side.
  Tests go inside a package only in `internals_test.go`, and only for properties
  with no public form.
- Cancellation and concurrency semantics where they apply.
- A [CHANGELOG.md](./CHANGELOG.md) entry when existing callers must change.

Adding a method to an exported interface is breaking. Raising the `go` directive
raises every dependent's toolchain floor. Both are compatibility decisions rather
than routine cleanup.

## Coordinated releases

Tags are immutable dependency promises; never move or recreate one that has been
published. A change spanning modules is released from the bottom upward:

1. Verify and tag `core/vX.Y.Z`.
2. Update `components` and `markdown` to that published core version, run with
   `GOWORK=off`, then tag the changed modules. Independent changed modules such as
   `highlight` and `ptytest` can be tagged in the same release once their own graphs
   pass.
3. Update `examples` to the published module versions and require a plain
   `GOWORK=off go mod tidy -diff` plus `go test ./...` to pass.

No release tag may contain a `replace` directive. Until step 1 exists, failure to
resolve a newly added core package from the previous tag is expected published-graph
state, not a reason to commit a local replacement.

## Tests

State behaviour, not implementation: what an idle frame must not write, what a
release away from a button must not do, what a resize must repaint. A test whose
expectation is read out of the code under test passes however that code changes,
so constants are spelled out rather than imported.

A guard nobody has seen fail is a guard nobody knows is wired up. Where a test
exists to catch a specific mistake, make the mistake once and check that it
fails.

## Pull requests

Keep commits reviewable and do not mix unrelated cleanup with behavioural change.
Explain the problem and the user-visible outcome, the trade-offs, and — for any
performance claim — the benchmark that supports it.
