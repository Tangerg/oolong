# oolong

[![ci](https://github.com/Tangerg/oolong/actions/workflows/ci.yml/badge.svg)](https://github.com/Tangerg/oolong/actions/workflows/ci.yml)
[![documentation](https://img.shields.io/badge/docs-online-a9602a)](https://tangerg.github.io/oolong/)
[![go reference](https://pkg.go.dev/badge/github.com/Tangerg/oolong/ui.svg)](https://pkg.go.dev/github.com/Tangerg/oolong/ui)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

Oolong is a terminal interface library for Go. Its core supplies terminal, text and scene primitives; base supplies reusable behavior; kit supplies appearance. Applications compose these layers for full-screen interfaces, streaming conversations and tools that remain part of a shell session.

Human input and typed automation actions use the same attached elements and controllers. Streamed content remains revisable until its source ends. Permanent output has an explicit admission receipt and physical writer progress, so reading state and source lifetime do not depend on guesses about what reached the terminal.

Oolong is pre-1.0 and requires Go 1.27. Breaking changes are documented in the [changelog](CHANGELOG.md). This redesign deliberately removes the old module paths, component routing and streaming-prefix APIs.

## Start here

Install the UI module and any optional format module your application uses:

```sh
go get github.com/Tangerg/oolong/ui@latest
go get github.com/Tangerg/oolong/extensions/markdown@latest
```

The new paths require a release containing this redesign. From this checkout, `go.work` resolves the local modules; no tags are created by applying the patch.

[Build your first interface](docs/guides/getting-started.md) contains a complete program checked by the repository's documentation tests. Follow the [bilingual learning path](docs/README.md), or run the smallest and most complete applications:

```sh
go run ./examples/hello
go run ./examples/agent
```

The [example catalog](docs/guides/examples.md) links each runnable command and its behavior tests. The streaming example also exercises a real pseudoterminal.

## Build from layers

| Layer | Physical packages | Responsibility |
| --- | --- | --- |
| Core primitives | `ui/core/grid`, `text`, `input`, `layout`, `scene` | Cells, Unicode mapping, protocol input, geometry, attachment identity and routing |
| Runtime and lifetimes | `ui/core/program`, `term`, `content`, `publication` | Owner turns, scoped work, source revision, output admission and writer progress |
| Base behavior | `ui/base` | Editors, field and choice models, dialogs, reading policy and virtual windows |
| Kit appearance | `ui/kit` | Theme tokens, skins and higher-level composition over base controllers |
| Semantic automation | `ui/automation` | Bounded observations, strict locators and typed actions over the same scene |
| Test support | `ui/uitest`, `ui/uitest/ptytest` | Application behavior and real terminal contracts |
| Optional dependencies | `extensions/markdown`, `highlight`, `latex`, `mermaid`, `ssh` | Format preparation or transport adapters with independent module dependencies |

The relationship is similar to platform primitives → unstyled behavior → styled components. A kit skin contributes to its base controller's existing element; it does not install a second focus tree, pointer router or state model. The Go import graph enforces the layer directions. See [core architecture](docs/core/architecture.md), [base](docs/base/index.md) and [kit](docs/kit/index.md).

## Construct an explicit session

| Constructor | Lifetime |
| --- | --- |
| `program.NewScreen(program.ScreenConfig{...})` | Own a full screen, then restore the terminal session |
| `program.NewInline(program.InlineConfig{...})` | Keep an interactive surface below completed output in terminal history |

Both configurations receive a `Build` function that returns one `scene.Element`, and both return a program whose `Run(ctx)` owns execution. `scene.Root` alone owns attachment, focus, capture, modal eligibility, layout identity and semantic observations. Root candidates publish geometry only when the compositor accepts the complete frame.

The runtime owns model mutation. Background work uses `Runtime.NewScope` and typed `Runtime.Start` work, returning results through owner-side delivery. Cancellation, worker completion, stale-result refusal and owner application remain distinct outcomes. Ordered byte sources can use bounded `program.ByteIngress`. [Lifetimes](docs/core/lifetimes.md) explains the contracts.

## Stream without chunk-dependent semantics

Create a `content.StreamDocument`, add it once to a Transcript, and append source bytes. End the source explicitly; schedule immutable snapshot preparation and return the result to `Accept`. The Markdown renderer prepares complete source snapshots, allowing later definitions and incomplete syntax to revise the preview. Final output is equivalent to whole-source rendering for every transport partition.

A Transcript combines document subscriptions, reading state and retention. `PublishCompleted` transfers only an eligible complete prefix to its configured publisher. `KeepCompleted` can retain a bounded recent context on the interactive surface; the default is zero. Publication refusal retains the original content and automatically retries when capacity changes.

`InlineRuntime.Publish` returns a `publication.Receipt`. `Accepted` means immutable content and output responsibility have transferred; `Written` means the terminal writer completed the associated bytes. `Failed` and `UnknownPartial` preserve failure information. Publication and writer budgets are checked at their respective owners. No application-side sequence of Changed, Finish and Commit calls is required.

Read [bounded streaming](docs/guides/streaming.md) and [history and virtualization](docs/guides/history.md) before choosing retention limits or archive policy.

## Test the contract you need

Use `ui/uitest` to drive a program without a real terminal. Use `ui/uitest/ptytest` to verify terminal mode ownership, relative cursor movement, resizing, scrollback and shutdown. Use `ui/automation` when the assertion concerns semantic identity or a typed action rather than a cell coordinate.

These facilities share production scene and action contracts. An automation locator refuses missing, ambiguous, stale or incomplete observations; it does not guess a coordinate. [Testing](docs/guides/testing.md) describes how the levels fit together.

## Modules and releases

`ui` is one independently versioned Go module. Optional format and transport modules live under `extensions/`; `examples` is a repository module. Module membership is derived by `scripts/modules.sh`, and the committed workspace is checked against it. There is no second maintained release inventory.

Each release names exactly one public module, with independently resolvable dependency versions. Releasing one extension does not bump or tag unrelated modules. See [modules](docs/guides/modules.md) and [release procedure](docs/guides/releasing.md).

## Read by task

- [Build your first interface](docs/guides/getting-started.md)
- [Compose a themeable picker](docs/guides/components.md)
- [Render Markdown, code and mathematics](docs/guides/content.md)
- [Build bounded streaming output](docs/guides/streaming.md)
- [Build an agent interface](docs/guides/agent.md)
- [Preserve history and reading intent](docs/guides/history.md)
- [Test an interface](docs/guides/testing.md)
- [Browse the examples](docs/guides/examples.md)
- [Understand ownership decisions](docs/decisions/ownership.md)
- [Compare primary reference projects](docs/decisions/prior-art.md)

[DESIGN.md](DESIGN.md) records the implemented ownership model and its limits. [ROADMAP.md](ROADMAP.md) separates implemented capabilities from remaining work. [Brand guidance](docs/decisions/brand.md) preserves the project's visual direction.

## Contribute and report problems

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) before changing behavior or exported contracts. Report security issues through [SECURITY.md](SECURITY.md). Historical reports under `docs/report` describe the revision they examined; their API statements are historical evidence, not current instructions.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
