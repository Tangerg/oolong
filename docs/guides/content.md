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

Markdown parsing accepts at most 256 simultaneously open blocks, including paragraphs and list items. Goldmark owns that structure; exceeding the bound returns `ErrBlockNestingLimit` before further nesting or recursive block traversal. Fenced code containing quote markers is still literal content. This is a block nesting budget, not a general bound on inline syntax or all parsing cost. A rejected parse returns no drawable, and a finalized `StreamDocument` reports `Failed` with the preparation error.

Markdown links admit only `http`, `https` and `mailto` destinations. Address-like labels must identify the same canonical web host or complete mailbox recipient set as the target. Validation consumes the text renderer's visible projection, so discarded controls cannot disguise the address being judged. A differing or ambiguous label loses its hidden click destination and exposes the actual target even when `Look.Target` has its zero style.

Markdown tables prepare cells and configured rule patterns as independent text fields before measuring them. Edge combining and prepend characters receive a space base that retains their style and link, so padding and separators keep their own columns. Column allocation reserves the widest indivisible grapheme as well as the readable floor; when those minimums cannot fit together, the table uses its record layout without discarding cell content.

Markdown extension hooks accept passive rendering functions from applications. Register only `markdown.FencedCode` or `markdown.DisplayMath` with `Look.SetRenderer`: Markdown owns recognition of these syntax kinds, so an unknown identity is a programming error and panics before changing existing registrations. A nil renderer removes a supported hook. Rendering failures still return through the renderer's error result. This lets code fences use highlighting and mathematics use LaTeX without making the Markdown module import its peers. Prepared Mermaid output has an external-process lifetime; perform it in scoped work before drawing.

`latex.Render` accepts a bounded expression body, without math delimiters or document commands. Its parser owns the supported command arguments, scripts and grouping, and returns every unsupported-input diagnostic through `Formula.Err`. Rendering writes neither stdout nor stderr. Ordinary mathematical text such as `08`, `1e`, `0x` and quote characters follows the expression grammar, without Go literal interpretation. Literal atoms retain display graphemes, so joined emoji and combining marks have the same geometry during layout and drawing. Math whitespace is ignored; text-style arguments preserve ordinary whitespace. A percent comment consumes its line ending in every mode. This is a terminal expression subset, not a TeX document engine.

Text arguments, including `\texttt`, preserve literal punctuation: `\texttt{file-name.txt}` renders `file-name.txt`. They still parse groups, escaped characters, comments and nested supported commands; `\texttt` is not verbatim input. Raw `^` or `_` inside a text argument returns an error and preserves the source; use `\_` for a literal underscore or an explicit mathematical argument for a script. A mathematical font command selects mathematics for its argument; returning from it restores the surrounding text context. `\operatorname` retains mathematical spacing and scripts while `* - / : ' .` have their text meaning. Ordinary spaces in operator names follow mathematical rules; write `\operatorname{lim\ sup}` for an explicit space. A mathematical font nested inside an operator retains that operator's punctuation context.

These modes intentionally replace the earlier whitespace-only distinction. Existing callers must escape text underscores, put scripts in mathematical arguments, and make spaces in operator names explicit. Text comments no longer introduce a space at their discarded line ending. [LaTeX text and mathematical fonts](https://www.latex-project.org/help/documentation/usrguide-historic.pdf) · [AMS operator names](https://www.latex-project.org/help/documentation/amsldoc.pdf) · [TeX comment scanning, sections 347 and 350](https://ctan.math.illinois.edu/info/knuth-pdf/tex/tex.pdf)

Mathematical syntax atoms keep their independent cell placement through explicit `text.Line` boundaries. In `👩{‍💻}^2`, the woman and laptop occupy separate two-column atoms and the superscript follows the grouped laptop; a complete `👩‍💻` within one atom remains one joined grapheme. `Formula.Source` retains the original expression, while `Rows` preserves the rendered atom geometry. Serializing those rows with `Line.String()` supplies copyable text without promising to retain that geometry. Named operators such as `\exp` share the same grammar; they do not require a braced argument or consume a following script as an argument.

Native terminal images remain live resources. Capturing their cell rectangle is not a permanent image publication protocol: `grid.Capture` refuses native painters with `errors.ErrUnsupported`. Applications may select a textual final representation explicitly.

Mermaid's Unix adapter keeps its launcher waitable until group cancellation and final termination finish, then reaps the launcher and settles its pipes. It does not wait for another process to reap terminated non-child zombies. `core/process` owns the shared operating-system observation and signal boundary; hosts without that identity guarantee return `errors.ErrUnsupported` at construction. Processes that deliberately leave the Unix process group are outside that boundary.

On Windows, Mermaid and the command example compose `process.Job`. That capability owns job membership at process creation, process-handle observation, termination and cleanup. Each consumer owns its prepared standard files, output copying and cancellation policy; it reaps the returned launcher and closes the job. Publish an updated UI module before resolving and releasing an adapter that consumes this new process capability.

Each returned list-item `Block` retains its children and marker together. A zero-height extension does not consume the marker; it appears beside the first visible child at the current width. An entirely invisible item still occupies one marker row. `Doc.Len` counts these retained blocks, and a nested `ExtensionError.Block` identifies the containing block in the returned sequence. `Rows` preserves the marker's column offset without adding list furniture to searchable or copied text.

## Keep copying honest

Rendered rows supply their `text.Line` projection through `text.Row.Content`, alongside soft-wrap metadata and an optional one-based `SourceLine`. Explicit grapheme boundaries survive wrapping, copying a row snapshot, selection and search geometry. `Line.String()` supplies copy or match bytes without claiming to serialize those boundaries. This supports logical copying and reflow without claiming a general Markdown source map. Format diagnostics belong to preparation, and no parser, network operation or external renderer runs from `Paint` or `HeightForWidth`.

Each Markdown block sequence owns its width cache. A reader that began before `Doc.SetBlocks` finishes against its original sequence; later readers see the replacement. A layout whose combined child heights and separator rows cannot fit in an int is refused before it can replace a valid cached layout. Embedded content retains its own concurrency contract.
