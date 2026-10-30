---
title: 相关项目与采用决策
description: 区分可复用架构经验与其他框架的具体实现形状。
contentType: Conceptual
---

# 相关项目与采用决策

语言：[English](../../decisions/prior-art.md) | 简体中文

## 审查依据

此次重新设计依据用户要求，对照三个终端生态及其一手资料。下列源修订固定了比较依据，语言或渲染风格本身不能推出某个渲染器在所有场景下更快。

| 生态 | 审查的一手资料 |
| --- | --- |
| OpenTUI | [固定工作区](https://github.com/anomalyco/opentui/tree/ae15b990629510038339fcfb60b865106febad57)、[交互](https://opentui.com/docs/core-concepts/interaction/)、[测试](https://opentui.com/docs/core-concepts/testing/) |
| Charm | [Bubble Tea](https://github.com/charmbracelet/bubbletea/tree/67b4c7a8a2b34a0b1ce15261b0c705bc7e1a04b7)、[Bubbles](https://github.com/charmbracelet/bubbles/tree/e04077d178b7752a88ad0f687966fd10e6effd3a)、[Lip Gloss](https://github.com/charmbracelet/lipgloss/tree/19f011f1cc573e5d2bf9d7a9c4f14d429647759e)、[Huh](https://github.com/charmbracelet/huh/tree/ffb6a97195b4a031925a31cc70c62fc1d309e69d)、[Glamour](https://github.com/charmbracelet/glamour/tree/010543cac1a07c9bfd9969e58d683f2ddef4c8f7)、[Ultraviolet](https://github.com/charmbracelet/ultraviolet/tree/a45b7c86333f9dfedb95f81064e80259b02937f0)、[Wish](https://github.com/charmbracelet/wish/blob/2af05484795152f53d9ad10b5be5748a4bec3c95/README.md) |
| Ratatui | [工作区架构](https://github.com/ratatui/ratatui/blob/cdcb0c01e4348d868a7bb0a6e392d91418e7a820/ARCHITECTURE.md)、[有状态组件](https://github.com/ratatui/ratatui/blob/cdcb0c01e4348d868a7bb0a6e392d91418e7a820/ratatui-core/src/widgets/stateful_widget.rs)、[渲染](https://ratatui.rs/concepts/rendering/under-the-hood/)、[快照测试](https://ratatui.rs/recipes/testing/snapshots/) |

## Oolong 采用什么

OpenTUI 为将交互、布局、生命周期与测试视为平台能力提供参考。Oolong 将经验落实为唯一 scene 机制和类型化挂载，不引入 JavaScript 框架，也不预设终端组合必须依赖 Yoga。

Charm 全家桶体现了运行时、可复用行为、样式、表单、内容渲染与宿主集成的一致性价值。Oolong 保留这些清晰职责，将紧密共同演化的 UI 层放入一个 Go 模块，同时保持重量级格式与传输依赖可选。历史渲染器比较不会变成永久竞争性断言。

Ratatui 为状态与绘制分离、明确单元格几何合同和验证渲染行为提供参考。Oolong 保留被动绘制与对抗性 Unicode 测试，同时通过有所有者的 scene 管理交互身份、语义动作和已写入帧输入投影。

## 进一步的 UX、DX 与 AX 决策

[Base UI](https://base-ui.com/react/overview/about) 与 [shadcn/ui](https://ui.shadcn.com/docs) 明确了完整行为和默认外观组合的区别，Oolong 对应为 core 原子能力、base 控制器与 kit 外观。[Playwright locator](https://playwright.dev/docs/locators) 则启发了保留查询意图、在执行时解析当前目标的选择。

这些经验转为本地合同：皮肤不能复制状态，观察不能启动工作，过时 target 不能操作替代者，完整 Markdown 在源结束前保持可修订。决策依据用户行为和可执行证据评估，不逐字照搬上游 API。
