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
| `keys` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/keys) | `go run ./examples/keys` |
| `latex` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/latex) | `go run ./examples/latex` |
| `markdown` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/markdown) | `go run ./examples/markdown` |
| `mermaid` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/mermaid) | `go run ./examples/mermaid` |
| `picker` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/picker) | `go run ./examples/picker` |
| `read` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/read) | `go run ./examples/read` |
| `run` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/run) | `go run ./examples/run` |
| `state` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/state) | `go run ./examples/state` |
| `streaming` | [Source and tests](https://github.com/Tangerg/oolong/tree/main/examples/streaming) | `go run ./examples/streaming` |

## Tests and dependencies

Run `go -C examples test ./...` for in-process example contracts. Follow [testing](testing.md) for scene and PTY boundaries. Examples are not a second framework: reusable behavior belongs in base and appearance in kit; product policy remains in each command.
