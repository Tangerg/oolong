# oolong

[![ci](https://github.com/Tangerg/oolong/actions/workflows/ci.yml/badge.svg)](https://github.com/Tangerg/oolong/actions/workflows/ci.yml)
[![documentation](https://img.shields.io/badge/docs-online-a9602a)](https://tangerg.github.io/oolong/)
[![go reference](https://pkg.go.dev/badge/github.com/Tangerg/oolong/core.svg)](https://pkg.go.dev/github.com/Tangerg/oolong/core)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

Oolong is a terminal user interface library for Go. It treats streamed output as
the primary lifetime: finished content becomes terminal scrollback, while a small
interactive tail remains live below it. Full-screen interfaces use the same grid,
input, layout, component, and runtime layers.

Oolong is pre-1.0. Releases may break exported APIs and document every break in the
[changelog](CHANGELOG.md). Every module requires Go 1.27 and is tested at that
language floor.

## Start here

[Read the documentation](https://tangerg.github.io/oolong/) for the complete
bilingual path from the runtime contract to bounded agent interfaces.

Install only the layers your program uses:

```sh
go get github.com/Tangerg/oolong/core@latest
go get github.com/Tangerg/oolong/components@latest
```

[Build your first Oolong interface](docs/getting-started.md) creates a complete
core-only program and explains its two-method component contract. Continue through
the [guided learning path](docs) to compose headless behavior, optional content,
bounded streaming, and a complete agent interface.

From a repository checkout, run the smallest and largest demonstrations:

```sh
go run ./examples/hello
go run ./examples/agent
```

The [example catalog](docs/examples.md) orders every program by the concepts it adds. Each
example has an in-process test; the canonical streaming example also runs on a real
pseudoterminal (PTY).

## Choose the output lifetime

Oolong has two runtime modes because terminal output has two distinct lifetimes:

| Mode | Use it when | What happens on exit |
| --- | --- | --- |
| `program.Config.Root` | The application owns the whole screen | The previous screen is restored |
| `program.Config.Inline` | Output should remain part of the shell session | Committed content remains in terminal scrollback |

An inline interface can publish a finished `grid.Drawable` with
`InlineRuntime.Print`. Published output leaves the component tree and is never
redrawn. This keeps memory, resize work, and interaction geometry tied to the live
tail instead of the age of the session.

## Build from layers

The library exposes one path at each level. Applications can stop at any level or
compose the next one:

| Layer | Packages | Responsibility |
| --- | --- | --- |
| Protocol and geometry | `core/ansi`, `grid`, `input`, `layout`, `text` | Terminal-neutral values, parsing, cells, styled text, and pure layout |
| Runtime and adapters | `core/program`, `term`, `programtest` | Ownership, frame scheduling, the local terminal, and in-process tests |
| Headless behavior | `components/headless` | Editors, forms, lists, trees, tables, dialogs, transcripts, and interaction state |
| Default appearance | `components/kit` | Themeable renderers and compound components over headless behavior |
| Optional content | `markdown`, `highlight`, `latex` | Incremental Markdown, syntax-highlighted source, and terminal mathematics without adding their dependencies to `core` |
| External hosts and tests | `ssh`, `ptytest` | Accepted SSH sessions and real-PTY assertions |

`headless` never imports `kit`. `program` never imports either component package.
The repository's architecture test derives every permitted import from a declared
dependency directed acyclic graph (DAG) and rejects upward edges.

## Use the component model

A root component has two methods:

```go
type Component interface {
    Draw(grid.View)
    Handle(input.Event) bool
}
```

`Draw` receives an already positioned and clipped view. `Handle` returns whether
the component consumed an event. Containers route focus and pointer input between
children; the runtime owns the only goroutine that mutates interface state.

Background work receives a concurrency-safe `program.Dispatcher`, not the runtime.
Byte streams cross through `program.ByteIngress`, an `io.Writer` with a caller-chosen
memory bound and ordered owner-side delivery.

## Test behavior at two boundaries

Use `programtest` for application behavior and `ptytest` for terminal ownership:

```go
host := programtest.New(t, 80, 24)
go program.Run(t.Context(), program.Config{Host: host, Root: build})
host.Shows(t, "ready")
host.Type("q")
```

`programtest.Host` observes frames without opening a terminal. `ptytest` starts a
real process on a PTY and can prove mode symmetry, resize behavior, idle output, and
cursor cleanup. [Test an Oolong interface](docs/testing.md) explains where each
harness belongs.

## Modules and dependencies

A module boundary exists only where the dependency set changes. Public modules are
released together under one version:

| Module | Direct third-party dependencies |
| --- | --- |
| [`core`](https://pkg.go.dev/github.com/Tangerg/oolong/core) | `uniseg`, `go-runewidth`, `x/term`, `x/sys` |
| [`components`](https://pkg.go.dev/github.com/Tangerg/oolong/components) | None beyond `core` |
| [`markdown`](https://pkg.go.dev/github.com/Tangerg/oolong/markdown) | `goldmark` |
| [`highlight`](https://pkg.go.dev/github.com/Tangerg/oolong/highlight) | `chroma` |
| [`latex`](https://pkg.go.dev/github.com/Tangerg/oolong/latex) | `go-latex` |
| [`ptytest`](https://pkg.go.dev/github.com/Tangerg/oolong/ptytest) | `x/sys` |
| [`ssh`](https://pkg.go.dev/github.com/Tangerg/oolong/ssh) | `charm.land/ssh` |

The `examples` and `internal` modules are repository tools and are not published.
Applications should upgrade all public Oolong modules together.

## Read by task

The [documentation index](docs) separates tutorials, how-to guides, reference, and
architecture material. The main paths are:

- [Build your first interface](docs/getting-started.md)
- [Compose a themeable picker](docs/components.md)
- [Render Markdown, code, and mathematics](docs/content.md)
- [Build bounded streaming output](docs/streaming.md)
- [Build a bounded agent interface](docs/agent.md)
- [Test an interface](docs/testing.md)
- [Browse the examples](docs/examples.md)
- [Understand the architecture](docs/architecture.md)
- [Compare prior systems](docs/prior-art.md)
- [Prepare a coordinated release](docs/releasing.md)

[DESIGN.md](DESIGN.md) records the implementation's current shape and limits.
[ROADMAP.md](ROADMAP.md) records completed capability work and the evidence that
closed each item. [docs/brand.md](docs/brand.md) keeps naming and visual direction
out of the Go API.

## Contribute and report problems

Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing behavior or an exported API.
Report security issues through the private process in [SECURITY.md](SECURITY.md).
Use the issue templates for reproducible defects and concrete framework-level
capability proposals.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
