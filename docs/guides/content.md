---
title: Render rich content
description: Keep format implementations optional and prepare passive results before painting.
contentType: How-to
---

# Render rich content

Language: English | [简体中文](../zh/guides/content.md)

## Choose syntax explicitly

Run `go run ./examples/content`; the [content example](https://github.com/Tangerg/oolong/tree/main/examples/content) composes independent Markdown, highlighting and mathematics renderers. `core/content.Registry` dispatches an explicit format to a registered renderer. It never guesses syntax from incoming text.

A renderer consumes context and a complete source string, returning a passive `grid.Drawable` and an error. Useful content may accompany an error. The application decides whether a failed preparation is displayed as partial content; ignoring the error would erase a real failure fact.

## Prepare complete snapshots

`markdown.NewRenderer(markdown.RendererConfig{Look: look})` captures its appearance and returns a neutral `content.Renderer`. Each preparation renders the whole source. The returned document view can be drawn and reflowed, but it does not expose semantic setters that could revise an accepted result.

`SourceSnapshot.Prepare` may run as `program.Work[content.Prepared]`. `StreamDocument.Accept` validates the originating identity, revision and EOF state. A late worker cannot replace newer content. Custom renderers must transfer read-only semantic results; layout caches do not authorize source mutation.

## Compose optional renderers

Markdown extension hooks accept passive rendering functions from applications. This lets code fences use highlighting and mathematics use LaTeX without making the Markdown module import its peers. Prepared Mermaid output has an external-process lifetime; perform it in scoped work before drawing.

Native terminal images remain live resources. Capturing their cell rectangle is not a permanent image publication protocol: `grid.Capture` refuses native painters with `errors.ErrUnsupported`. Applications may select a textual final representation explicitly.

## Keep copying honest

Rendered rows can supply `text.Row` soft-wrap metadata. That supports logical copying and reflow without claiming a general Markdown source map. Format diagnostics belong to preparation, and no parser, network operation or external renderer runs from `Paint` or `HeightForWidth`.
