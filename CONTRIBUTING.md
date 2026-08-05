# Contributing

Focused issues and pull requests are welcome. Before changing behaviour or an
exported API, read the module and ring boundaries in the
[README](./README.md#what-it-is) and the reasoning behind them in
[DESIGN.md](./DESIGN.md).

## Requirements

- Go 1.26 or newer. The `go` directive is a floor, not the version anyone
  happens to have.
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
for m in core components markdown internal ptytest examples; do (cd "$m" && go test ./...); done
```

Before opening a pull request, the whole gate CI runs:

```sh
test -z "$(gofumpt -l .)"
for m in core components markdown internal ptytest examples; do (cd "$m" && \
  go mod tidy -diff && go vet ./... && go test -race -count=1 ./... && \
  golangci-lint run ./... && govulncheck ./...) || break; done
go work sync && git diff --quiet -- go.work
npx --yes markdownlint-cli2
```

Fuzz targets run in CI on every push. Run one locally with:

```sh
cd core && go test ./input/ -run '^$' -fuzz FuzzParserDoesNotCareHowBytesArrive -fuzztime=30s
```

## Boundaries

A **module** boundary is where dependencies differ, and nowhere else. It costs
version skew and buys an independent dependency set:

- `core` carries the whole third-party list and everything the engine is.
- `components` imports nothing outside `core` and the standard library.
- `markdown` is where a parser is allowed to be, and nothing imports it: that is
  what the two modules above buy by refusing one.
- `ptytest` depends on neither and nothing depends on it.
- Anything wanting a heavy dependency — markdown, syntax highlighting — becomes
  a module of its own so that neither of the first two hears about it.

A **ring** boundary is inside a module, where the compiler cannot see it:

- The substrate must not reach for the loop that drives it.
- `core/program` must never know the widgets exist. The module graph does not
  catch this — `core` could require `components` and Go would allow it.
- `components/headless` must not depend on `components/kit`, or walking away
  from one appearance would mean walking away from the behaviour too.

`internal/arch` enforces both and fails when a rule would no longer refuse
anything. Adding a module or a package means adding a rule for it.

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
