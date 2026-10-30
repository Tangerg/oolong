---
title: Oolong 文档
description: 由同一套所有者组合而成的终端界面。
contentType: Landing
---

# Oolong 文档

语言：[English](/) | 简体中文

## 通过构建学习

| 步骤 | 指南 | 可运行结果 |
| --- | --- | --- |
| 1 | [第一个界面](guides/getting-started.md) | 一个场景和一个终端会话 |
| 2 | [组合组件](guides/components.md) | 共享行为与可替换外观 |
| 3 | [渲染富内容](guides/content.md) | 可选 Markdown、代码和数学 |
| 4 | [构建有界流](guides/streaming.md) | 可修订预览与显式永久发布 |
| 5 | [构建代理界面](guides/agent.md) | 人工与语义动作共享场景 |

[浏览可运行示例](guides/examples.md)。每个命令都有进程内测试。

## 按层阅读

- [Core 架构](core/architecture.md)：身份、源、布局、动作、资源与写入进度
- [逻辑文本](core/text.md)：文本位置与单元格投影
- [生命周期](core/lifetimes.md)：取消、交付、发布和物理写入
- [Base 行为](base/index.md)：可复用控制器策略
- [Kit 外观](kit/index.md)：主题与组件组合
- [历史与虚拟化](guides/history.md)：活动内容、有界窗口与应用归档

## 验证与维护

[测试](guides/testing.md)、[排错](guides/troubleshooting.md)、[模块](guides/modules.md)和[独立发布](guides/releasing.md)定义操作合同。[所有权决策](decisions/ownership.md)、[相关项目](decisions/prior-art.md)与[品牌](decisions/brand.md)记录其理由和语言。

公开模块是 `github.com/Tangerg/oolong/ui`，可选依赖边界位于 `extensions/`。包注释与可执行 Go 示例是 API 参考。`docs/report` 中历史报告保留其分析版本。
