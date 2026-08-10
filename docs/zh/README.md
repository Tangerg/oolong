---
layout: home
title: Oolong
titleTemplate: false
description: 面向 Go 的流式优先终端 UI 底座。
contentType: Landing
hero:
  name: Oolong
  text: 面向 Go 的流式优先终端 UI
  tagline: 流可以很长，活动界面始终有界。
  image:
    src: /logo.svg
    alt: Oolong 流动标志
  actions:
    - theme: brand
      text: 构建第一个界面
      link: /zh/getting-started
    - theme: alt
      text: 浏览示例
      link: /zh/examples
features:
  - title: 输出流是首要生命周期
    details: 已完成内容进入终端回滚区，仍可交互的尾部保持小而有界。
  - title: 行为与外观自由组合
    details: 使用 headless 控制器、精致 kit 或自定义设计系统，无需改变运行时所有权。
  - title: 从底层开始保持 Go 风格
    details: 具体值、消费方接口、显式错误和单向依赖，让每一层都可以独立替换。
---

语言：[English](/) | 简体中文

## 通过构建逐步学习

采用 Oolong 时请依次完成这条路径。每一步都会产出一个可运行、经过测试的切片，并且只在上一层之上添加一种能力。

| 级别 | 构建内容 | 结果 |
| --- | --- | --- |
| 入门 | [第一个界面](getting-started.md) | 运行一个只使用 core 的全屏组件 |
| 入门 | [可换主题的选择器](components.md) | 组合 headless 行为、kit 外观、布局和输入 |
| 进阶 | [Markdown、代码与数学公式](content.md) | 独立使用并组合可选渲染器 |
| 进阶 | [有界流式输出](streaming.md) | 把有序后台字节接入内联界面 |
| 高级 | [有界 Agent 界面](agent.md) | 组合流式文本、领域事件、审核和回滚区所有权 |

如果你更喜欢从完整命令学习，请[浏览示例目录](examples.md)。每个示例都包含进程内测试。

## 选择满足需求的最小层级

只安装应用实际导入的模块：

```sh
go get github.com/Tangerg/oolong/core@latest
go get github.com/Tangerg/oolong/components@latest
```

从 `core` 获得运行时、终端、网格、输入、文本和布局。需要可复用 headless 行为与默认主题时再添加 `components`。Markdown、高亮、LaTeX、SSH 和 PTY 测试保持为独立模块。

[为界面选择模块](modules.md)列出了依赖与发布规则。

## 验证并理解系统

- [测试界面](testing.md)，覆盖进程内与伪终端边界
- [排查应用问题](troubleshooting.md)，从症状定位所有权边界
- [阅读架构](architecture.md)，理解规范性生命周期与依赖规则
- [比较既有系统](prior-art.md)，理解采纳与拒绝的设计
- [准备协调发布](releasing.md)，维护仓库发布列车

包契约发布在 [pkg.go.dev](https://pkg.go.dev/github.com/Tangerg/oolong/core)。[GitHub 仓库](https://github.com/Tangerg/oolong)包含源码、示例、变更日志与贡献规范。
