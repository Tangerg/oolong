---
title: 渲染 Markdown、代码与数学公式
description: 独立或组合使用 Markdown、语法高亮和 LaTeX。
contentType: How-to
---

# 渲染 Markdown、代码与数学公式

语言：[English](../content.md) | 简体中文

本指南先独立使用 Markdown、语法高亮和 LaTeX，再在应用边界组合它们。每个可选模块都
可以移除，应用只为自己导入的解析器承担代价。

完整程序：[`examples/markdown`](https://github.com/Tangerg/oolong/tree/main/examples/markdown)、
[`examples/latex`](https://github.com/Tangerg/oolong/tree/main/examples/latex) 和
[`examples/content`](https://github.com/Tangerg/oolong/tree/main/examples/content)

## 开始之前

把内容放进实时组件树之前，请先阅读
[组合一个可换主题的选择器](components.md)。独立的 `highlight` 与 `latex` 入口
只需要核心文本和网格模型。

## 选择内容所需的模块

三个模块彼此平级，各自拥有最自然的结果：

| 模块 | 主入口 | 结果 | 用途 |
| --- | --- | --- | --- |
| `markdown` | `markdown.Render` | `[]markdown.Block` | 结构化文本与 GFM |
| `highlight` | `highlight.Lines` | `[]text.Line` | 一段带样式的源码 |
| `latex` | `latex.Render` | `*latex.Formula` | 可测量、可选择的数学公式 |

只安装选中的模块以及应用需要的底层模块：

```sh
go get github.com/Tangerg/oolong/markdown@latest
go get github.com/Tangerg/oolong/highlight@latest
go get github.com/Tangerg/oolong/latex@latest
```

这些模块共享发布版本，但不会互相导入。

## 渲染已完成的 Markdown 文档

`markdown.Render` 返回不可变的语义块。需要用一个值完成测量、绘制、暴露选择行和缓存
宽度相关布局时，请把它们放进 `Doc`。

```go
blocks := markdown.Render(source, markdown.Look{})
doc := new(markdown.Doc)
doc.SetBlocks(blocks)

height := doc.Measure(width)
doc.Draw(view)
rows := doc.Rows(width)
```

零值外观仍会使用终端默认颜色生成可读内容。产品可以把自己的语义主题映射到
`markdown.Look`；Markdown 模块不会导入组件主题，也不会假定调色板。

## 不通过 Markdown 高亮源码

当应用已经知道一个值是代码时，直接使用 `highlight.Lines`：

```go
lines := highlight.Lines("go", source, "github-dark")
for row, line := range lines {
    line.Draw(view, 0, row)
}
```

未知语言会先根据源码推断，仍无法识别时则退化为纯文本。若周围面板需要采用所选方案的
背景，请使用 `highlight.Background`；词法单元行不会替应用强制做出这个决定。

## 不通过 Markdown 渲染公式

`latex.Render` 返回完整公式模型。不支持或未完成的输入仍会以源码形式绘制，同时可通过
`Err` 观察失败。

```go
look := latex.Look{
    Text:   theme.Text,
    Rule:   theme.Subtle,
    Error:  theme.Danger,
    Glyphs: latex.GlyphsFor(locale),
}
formula := latex.Render(`x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`, look)
if err := formula.Err(); err != nil {
    log.Printf("formula: %v", err)
}
formula.Draw(view)
```

`Formula` 还提供 `Measure`、`Width`、`Lines`、`Rows` 和 `Source`。它没有仅图像的路径，
因此数学内容在 ASCII 终端上仍可搜索、可选择且有意义。

## 在 Markdown 中组合语义渲染器

Markdown 会识别围栏代码与展示公式，由应用选择渲染每类语义内容的平级模块：

```go
look := markdown.Look{
    Text:     theme.Text,
    Headings: []grid.Style{theme.Heading, theme.Strong},
    Code:     theme.Info,
    Block:    theme.Sunken,
    Link:     theme.Accent,
    Marker:   theme.Accent,
}
look.SetRenderer(markdown.FencedCode, highlight.Of("github-dark"))
look.SetRenderer(markdown.DisplayMath, latex.Of(formulaLook))

doc.SetBlocks(markdown.Render(source, look))
```

共享接缝是由消费方定义的函数形状：

```go
type Renderer func(info, source string) []text.Line
```

解析树不会越过边界。Highlight 与 LaTeX 只返回核心样式文本，也都不知道 Markdown 是
消费方。

## 在需要时保留领域结果

`latex.Of` 是只需要文本行时使用的适配器。当应用需要统计、记录或展示解析失败时，请
直接包装 `latex.Render`：

```go
look.SetRenderer(markdown.DisplayMath,
    func(_ string, source string) []text.Line {
        formula := latex.Render(source, formulaLook)
        if err := formula.Err(); err != nil {
            metrics.RecordFormulaFailure(source, err)
        }
        return formula.Lines()
    },
)
```

返回 `nil` 表示拒绝处理扩展，并要求 Markdown 展示源码。返回非 nil 的空切片表示有意
不生成任何行。Markdown 继续拥有块布局：代码可以折行，二维数学公式则会裁剪而不重排。

## 对流式内容使用同一组合

请在馈入分块前设置完整外观。`Feed` 只返回一次稳定块，`Open` 返回仍可能变化的短尾部，
`Flush` 结算流的末尾。

```go
var stream markdown.Stream
stream.SetLook(look)
var stable, open markdown.Doc

for chunk := range answer {
    stable.Append(stream.Feed(chunk)...)
    open.SetBlocks(stream.Open())
}
stable.Append(stream.Flush()...)
open.SetBlocks(nil)
```

这两个文档让所有权切点变得可见，但不会替应用选择呈现方式。
[流式指南](streaming.md)使用 `headless.Transcript` 和 `program.ByteIngress`
展示了具体 transcript 模式。

## 分层测试每个边界

使用聚焦测试，使失败能够直接指出损坏的层级：

- 用 `markdown.Doc.Rows(width)` 断言文档结构和折行
- 用 `highlight.Lines` 的 span 断言语言与样式选择
- 用 `Formula.Err`、`Lines` 和 `Width` 断言数学输入
- 通过 `programtest` 运行组合组件，断言最终可见行为

运行仓库中的三个切片：

```sh
cd examples
go test ./markdown ./latex ./content
```

接下来阅读[构建有界流式输出](streaming.md)以接入后台字节，再阅读
[构建有界 Agent 界面](agent.md)以了解完整应用形态。
