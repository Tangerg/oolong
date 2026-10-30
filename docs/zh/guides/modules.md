---
title: 选择模块与依赖
description: 让 UI 层级共同演化，将可选依赖边界独立分发。
contentType: Reference
---

# 选择模块与依赖

语言：[English](../../guides/modules.md) | 简体中文

## 公开分发

`github.com/Tangerg/oolong/ui` 包含 `core`、`base`、`kit`、`automation` 和 `uitest`。包边界表达架构，无需让每次行为和外观变更经过多个同步模块版本。Go 只编译实际导入的包，依赖 UI 模块不意味着应用必须导入全部组件。

可选模块位于 `github.com/Tangerg/oolong/extensions/<name>`。解析器、渲染器和传输依赖构成模块边界的理由。源码导入与物理目录采用同一层次。

## 推导工作区清单

运行 `scripts/modules.sh` 获取所有工作区模块，运行 `scripts/modules.sh --public` 获取可发布模块。工具依据仓库和 Go 包事实推导成员，本文不再维护第二份依赖数量或版本目录。

`go.work` 支持本地组合。`scripts/sync-workspace.sh` 推导模块条目，`--check` 验证它们。工作区内测试通过，不能证明模块 tag 可以独立解析。发布门禁还会在 `GOWORK=off` 下使用选定模块声明的已发布依赖运行测试。

## 独立版本

一次发布指定一个模块与一个语义版本，例如 `ui/v0.1.0` 或 `extensions/markdown/v0.2.0`。独立发布仍要求真实的 `go.mod`：发布上层模块前，应声明并审查公开 API 所需的下层依赖版本。

不保留原 `core`、`components` 或顶层扩展导入路径作为适配器。此次重新设计有意要求源码迁移。[Changelog](https://github.com/Tangerg/oolong/blob/main/CHANGELOG.md) 记录替代合同。
