# Contributing

Engineering conventions that apply to every change live in [`AGENTS.md`](./AGENTS.md). This page covers only
what is specific to Oolong: its toolchain, its package and module boundaries, and the gates a change must pass.

Before changing behaviour or an exported API, read the boundaries in the
[README](./README.md#build-from-layers) and the reasoning behind them in [DESIGN.md](./DESIGN.md).

## Requirements

### Toolchain

- **Go 1.27 or newer.** Every module declares and independently tests that same language floor, so a workspace
  build cannot hide a newer language dependency from a downstream user.
- **Pinned checkers.** `golangci-lint` v2 (CI pins v2.14.0), `deadcode` from `golang.org/x/tools` (v0.50.0),
  `gofumpt` (`v0.11.1-0.20260820074422-a2bc6805583d`), `shfmt` (v3.13.1), and `govulncheck` (v1.8.0). API and
  release checks pin `golang.org/x/exp/cmd/apidiff` and `golang.org/x/exp/cmd/gorelease` at
  `v0.0.0-20261007192929-f45ad48fbe92`.
- **Node.js 22.18 or newer** when changing Markdown or preparing a release. The exact documentation toolchain
  is in `package-lock.json`.
- Tests use the standard `testing` package.

Two pins are deliberate and must not be floated. The gofumpt pseudo-version is the first upstream revision
that understands Go 1.27 methods with type parameters; keep it exact until that support has a tagged release.
The VitePress preview pin is held because the current stable line still resolves to dependencies with
published advisories above our audit threshold. Re-evaluate the pin rather than suppressing the audit.
Markdown checks call the pinned `markdownlint` API and Node's native file glob with the rules in
`.markdownlint.json`. The scoped KaTeX resolution in `package.json` selects the
upstream security release while the math extension still requests the older series.
Remove that constraint when the extension accepts the fixed series itself.

### Workspace

This is a workspace of several modules. `go.work` is committed and a checkout does not build without it.

`scripts/modules.sh` is the executable inventory for all workspace modules, and its `--public` form is the
only tooling source for tagged modules. Public membership is derived from externally importable production
packages, not an exclusion list; `ui/internal/arch` independently reads Go's package facts and rejects drift from
either set.

## Development workflow

While iterating:

```sh
set -eu
gofumpt -w .
shfmt -w scripts
modules=$(scripts/modules.sh)
for m in $modules; do (cd "$m" && go test ./...); done
```

Before opening a pull request, run the whole gate CI runs:

```sh
set -eu
unformatted=$(gofumpt -l .)
test -z "$unformatted"
shfmt -d scripts
modules=$(scripts/modules.sh)
for m in $modules; do (cd "$m" && \
  go vet ./... && go test -race -count=1 ./... && \
  golangci-lint run ./... && govulncheck ./...); done
scripts/check-reachability.sh
go -C ui run ./internal/cmd/apiledger -root ..
scripts/sync-workspace.sh --check
npm ci
npm run docs:check
```

CI additionally copies the repository, turns `go.work` off, and checks every module with local replacements
for its declared Oolong dependencies, running both tests and `go mod tidy -diff` in that disposable graph.
Those replacements are never committed: they let a coordinated change be checked before the lower module has a
tag, while a missing `require` still fails.

Fuzz targets run in CI on every push. Run one locally with:

```sh
cd ui && go test ./core/input/ -run '^$' -fuzz FuzzParserDoesNotCareHowBytesArrive -fuzztime=30s
```

## Boundaries

### Packages and modules

The UI module contains core primitives, base behavior, kit appearance, semantic automation and test support. Optional format and transport modules live in `extensions/`. A module boundary exists where independent dependencies and versions are useful; it is not repeated at every package layer.

The executable inventory is `scripts/modules.sh`. Its `--public` form derives externally importable production modules. `scripts/sync-workspace.sh --check` verifies that the committed workspace matches the discovered module facts. Do not add a second maintained module list.

### Import directions

Core foundations provide terminal-neutral values. Text, input, scene, content, publication, transport and runtime compose only their declared lower capabilities. Core never imports base, kit, automation or test support. Base never imports kit. Format extensions consume core contracts; applications compose the extensions together.

`ui/internal/arch` reads actual Go package imports and checks the declared direct dependency DAG. Commentary may link to a higher-layer example or explain who consumes an API; those references are not imports. Test the real dependency rather than banning useful documentation navigation.

## Public API changes

Any exported change must include:

- A comment that defines behaviour and edge cases, not just restates the name.
- A test in the external package (`foo_test`) showing it from a caller's side. Tests go inside a package only
  in `*_internals_test.go`, and only for properties with no public form.
- Cancellation and concurrency semantics where they apply.
- A [CHANGELOG.md](./CHANGELOG.md) entry when existing callers must change.

A `deadcode` finding on an export asks for caller-side contract coverage. Removal additionally requires an API
review showing that the responsibility is misplaced, duplicated, or cannot be given a coherent contract.

Every incompatible API symbol since the preceding published tag must also appear in the Unreleased migration
ledger, either exactly as `apidiff` reports it or qualified by a root module's ledger name. The pinned gate
enforces that evidence independently of reachability.

Two changes are compatibility decisions rather than routine cleanup: adding a method to an exported interface
is breaking, and raising the `go` directive raises every dependent's toolchain floor.

## Documentation and examples

Write one page for one reader task. Tutorials and how-to guides go under `docs/guides`, layer explanations under `docs/core`,
`docs/base` and `docs/kit`, and decisions under `docs/decisions`. Package reference lives in Go
comments and executable examples, and complete applications under `examples`. Update the English and Chinese
pages together when they describe the same contract.

`ui/internal/arch` checks every relative Markdown path and heading anchor. The getting-started programs are
extracted and compiled, and the example catalog is derived from command directories. Every documentation page
declares its reader task in frontmatter, and every current English page has the same route below `docs/zh`. Do not add
a second hand-maintained count or list.

Install the pinned toolchain once, then run the VitePress development server from the repository root:

```sh
npm ci
npm run docs:dev
```

The architecture diagram has one JSON source. After editing it, run `node scripts/render-architecture.mjs`; `npm run docs:diagram` verifies the generated HTML.

Run `npm run docs:check` before committing navigation, theme, or Markdown changes. It audits the locked
toolchain, checks prose and internal routes, and emits the same static files that
`.github/workflows/pages.yml` deploys from `main`. Generated files under `docs/.vitepress/dist` are never
committed or uploaded by hand.

## Independent module releases

Tags are immutable dependency promises. [`scripts/release.sh`](scripts/release.sh) releases exactly one named public module after validating the committed dependency versions and repository gates. Read [the release procedure](docs/guides/releasing.md), then inspect the dry run:

```sh
scripts/release.sh ui 0.1.0
scripts/release.sh extensions/markdown 0.2.0
```

The optional `--execute` flag publishes the selected tag. The script does not edit requirements, bump peers, create implementation commits or coordinate a shared version. Lower dependencies must already be independently resolvable before a higher module is released. The changelog section uses the full module tag, such as `[ui/v0.1.0]`.

Temporary local replacements are allowed only in disposable verification graphs. No release module may contain a committed `replace`. Tag checks test the selected module against its declared published dependencies; they do not require unrelated extensions to share its version. API comparison remains a pre-publication gate.

## Tests

State behaviour, not implementation: what an idle frame must not write, what a release away from a button must
not do, what a resize must repaint. A test whose expectation is read out of the code under test passes however
that code changes, so constants are spelled out rather than imported.

A guard nobody has seen fail is a guard nobody knows is wired up. Where a test exists to catch a specific
mistake, make the mistake once and check that it fails.

## Pull requests

Keep commits reviewable and do not mix unrelated cleanup with behavioural change. Explain the problem and the
user-visible outcome, the trade-offs, and — for any performance claim — the benchmark that supports it.
