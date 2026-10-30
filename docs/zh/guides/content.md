---
title: 渲染富内容
description: 保持格式实现可选，在绘制前准备被动结果。
contentType: How-to
---

# 渲染富内容

语言：[English](../../guides/content.md) | 简体中文

## 显式选择语法

运行 `go run ./examples/content`；[内容示例](https://github.com/Tangerg/oolong/tree/main/examples/content) 组合独立的 Markdown、高亮和数学渲染器。`core/content.Registry` 将显式格式分派给已注册渲染器，不根据输入文本猜测语法。

渲染器消费 context 与完整源字符串，返回被动 `grid.Drawable` 和错误。有用内容可以伴随错误返回。应用决定是否展示准备失败的部分内容；忽略错误会抹掉真实失败事实。

## 准备完整快照

`markdown.NewRenderer(markdown.RendererConfig{Look: look})` 固定外观并返回中性的 `content.Renderer`。每次准备都渲染完整源。结果文档可以绘制和重排，但不暴露会修订已接受结果的语义 setter。

`SourceSnapshot.Prepare` 可作为 `program.Work[content.Prepared]` 执行。`StreamDocument.Accept` 验证来源身份、修订与 EOF 状态，迟到的工作线程不能覆盖新内容。自定义渲染器必须移交只读语义结果；布局缓存不授予修改源的权限。

## 组合可选渲染器

Markdown 扩展钩子接收应用提供的被动渲染函数，让代码围栏使用高亮、数学使用 LaTeX，同时不要求 Markdown 模块导入其同级模块。准备 Mermaid 输出涉及外部进程，应在绘制前作为作用域工作完成。

终端原生图片仍是活动资源。复制其单元格矩形不构成永久图片发布协议：`grid.Capture` 对原生 painter 返回 `errors.ErrUnsupported`。应用可以显式选择文本最终表示。

## 如实表达复制能力

渲染行可提供 `text.Row` 软折行元数据，在不声称拥有通用 Markdown 源映射的情况下支持逻辑复制和重排。格式诊断属于准备阶段，解析器、网络或外部渲染器不会在 `Paint` 或 `HeightForWidth` 中执行。
