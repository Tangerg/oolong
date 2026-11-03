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

`go.work` 的 `use` 指令定义工作区成员。`scripts/sync-workspace.sh` 根据成员声明的依赖生成带版本的替换规则，`--check` 验证这些规则。CI 分别验证当前工作区的集成，以及每个模块在 `GOWORK=off`、没有本地替换时使用声明的已发布依赖运行测试。发布门禁还会对选定模块重复独立检查。

## 独立版本

一次发布指定一个模块与一个语义版本，例如 `ui/v0.1.0` 或 `extensions/markdown/v0.2.0`。独立发布仍要求真实的 `go.mod`：发布上层模块前，应声明并审查公开 API 所需的下层依赖版本。

开发需要比最新 tag 更新的 API 时，先推送下层模块的实现，再在需要该 API 的消费模块中运行 `GOWORK=off go get <module>@<commit>`。Go 会选择规范的伪版本并记录校验和。随后关闭工作区运行 `go mod tidy`，并重新生成工作区替换规则。本地替换不能代替可解析的依赖声明。

不保留原 `core`、`components` 或顶层扩展导入路径作为适配器。此次重新设计有意要求源码迁移。[Changelog](https://github.com/Tangerg/oolong/blob/main/CHANGELOG.md) 记录替代合同。
