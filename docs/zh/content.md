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

内容模块彼此平级，各自拥有最自然的结果：

| 模块 | 主入口 | 结果 | 用途 |
| --- | --- | --- | --- |
| `markdown` | `markdown.Render` | `[]markdown.Block` | 结构化文本与 GFM |
| `mermaid` | `mermaid.New` / `Render` | `*mermaid.Image, error` | 后台生成 PNG 图表 |
| `highlight` | `highlight.New` | `highlight.Renderer` | 可复用的源码样式渲染器 |
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
blocks, err := markdown.Render(source, markdown.Look{})
if err != nil {
    log.Printf("markdown: %v", err)
}
doc := new(markdown.Doc)
doc.SetBlocks(blocks)

height := doc.HeightForWidth(width)
doc.Draw(view)
rows := doc.Rows(width)
```

零值外观仍会使用终端默认颜色生成可读内容。产品可以把自己的语义主题映射到
`markdown.Look`；Markdown 模块不会导入组件主题，也不会假定调色板。

高度测量有明确单位：`grid.Drawable` 提供 `HeightForWidth(width)`。
`layout.Measurer.Measure(axis, across)` 显式指定所求轴与另一轴的可用空间。
实时容器的纵向槽使用 `HeightForWidth`，横向槽使用可选的 `WidthForHeight`；
固定与弹性槽无需内容测量能力。

## 不通过 Markdown 高亮源码

当应用已经知道一个值是代码时，先构造一个渲染器，再在所有需要该配色的地方使用同一个
`Lines` 方法：

```go
highlighter := highlight.New("github-dark")
lines := highlighter.Lines("go", source)
for row, line := range lines {
    line.Draw(view, 0, row)
}
```

未知语言会先根据源码推断，仍无法识别时则退化为纯文本。若周围面板需要采用所选方案的
背景，请使用 `highlighter.Background`；词法单元行不会替应用强制做出这个决定。

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

`Formula` 还提供 `HeightForWidth`、`Width`、`Lines`、`Rows` 和 `Source`。它没有仅图像的路径，
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
highlighter := highlight.New("github-dark")
look.SetRenderer(markdown.FencedCode,
    func(info, source string) (grid.Drawable, error) {
        return text.NewBlock(text.BlockConfig{
            Lines: highlighter.Lines(info, source), Wrap: true,
        }), nil
    },
)
look.SetRenderer(markdown.DisplayMath,
    func(_ string, source string) (grid.Drawable, error) {
        formula := latex.Render(source, formulaLook)
        return formula, formula.Err()
    },
)
blocks, err := markdown.Render(source, look)
doc.SetBlocks(blocks)
if err != nil {
    log.Printf("markdown: %v", err)
}
```

扩展直接返回 `grid.Drawable` 和错误，可选实现 `Rows(width int) []text.Row`。子内容拥有布局，Markdown 只负责缩进、装饰和块位置；无文本投影的内容保留等高空行。

## 显式处理扩展结果

内容与错误可以同时返回，Markdown 会保留可读内容并返回 `ExtensionError`，其中包含块位置、扩展类型和 info，支持 `errors.Is/As`。`nil, nil` 是契约错误；返回 `ErrUnhandled` 明确选择显示源码，空 `text.Block` 明确选择零行。

发布后的子内容必须保持语义稳定。替换文档快照，而不是修改已发布块引用的 drawable。不要在同步回调中启动浏览器或执行其他昂贵操作。

## 对流式内容使用同一组合

请在馈入分块前设置完整外观。`Feed` 只返回一次稳定块，`Open` 返回仍可能变化的短尾部，
`Flush` 结算流的末尾。

```go
var stream markdown.Stream
stream.SetLook(look)
var stable, open markdown.Doc

for chunk := range answer {
    blocks, feedErr := stream.Feed(chunk)
    stable.Append(blocks...)
    tail, openErr := stream.Open()
    open.SetBlocks(tail)
    if err := errors.Join(feedErr, openErr); err != nil {
        log.Printf("stream: %v", err)
    }
}
blocks, err := stream.Flush()
stable.Append(blocks...)
if err != nil {
    log.Printf("stream: %v", err)
}
open.SetBlocks(nil)
```

这两个文档让所有权切点变得可见，但不会替应用选择呈现方式。
[流式指南](streaming.md)使用 `headless.Transcript` 和 `program.ByteIngress`
展示了具体 transcript 模式。

## 分层测试每个边界

使用聚焦测试，使失败能够直接指出损坏的层级：

- 用 `markdown.Doc.Rows(width)` 断言文档结构和折行
- 用 `highlight.Renderer.Lines` 的 span 断言语言与样式选择
- 用 `Formula.Err`、`Lines` 和 `Width` 断言数学输入
- 通过 `programtest` 运行组合组件，断言最终可见行为

运行仓库中的三个切片：

```sh
cd examples
go test ./markdown ./latex ./content
```

接下来阅读[构建有界流式输出](streaming.md)以接入后台字节，再阅读
[构建有界 Agent 界面](agent.md)以了解完整应用形态。

## 注册共同的消费契约

`core/content.Registry` 按显式格式名分派，不导入具体格式，没有全局注册。格式名会去空格并转为小写；重复名、非法名、空回调在构造时失败，未知格式返回 `ErrUnknownFormat`。注册表不安排 goroutine，也不自动重试或猜测格式。

```go
registry, err := content.New(content.Config{Bindings: []content.Binding{
    {Format: "latex", Render: func(_ context.Context, source string) (grid.Drawable, error) {
        formula := latex.Render(source, formulaLook)
        return formula, formula.Err()
    }},
}})
if err != nil {
    return err
}
body, err := registry.Render(ctx, "latex", source)
```

`examples/content` 在顶层格式切换和 Markdown 内嵌分派中消费同一个注册表。直接的单格式调用依然适用；不需要创建注册表才能绘制文档。

## Mermaid

`mermaid` 是独立 Go 模块，使用应用安装的[官方 Mermaid CLI](https://github.com/mermaid-js/mermaid-cli)，不实现不完整的 ASCII 语法子集。进程后端支持 macOS、Linux 和 Windows 10 及以上版本；其他平台构造时返回 `errors.ErrUnsupported`。CLI 和 Chromium 是可选外部依赖，不会自动下载。

安装经过本次验证的后端并运行示例：

```sh
npm install --prefix /tmp/oolong-mermaid --save-exact @mermaid-js/mermaid-cli@11.17.0
PATH="/tmp/oolong-mermaid/node_modules/.bin:$PATH" go -C examples run ./mermaid
```

可用 `OOLONG_MERMAID_BROWSER` 为示例选择已有 Chromium 可执行文件。模块 `Config` 显式设置可执行文件、受信任的前置参数、浏览器、主题、视口、超时和源码、输出字节、解码像素与边数限额。默认限额为 30 秒（关闭最多额外 1 秒）、64 KiB 源码、8 MiB PNG、1600 万像素和 500 条边。限额约束接收的数据，不是浏览器内存或磁盘沙箱。

后台调用 `Renderer.Render(ctx, source)` 得到自持有的 PNG。内容 owner 校验任务代次及对应的源码和主题后，才通过 `Runtime.Images().Transmit` 上传，并用被动 `kit.Image` 组合进文档。Unix 取消时先向官方 CLI 发送 SIGINT，让 Puppeteer 清理独立浏览器进程组，再兜底终止 CLI 进程组；自定义 Unix 启动器必须保留该信号清理行为。Windows 在创建进程时就将 CLI 加入 Job Object，禁止浏览器子进程逃逸，并在临时文件清理前终止和等待整个作业。npm `.cmd` 启动器会解析到已安装包的入口，由 `node.exe` 直接运行，不经过命令解释器。

`examples/mermaid` 展示后台生成、过期结果拒绝、源码替换、错误展示和退出时等待 worker。替换与退出会先移除图片 placement，再释放图片数据。图片被文档持有期间不可提前释放。

语法稳定、图片准备完成、`Transcript.Finish` 是三个不同状态。等待图片的占位块不能 Finish 或 Commit；只有 owner 接受最终内容或最终错误展示后才能宣布完成。`Stream` 不负责异步图片任务，应用需要保留对应源码并在结果就绪时替换未提交的文档。

### 图片显示和操作

示例在上传前同时检查实时图片协议和单元格像素尺寸。支持 Kitty 图形协议的终端可内嵌显示图片；其他终端显示源码，仍提供 **Open Image**、**Copy Image Path** 和 **Copy Source**。使用 `o`、`p`、`c` 或点击操作栏；`r` 替换图表，`q` 退出。

macOS 和 Windows 使用系统默认图片查看器。只有打开图片或复制图片路径时才导出文件。导出的图片保留在操作系统临时目录，替换图表或退出程序后仍可使用，直到文件被清理。中间生成文件和浏览器资料目录则始终归生成任务所有，并随任务清理。

Windows 安装 Node.js 和官方 CLI 后，可在 PowerShell 运行：

```powershell
npm install -g @mermaid-js/mermaid-cli@11.17.0
go -C examples run ./mermaid
```

原生后端 CI 任务在 macOS 和 Windows 上运行官方渲染器。常规测试覆盖不支持图片的终端、剪贴板操作、导出文件保留和过期结果拒绝；Windows 专属测试验证取消和父进程正常结束时，存活的子进程都被终止。
