# Contributing

Focused issues and pull requests are welcome. Before changing behaviour or an
exported API, read the module and ring boundaries in the
[README](./README.md#build-from-layers) and the reasoning behind them in
[DESIGN.md](./DESIGN.md).

## Requirements

- Go 1.27 or newer. Every module declares and independently tests that same language
  floor, so a workspace build cannot hide a newer language dependency from a
  downstream user.
- `golangci-lint` v2 (CI pins v2.13.1), `deadcode` from `golang.org/x/tools`
  (v0.49.0), `gofumpt` (`v0.11.1-0.20260820074422-a2bc6805583d`), `shfmt`
  (v3.13.1), and `govulncheck` (v1.7.0). API and release checks pin
  `golang.org/x/exp/cmd/apidiff` and
  `golang.org/x/exp/cmd/gorelease` at
  `v0.0.0-20260820142414-ca536658362e`. The gofumpt pseudo-version is the first
  upstream revision that understands Go 1.27 methods with type parameters; keep it
  exact until that support has a tagged release.
- Node.js 22 or newer when changing Markdown or preparing a release. The exact
  documentation toolchain is in `package-lock.json`. Its VitePress preview pin is
  deliberate: the current stable line still resolves to dependencies with
  published advisories, while this lock audits clean. Re-evaluate the pin rather
  than floating it or suppressing the audit.
- Tests written with the standard `testing` package.

This is a workspace of several modules. `go.work` is committed and a checkout
does not build without it.

## Development workflow

While iterating:

```sh
gofumpt -w .
shfmt -w scripts
for m in $(scripts/modules.sh); do (cd "$m" && go test ./...); done
```

Before opening a pull request, the whole gate CI runs:

```sh
test -z "$(gofumpt -l .)"
shfmt -d scripts
for m in $(scripts/modules.sh); do (cd "$m" && \
  go vet ./... && go test -race -count=1 ./... && \
  golangci-lint run ./... && govulncheck ./...) || break; done
scripts/check-reachability.sh
scripts/check-api-changelog.sh
go work sync && git diff --quiet -- go.work
npm ci
npm run docs:check
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
- `markdown` is where its parser is allowed to be, `highlight` is where lexers and
  palettes live, and `latex` owns mathematical parsing and terminal layout. They
  are peers and may not import one another; consumer-owned core-text seams are how
  applications compose them.
- `ptytest` depends on neither and nothing depends on it.
- Anything wanting a heavy dependency — markdown, syntax highlighting, mathematics — becomes
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
  Tests go inside a package only in `*_internals_test.go`, and only for properties
  with no public form.
- Cancellation and concurrency semantics where they apply.
- A [CHANGELOG.md](./CHANGELOG.md) entry when existing callers must change.

Related construction settings belong in a package `Config` struct, whose optional
fields have useful zero meanings. Do not add a functional-options API.

Repository usage is not an API-retention criterion. A library operation may exist
solely for downstream callers or to satisfy a consumer-owned interface. A `deadcode`
finding on an export asks for caller-side contract coverage; removal additionally
requires an API review showing that the responsibility is misplaced, duplicated, or
cannot be given a coherent contract. Every removal since the preceding published tag
must also appear by its exact old name in the Unreleased migration ledger; the pinned
`apidiff` gate enforces that evidence independently of reachability.

Adding a method to an exported interface is breaking. Raising the `go` directive
raises every dependent's toolchain floor. Both are compatibility decisions rather
than routine cleanup.

## Documentation and examples

Write one page for one reader task. Put tutorials and how-to guides under `docs`,
package reference in Go comments and executable examples, and complete applications
under `examples`. Update the English and Chinese pages together when they describe
the same contract.

Every relative Markdown path and heading anchor is checked by `internal/arch`. The
getting-started programs are extracted and compiled, while the example catalog is
derived from command directories. Every documentation page declares its reader task
in frontmatter, and every English page has the same route below `docs/zh`. Do not add
a second hand-maintained count or list.

Install the pinned documentation toolchain once, then run the VitePress development
server from the repository root:

```sh
npm ci
npm run docs:dev
```

Use `npm run docs:check` before committing navigation, theme, or Markdown changes.
This command audits the locked toolchain, checks prose and internal routes,
and emits the same static files that
`.github/workflows/pages.yml` deploys from `main`. Generated files under
`docs/.vitepress/dist` are never committed or uploaded by hand.

## Coordinated releases

Tags are immutable dependency promises; never move or recreate one that has been
published. [`scripts/release.sh`](scripts/release.sh) is the only supported release
path. It derives dependency phases, updates downstream requirements, runs the pinned
compatibility policy, and pushes one coordinated version across every public module.

Read [Prepare a coordinated release](docs/releasing.md), then inspect the dry run
before enabling its only destructive mode:

```sh
scripts/release.sh X.Y.Z
scripts/release.sh X.Y.Z --execute
```

No release tag may contain a `replace` directive. Do not create module tags or edit
Oolong dependency versions by hand; that would introduce a second release path whose
ordering and failure semantics are not guarded.

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
