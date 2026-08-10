# Run the examples

The examples form one learning path from a two-method component to a complete mock
coding agent. Focused examples keep one `main.go`; larger examples split product
state, background sources, and review flows by responsibility.

Run commands from the repository root, where `go.work` connects the current modules:

```sh
go run ./examples/hello
```

Every example has an in-process test. Run the complete example module with:

```sh
cd examples
go test ./...
```

## Learn the runtime contract

| Example | Run | Controls | Public concepts |
| --- | --- | --- | --- |
| [`hello`](hello) | `go run ./examples/hello` | Any key counts; `q` or `Ctrl+C` quits | `program.Component`, `Config.Root`, `grid.View`, input ownership |
| [`keys`](keys) | `go run ./examples/keys` | `g` moves after a deadline; `gg` jumps to the top; `q` quits | Named actions, exact-prefix sequences, caller-owned timeout |
| [`form`](form) | `go run ./examples/form` | `Tab` changes field; arrows choose; `Enter` submits; `Esc` cancels | Controlled fields, validation, grid and spoken rendering |

Pipe the form to see the same headless fields asked without a terminal:

```sh
go run ./examples/form | cat
```

## Compose headless behavior

| Example | Run | Controls | Public concepts |
| --- | --- | --- | --- |
| [`picker`](picker) | `go run ./examples/picker` | Type to filter; arrows move; `Enter` picks; `Esc` quits | Text field, fuzzy ranking, list, highlighted matches |
| [`composer`](composer) | `go run ./examples/composer` | Type `@` for references; arrows choose; `Enter` submits; `Ctrl+C` quits | Editor completion, draft history, atomic paste elements |
| [`files`](files) | `go run ./examples/files .` | `Tab` changes pane; arrows move; left/right close/open; `q` quits | Container focus, tree identity, viewport, pointer routing |
| [`dashboard`](dashboard) | `go run ./examples/dashboard` | `Alt+Left/Right` changes tab; arrows adjust; click headers to sort; `q` quits | Tabs, tables, sliders, progress, animation lifetime |

These programs assemble product-level interactions from smaller controllers. The
library does not add a second `Picker` or `FileBrowser` API beside the primitives
that already express them.

## Render optional content

| Example | Run | Controls | Public concepts |
| --- | --- | --- | --- |
| [`markdown`](markdown) | `go run ./examples/markdown` | `q` quits | Finished Markdown blocks, document measurement, width-dependent layout |
| [`latex`](latex) | `go run ./examples/latex` | `q` quits | Standalone formula, two-dimensional layout, direct drawing |
| [`content`](content) | `go run ./examples/content` | `q` quits | Consumer-owned composition of Markdown, Highlight, and LaTeX |

The first two show that Markdown and LaTeX are complete content models on their own.
The third connects Highlight and LaTeX through Markdown's semantic renderer seam;
the optional modules remain peers and do not import one another.

## Publish streamed output

| Example | Run | Controls | Public concepts |
| --- | --- | --- | --- |
| [`run`](run) | `go run ./examples/run -- go test ./core/...` | `Ctrl+E` hands over to `$EDITOR`; `Ctrl+Z` suspends; `Ctrl+C` quits | ANSI decoding, subprocess output, terminal handover, inline publication |
| [`read`](read) | `go run ./examples/read` | `Ctrl+C` quits after the deterministic answer | Incremental Markdown, semantic extensions, LaTeX mathematics, stable blocks, open tail, scrollback ownership |
| [`streaming`](streaming) | `go run ./examples/streaming` | `Enter` requests approval; `Ctrl+X` cancels; `Ctrl+C` quits | Bounded ingress, transcript, selection, approval, failure, resize |
| [`agent`](agent) | `go run ./examples/agent` | `Enter` sends; `/help` lists commands; `Ctrl+X` cancels; `Ctrl+C` quits | Agent composition, plan, command completion, tool review, diff, bounded history |

`streaming` is the canonical library integration. `agent` adds application policy
without moving model names, tool grammar, or product state into the framework.
Neither example performs network requests or changes files.

## Read each example as a tested slice

The neighboring `main_test.go` starts the program through `programtest.Host`, sends
events, and asserts visible behavior. The complex agent also keeps narrow and wide
frame goldens. Regenerate intended visual changes with:

```sh
cd examples
go test -update ./agent
```

The streaming example adds a real-PTY test for terminal scrollback and idle output.
Use the [testing guide](../docs/testing.md) to choose between `programtest` and
`ptytest` in an application.

The repository architecture gate discovers example commands from their `main.go`
files. It rejects an example missing from this catalog, an example without a test,
or a catalog entry whose path no longer exists.
