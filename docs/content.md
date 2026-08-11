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

The three modules are peers with different natural results:

| Module | Primary entry point | Result | Use it for |
| --- | --- | --- | --- |
| `markdown` | `markdown.Render` | `[]markdown.Block` | Structured prose and GFM |
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
blocks := markdown.Render(source, markdown.Look{})
doc := new(markdown.Doc)
doc.SetBlocks(blocks)

height := doc.Measure(width)
doc.Draw(view)
rows := doc.Rows(width)
```

The zero look remains readable in terminal-default colors. A product can map its own
semantic theme into `markdown.Look`; the Markdown module does not import a component
theme or assume a palette.

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

`Formula` also provides `Measure`, `Width`, `Lines`, `Rows`, and `Source`. There is
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
look.SetRenderer(markdown.FencedCode, highlighter.Lines)
look.SetRenderer(markdown.DisplayMath,
    func(_ string, source string) []text.Line {
        return latex.Render(source, formulaLook).Lines()
    },
)

doc.SetBlocks(markdown.Render(source, look))
```

The shared seam is a consumer-owned function shape:

```go
type Renderer func(info, source string) []text.Line
```

No parser tree crosses the boundary. Highlight and LaTeX return core styled text,
and neither knows that Markdown is the consumer.

## Observe domain results while composing

There is no second, lossy LaTeX entry point. The same `latex.Render` call used on its
own is wrapped at the consumer boundary, so an application can count, log, or display
parse failures before it returns the lines Markdown needs:

```go
look.SetRenderer(markdown.DisplayMath,
    func(_ string, source string) []text.Line {
        formula := latex.Render(source, formulaLook)
        if err := formula.Err(); err != nil {
            metrics.RecordFormulaFailure(source, err)
        }
        return formula.Lines()
    },
)
```

Returning `nil` declines an extension and asks Markdown to show its source. Returning
a non-nil empty slice deliberately produces no rows. Markdown retains block layout:
code may wrap, while two-dimensional mathematics clips instead of reflowing.

## Apply the same composition to a stream

Set the completed look before feeding chunks. `Feed` returns stable blocks once;
`Open` returns the short tail that can still change; `Flush` settles the end.

```go
var stream markdown.Stream
stream.SetLook(look)
var stable, open markdown.Doc

for chunk := range answer {
    stable.Append(stream.Feed(chunk)...)
    open.SetBlocks(stream.Open())
}
stable.Append(stream.Flush()...)
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
