---
title: 可运行示例
description: 从当前示例目录查找完整命令及其测试。
contentType: Reference
---

# 可运行示例

语言：[English](../../guides/examples.md) | 简体中文

## 按顺序学习

先运行 [hello](https://github.com/Tangerg/oolong/tree/main/examples/hello)，再看 [picker](https://github.com/Tangerg/oolong/tree/main/examples/picker)、[content](https://github.com/Tangerg/oolong/tree/main/examples/content)、[streaming](https://github.com/Tangerg/oolong/tree/main/examples/streaming) 和 [agent](https://github.com/Tangerg/oolong/tree/main/examples/agent)。每个命令都有 `main_test.go`，示例与应用消费相同公开 API。

## 命令目录

在仓库根目录执行以下命令。表格来自包含 `main.go` 的目录，仓库示例检查拒绝缺少可运行覆盖的条目。

| 命令 | 证据 | 运行 |
| --- | --- | --- |
| `agent` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/agent) | `go run ./examples/agent` |
| `composer` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/composer) | `go run ./examples/composer` |
| `content` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/content) | `go run ./examples/content` |
| `dashboard` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/dashboard) | `go run ./examples/dashboard` |
| `files` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/files) | `go run ./examples/files` |
| `form` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/form) | `go run ./examples/form` |
| `hello` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/hello) | `go run ./examples/hello` |
| `keys` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/keys) | `go run ./examples/keys` |
| `latex` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/latex) | `go run ./examples/latex` |
| `markdown` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/markdown) | `go run ./examples/markdown` |
| `mermaid` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/mermaid) | `go run ./examples/mermaid` |
| `picker` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/picker) | `go run ./examples/picker` |
| `read` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/read) | `go run ./examples/read` |
| `run` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/run) | `go run ./examples/run` |
| `state` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/state) | `go run ./examples/state` |
| `streaming` | [源码与测试](https://github.com/Tangerg/oolong/tree/main/examples/streaming) | `go run ./examples/streaming` |

## 测试与依赖

运行 `go -C examples test ./...` 验证进程内示例合同。场景与 PTY 边界参见[测试指南](testing.md)。示例不是第二套框架：可复用行为属于 base，外观属于 kit，产品策略留在各命令中。
