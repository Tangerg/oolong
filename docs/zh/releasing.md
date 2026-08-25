---
title: 准备协调发布
description: 验证并为每个公开 Oolong 模块发布同一个不可变版本。
contentType: How-to
---

# 准备协调发布

语言：[English](../releasing.md) | 简体中文

本维护者指南会为所有公开模块发布一个不可变版本。`scripts/modules.sh --public` 是发布范围的可执行清单：它从 `go.work` 推导完整成员关系，再从受支持源码集里可被外部导入的生产包推导公开子集；发布脚本从各模块的 `go.mod` 推导依赖顺序。不要手工创建 tag 或修改依赖版本。

清单推导是一项可能失败的查询，不是尽力而为的过滤器。脚本只有在全部受支持源码集都可读取后
才会发布任何名称；推导失败或公开集合为空都会停止发布。消费方会先取得一份成功的完整清单，
然后才开始迭代；诊断则始终留在标准错误。这样，工具链、依赖图或检出不可用时，就不会被误判
成“仓库里没有任何需要发布的东西”。

## 了解发布范围

公开发布列车包含 `core`、`components`、`markdown`、`highlight`、`latex`、`ptytest` 和 `ssh`。即使一个模块自身没有文件变化，它也会获得相同版本。`examples` 与 `internal` 会接受测试，但永远不会发布 tag。
这份文字列表用于解释当前发布列车；工具必须消费 `scripts/modules.sh --public`，不得复制它。

在 v1 之前，所有导出 API 都可能变化。从 v1 开始，发布必须保持与前一个 v1 版本的 Go 源码兼容性。固定版本的 `gorelease` 检查会强制执行这条边界。与此独立，钉住版本的 `apidiff` 会在每次变更中把工作 API 与前一个不可变 tag 比对。每个不兼容的导出 API 变更都必须以精确名称出现在 Unreleased 迁移清单中，使评审能够区分有意的契约决定与被仓库内可达性推动的删除。

## 准备仓库

请在 `main` 上完成以下步骤：

1. 选择下一个规范 `X.Y.Z` 版本
2. 把 `Unreleased` 下的条目移动到 `## [X.Y.Z] — YYYY-MM-DD`
3. 为后续工作留下新的空 `Unreleased` 章节
4. 运行完整 CI 工作流并解决每个失败
5. 确认 `main` 干净且与 `origin/main` 一致

任何模块都不得包含 `replace` 指令。工作区替换不属于已发布模块，它会让本地验证测试不同于消费方实际得到的依赖图。

## 检查 dry run

先运行不带 `--execute` 的发布命令：

```sh
scripts/release.sh X.Y.Z
```

Dry run 会执行本地关卡、检查第一阶段 API 兼容性、推导依赖阶段，并打印每次依赖升级与 tag。它不会修改仓库，也不会推送 tag。

请检查输出中的以下事实：

- 每个公开模块只在一个 tag 阶段出现一次
- 每个模块都在其导入的 Oolong 模块之后发布 tag
- 提议的 changelog 章节存在
- 每个不兼容的导出 API 变更，都以精确名称出现在所属模块的 Unreleased 迁移清单中
- `gorelease` 报告预期的 pre-1.0 break 或 v1 兼容结果
- 本地和远端都不存在相同版本的 tag

## 执行一次发布

检查计划后运行：

```sh
scripts/release.sh X.Y.Z --execute
```

连接终端时，脚本会再次要求输入版本。随后它会分阶段更新下游依赖、测试每个已发布依赖图、提交并推送依赖升级、在最后一个安全点运行兼容性检查，并推送带注释的模块 tag。

第一个 tag 推送后不要中断流程。Go proxy 或 checksum database 记录的 tag 是不可变的，即使稍后从 GitHub 删除也不会改变。

## 验证发布结果

脚本会验证每个远端 tag，并打印它所指向的 commit。Go proxy 最终一致，请稍后查询一次，不要在发布过程中轮询：

```sh
GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod \
  go list -m github.com/Tangerg/oolong/core@vX.Y.Z
```

再检查一个下游模块，然后根据同一个 changelog 章节创建 GitHub release notes。不要发布另一套包含不同能力主张的说明。

## 从部分发布中恢复

永远不要移动、删除或重新创建已经发布的 tag。如果流程在任何 tag 到达远端后停止：

1. 记录已经推送的模块 tag 与依赖升级 commit
2. 在 `main` 上修复原因
3. 把剩余发布说明移动到下一个 patch 版本
4. 为所有公开模块运行新的协调发布

新版本会取代不完整版本。如果某个已发布模块版本不应继续被选择，请在该模块下一次发布的 `go.mod` 中 retract 它，并在 changelog 中解释原因。
