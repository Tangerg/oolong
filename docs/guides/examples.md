---
title: Runnable examples
description: Find complete commands and their tests from the checked-out example directories.
contentType: Reference
---

# Runnable examples

Language: English | [简体中文](../zh/guides/examples.md)

## Learn in order

Start with [hello](https://github.com/Tangerg/oolong/tree/main/examples/hello), then [picker](https://github.com/Tangerg/oolong/tree/main/examples/picker), [content](https://github.com/Tangerg/oolong/tree/main/examples/content), [streaming](https://github.com/Tangerg/oolong/tree/main/examples/streaming), and [agent](https://github.com/Tangerg/oolong/tree/main/examples/agent). Each command has `main_test.go`; examples are consumers of the same public APIs as applications.

## Command catalog

Run these commands from the repository root. The table follows directories containing `main.go`; the repository's example checks reject missing runnable coverage.

| Command | Evidence | Run |
| --- | --- | --- |
| `agent` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/agent) | `go run ./examples/agent` |
| `composer` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/composer) | `go run ./examples/composer` |
| `content` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/content) | `go run ./examples/content` |
| `dashboard` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/dashboard) | `go run ./examples/dashboard` |
| `files` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/files) | `go run ./examples/files` |
| `form` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/form) | `go run ./examples/form` |
| `hello` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/hello) | `go run ./examples/hello` |
| `image` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/image) | `go run ./examples/image` |
| `keys` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/keys) | `go run ./examples/keys` |
| `latex` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/latex) | `go run ./examples/latex` |
| `markdown` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/markdown) | `go run ./examples/markdown` |
| `mermaid` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/mermaid) | `go run ./examples/mermaid` |
| `picker` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/picker) | `go run ./examples/picker` |
| `read` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/read) | `go run ./examples/read` |
| `run` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/run) | `go run ./examples/run` |
| `state` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/state) | `go run ./examples/state` |
| `streaming` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/streaming) | `go run ./examples/streaming` |

## Foreground editor

The `run` example interprets `EDITOR` as a command in the host's shell syntax: `sh` on Unix and `cmd.exe` on Windows. Values such as `code --wait` and quoted executable paths can include arguments. Quote a path containing spaces according to that shell's rules; the example does not guess whether the whole value is a path. An empty value selects `vi`. The temporary filename is passed separately from command syntax, and editor failures remain process errors.

## Tests and dependencies

Run `go -C examples test ./...` for in-process example contracts. Follow [testing](testing.md) for scene and PTY boundaries. Examples are not a second framework: reusable behavior belongs in base and appearance in kit; product policy remains in each command.
