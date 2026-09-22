---
title: Render Markdown, code, and mathematics
description: Use Markdown, syntax highlighting, and LaTeX independently or together.
contentType: How-to
---

# Render Markdown, code, and mathematics

Language: English | [简体中文](zh/content.md)

This guide uses Markdown, syntax highlighting, and LaTeX independently, then
combines them at the application boundary. Each optional module stays removable:
applications pay only for the parsers they import.

Complete programs: [`examples/markdown`](https://github.com/Tangerg/oolong/tree/main/examples/markdown),
[`examples/latex`](https://github.com/Tangerg/oolong/tree/main/examples/latex), and
[`examples/content`](https://github.com/Tangerg/oolong/tree/main/examples/content)

## Before you begin

Read [Compose a themeable picker](components.md) before placing content inside a live
component tree. The standalone `highlight` and `latex` entry points require only the
core text and grid model.

## Choose the modules your content needs

Content modules are peers with different natural results:

| Module | Primary entry point | Result | Use it for |
| --- | --- | --- | --- |
| `markdown` | `markdown.Render` | `[]markdown.Block` | Structured prose and GFM |
| `mermaid` | `mermaid.New` / `Render` | `*mermaid.Image, error` | PNG diagram preparation |
| `highlight` | `highlight.New` | `highlight.Renderer` | Reusable styled source rendering |
| `latex` | `latex.Render` | `*latex.Formula` | Measured, selectable mathematics |

Install only the selected modules and the lower layers your application uses:

```sh
go get github.com/Tangerg/oolong/markdown@latest
go get github.com/Tangerg/oolong/highlight@latest
go get github.com/Tangerg/oolong/latex@latest
```

These modules share a release version but do not import one another.

## Render a finished Markdown document

`markdown.Render` returns immutable semantic blocks. Put them in a `Doc` when one
value should measure, draw, expose rows for selection, and memoize width-dependent
layout.

```go
blocks, err := markdown.Render(source, markdown.Look{})
if err != nil {
    log.Printf("markdown: %v", err)
}
doc := new(markdown.Doc)
doc.SetBlocks(blocks)

height := doc.HeightForWidth(width)
doc.Draw(view)
rows := doc.Rows(width)
```

The zero look remains readable in terminal-default colors. A product can map its own
semantic theme into `markdown.Look`; the Markdown module does not import a component
theme or assume a palette.

Height is explicit: `grid.Drawable` provides `HeightForWidth(width)`, never a width
request disguised as a height request. `layout.Measurer.Measure(axis, across)` names
both the requested axis and the available cross extent. A live container uses
`HeightForWidth` for vertical slots and the optional `WidthForHeight` capability for
horizontal slots; fixed and flexible slots need neither.

## Highlight source without Markdown

Construct one renderer when the application knows a value is code, then use the same
`Lines` method everywhere that scheme is needed:

```go
highlighter := highlight.New("github-dark")
lines := highlighter.Lines("go", source)
for row, line := range lines {
    line.Draw(view, 0, row)
}
```

An unknown language falls back to source analysis and then plain text. Use
`highlighter.Background` when the surrounding pane should adopt the selected scheme's
background; token lines do not force that decision.

## Render a formula without Markdown

`latex.Render` returns the complete formula model. Unsupported or incomplete input
is still drawable as its source, while `Err` makes the failure observable.

```go
look := latex.Look{
    Text:   theme.Text,
    Rule:   theme.Subtle,
    Error:  theme.Danger,
    Glyphs: latex.GlyphsFor(locale),
}
formula := latex.Render(`x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`, look)
if err := formula.Err(); err != nil {
    log.Printf("formula: %v", err)
}
formula.Draw(view)
```

`Formula` also provides `HeightForWidth`, `Width`, `Lines`, `Rows`, and `Source`. There is
no image-only path, so mathematics remains searchable, selectable, and useful on an
ASCII terminal.

## Compose semantic renderers in Markdown

Markdown recognizes fenced code and display mathematics. The application selects
the peer that renders each semantic body:

```go
look := markdown.Look{
    Text:     theme.Text,
    Headings: []grid.Style{theme.Heading, theme.Strong},
    Code:     theme.Info,
    Block:    theme.Sunken,
    Link:     theme.Accent,
    Marker:   theme.Accent,
}
highlighter := highlight.New("github-dark")
look.SetRenderer(markdown.FencedCode,
    func(info, source string) (grid.Drawable, error) {
        return text.NewBlock(text.BlockConfig{
            Lines: highlighter.Lines(info, source), Wrap: true,
        }), nil
    },
)
look.SetRenderer(markdown.DisplayMath,
    func(_ string, source string) (grid.Drawable, error) {
        formula := latex.Render(source, formulaLook)
        return formula, formula.Err()
    },
)
blocks, err := markdown.Render(source, look)
doc.SetBlocks(blocks)
if err != nil {
    log.Printf("markdown: %v", err)
}
```

Extensions return `grid.Drawable` and an error, optionally implementing `Rows(width int) []text.Row`. Children own layout; Markdown owns indentation, decoration and block positions. Content without text projection contributes blank rows of the same height.

## Handle extension results explicitly

Content and errors may coexist. Markdown retains readable content and returns `ExtensionError` with the block index, extension and info; `errors.Is/As` preserve backend diagnostics. `nil, nil` is a contract error. Return `ErrUnhandled` to display source explicitly, or an empty `text.Block` for zero rows.

Published children must remain semantically stable. Replace document snapshots instead of mutating drawable values retained by published blocks. Do not launch a browser or perform other expensive work inside the synchronous callback.

## Apply the same composition to a stream

Set the completed look before feeding chunks. `Feed` returns stable blocks once;
`Open` returns the short tail that can still change; `Flush` settles the end.

```go
var stream markdown.Stream
stream.SetLook(look)
var stable, open markdown.Doc

for chunk := range answer {
    blocks, feedErr := stream.Feed(chunk)
    stable.Append(blocks...)
    tail, openErr := stream.Open()
    open.SetBlocks(tail)
    if err := errors.Join(feedErr, openErr); err != nil {
        log.Printf("stream: %v", err)
    }
}
blocks, err := stream.Flush()
stable.Append(blocks...)
if err != nil {
    log.Printf("stream: %v", err)
}
open.SetBlocks(nil)
```

The two documents make the ownership cut visible but do not choose a presentation.
The [streaming guide](streaming.md) shows the concrete transcript pattern with
`headless.Transcript` and `program.ByteIngress`.

## Test each boundary

Use focused tests so a failure names the layer that broke:

- Assert `markdown.Doc.Rows(width)` for document structure and wrapping
- Assert `highlight.Renderer.Lines` spans for language and style selection
- Assert `Formula.Err`, `Lines`, and `Width` for mathematical input
- Run the composed component through `programtest` for final visible behavior

Run the three repository slices with:

```sh
cd examples
go test ./markdown ./latex ./content
```

Continue with [Build bounded streaming output](streaming.md) for background bytes,
then [Build a bounded agent interface](agent.md) for the complete application shape.

## Register a common consumer contract

`core/content.Registry` dispatches explicit format names without importing implementations or using global registration. Names are trimmed and lowercased. Duplicate names, invalid names and nil callbacks fail construction; unknown formats return `ErrUnknownFormat`. The registry does not schedule goroutines, retry failures or guess formats.

```go
registry, err := content.New(content.Config{Bindings: []content.Binding{
    {Format: "latex", Render: func(_ context.Context, source string) (grid.Drawable, error) {
        formula := latex.Render(source, formulaLook)
        return formula, formula.Err()
    }},
}})
if err != nil {
    return err
}
body, err := registry.Render(ctx, "latex", source)
```

`examples/content` uses one registry for top-level format selection and Markdown embedded dispatch. Direct single-format calls remain useful; rendering a document does not require a registry.

## Mermaid

`mermaid` is an independent Go module using the rendering API of the application-installed [official Mermaid CLI](https://github.com/mermaid-js/mermaid-cli). It does not implement a partial ASCII syntax subset. The process backend supports macOS, Linux and Windows 10 or later; other platforms return `errors.ErrUnsupported` at construction. CLI and Chromium are optional external dependencies and are never downloaded automatically.

Install the backend version verified for this change and run the example:

```sh
npm install --prefix /tmp/oolong-mermaid --save-exact @mermaid-js/mermaid-cli@11.17.0
PATH="/tmp/oolong-mermaid/node_modules/.bin:$PATH" go -C examples run ./mermaid
```

Set `OOLONG_MERMAID_BROWSER` to select an existing Chromium executable in the example. Module `Config` explicitly sets the installed CLI locator, Node.js runtime, browser, theme, viewport, timeout and source/output/pixel/edge limits. Defaults are 30 seconds (plus bounded process shutdown), 64 KiB source, 8 MiB PNG, 16 million pixels and 500 edges. These limit accepted data, not browser peak memory or disk use.

Call `Renderer.Render(ctx, source)` in a worker to prepare an owned PNG. The content owner validates the generation and its source/theme before uploading through `Runtime.Images().Transmit` and composing a passive `kit.Image`. The module owns a Node.js launcher that uses the official package's rendering API and launches Chromium in the same process group on Unix. Forced group shutdown therefore does not depend on a responsive CLI or its signal handlers. Windows attaches the launcher atomically to a Job Object, tracks process handles through completion notifications, and waits for process objects to signal before returning. The configured `Executable` locates the installed npm package; it is not executed as a custom command. `Node` selects the JavaScript runtime. Arbitrary launch arguments are not supported.

`examples/mermaid` demonstrates worker preparation, stale-result rejection, source replacement, visible errors and joining workers on exit. Replacement and shutdown erase the image placement before releasing its data. Never release an image still retained by a document.

Syntax stability, image readiness and `Transcript.Finish` are separate states. Never Finish or Commit a pending image placeholder. Only the owner accepting final content or a final visible error may mark it finished. `Stream` does not own asynchronous image jobs; applications retain the matching source and replace uncommitted documents when results arrive.

### Image display and actions

The example checks both live image protocol support and cell geometry before uploading. A terminal supporting the Kitty graphics protocol can display the image inline. Other terminals show the source and keep the image available through **Open Image**, **Copy Image Path** and **Copy Source**. Use `o`, `p`, `c`, or click the toolbar; `r` replaces the diagram and `q` quits.

Opening an image uses the system viewer on macOS and Windows. Export files are created only when opening or copying an image path. They remain in the operating system's temporary directory after replacement or exit, so copied paths stay usable until those files are removed. Intermediate render files and browser profiles are always task-owned and cleaned up.

On Windows, install Node.js and the official CLI, then run from PowerShell:

```powershell
npm install -g @mermaid-js/mermaid-cli@11.17.0
go -C examples run ./mermaid
```

The native backend CI job runs the official renderer on macOS and Windows. Ordinary tests also cover unsupported terminals, clipboard actions, persistent exports, and rejection of stale work. Windows-specific tests verify that cancellation and normal parent exit both terminate surviving descendants.
