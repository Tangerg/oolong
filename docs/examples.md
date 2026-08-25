---
title: Learn from runnable examples
description: Follow fifteen tested Oolong commands from the core runtime to a bounded mock agent.
contentType: Tutorial
---

# Learn from runnable examples

Language: English | [简体中文](zh/examples.md)

The examples form one executable learning path. Run them from a repository checkout, then read the neighboring test to see the public behavior each slice guarantees.

## Run and test the catalog

Run one command from the repository root, where `go.work` connects current modules:

```sh
go run ./examples/hello
```

Test every command from the example module:

```sh
cd examples
go test ./...
```

Start with the first group when the runtime contract is new. Jump to a later group when you already understand its prerequisites.

## Learn the runtime contract

These examples introduce drawing, input ownership, key sequences, and headless fields:

| Example | Run | Controls | Focus |
| --- | --- | --- | --- |
| [`hello`](https://github.com/Tangerg/oolong/tree/main/examples/hello) | `go run ./examples/hello` | Any key counts; `q` or `Ctrl+C` quits | `program.Component`, `Config.Root`, `grid.View` |
| [`keys`](https://github.com/Tangerg/oolong/tree/main/examples/keys) | `go run ./examples/keys` | `g` moves after a deadline; `gg` jumps; `q` quits | Named actions, exact-prefix sequences, caller-owned timeout |
| [`form`](https://github.com/Tangerg/oolong/tree/main/examples/form) | `go run ./examples/form` | `Tab` changes field; arrows choose; `Enter` submits | Controlled fields, validation, grid and spoken rendering |
| [`state`](https://github.com/Tangerg/oolong/tree/main/examples/state) | `go run ./examples/state` | Type in four fields; `Tab` advances; `Enter` submits | Local state, exact binding, normalization, and rejection through one `Accessor` seam |

Pipe the form to exercise the same headless fields without a terminal:

```sh
go run ./examples/form | cat
```

Read [Build your first interface](getting-started.md) before modifying these commands.

## Compose headless behavior

These examples combine reusable behavior while the application keeps product layout and meaning:

| Example | Run | Controls | Focus |
| --- | --- | --- | --- |
| [`picker`](https://github.com/Tangerg/oolong/tree/main/examples/picker) | `go run ./examples/picker` | Type to filter; arrows move; `Enter` picks | Text field, fuzzy ranking, list, highlighted matches |
| [`composer`](https://github.com/Tangerg/oolong/tree/main/examples/composer) | `go run ./examples/composer` | Type `@`; arrows choose; `Enter` submits | Completion, draft history, atomic paste elements |
| [`files`](https://github.com/Tangerg/oolong/tree/main/examples/files) | `go run ./examples/files .` | `Tab` changes pane; arrows navigate; `q` quits | Focus, tree identity, viewport, pointer routing |
| [`dashboard`](https://github.com/Tangerg/oolong/tree/main/examples/dashboard) | `go run ./examples/dashboard` | Press `1`–`3`, sort headers, adjust sliders | Caller-owned tabs and slider, tables, progress, animation lifetime |

The library does not add a second `Picker` or `FileBrowser` API. Each product interaction is a composition over the same controllers.

Read [Compose a themeable picker](components.md) for the ownership and appearance seams used in this group.

## Render optional content

These examples prove that each content module works independently before composition:

| Example | Run | Controls | Focus |
| --- | --- | --- | --- |
| [`markdown`](https://github.com/Tangerg/oolong/tree/main/examples/markdown) | `go run ./examples/markdown` | `q` quits | Finished blocks, measurement, width-dependent layout |
| [`latex`](https://github.com/Tangerg/oolong/tree/main/examples/latex) | `go run ./examples/latex` | `q` quits | Standalone formula, two-dimensional text layout |
| [`content`](https://github.com/Tangerg/oolong/tree/main/examples/content) | `go run ./examples/content` | `q` quits | Markdown with Highlight and LaTeX peer renderers |

Read [Render Markdown, code, and mathematics](content.md) to use the three natural entry points and their consumer-owned composition seam.

## Publish streamed output

These examples add background work, incremental transforms, inline publication, and application policy:

| Example | Run | Controls | Focus |
| --- | --- | --- | --- |
| [`run`](https://github.com/Tangerg/oolong/tree/main/examples/run) | `go run ./examples/run -- go test ./core/...` | `Ctrl+E` edits; `Ctrl+Z` suspends; `Ctrl+C` quits | ANSI decoding, subprocess output, terminal handover |
| [`read`](https://github.com/Tangerg/oolong/tree/main/examples/read) | `go run ./examples/read` | `Ctrl+C` quits after the answer | Incremental Markdown, extensions, stable blocks, open tail |
| [`streaming`](https://github.com/Tangerg/oolong/tree/main/examples/streaming) | `go run ./examples/streaming` | `Enter` approves; `Ctrl+X` cancels | Controlled dialog, bounded ingress, transcript, failure, resize |
| [`agent`](https://github.com/Tangerg/oolong/tree/main/examples/agent) | `go run ./examples/agent` | `Enter` sends; `/help` lists commands | Plan, completion, tool review, diff, bounded history |

`streaming` is the canonical library integration. `agent` adds product policy without moving model names, tool grammar, or workspace effects into the framework. Neither command performs network requests or changes files.

Read [Build bounded streaming output](streaming.md) before the canonical integration, then [Build a bounded agent interface](agent.md) for the advanced boundary.

## Review tests and visual goldens

Each `main_test.go` starts the command through `programtest.Host`, sends events, and asserts visible behavior. The streaming example also tests terminal scrollback and idle output on a real pseudoterminal (PTY).

The agent command stores narrow and wide text goldens. Regenerate them only when the new geometry is intended:

```sh
cd examples
go test -update ./agent
```

Review every changed golden beside the code change. Ordinary `go test ./...` never rewrites fixtures.
