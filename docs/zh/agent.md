---
title: 构建有界 Agent 界面
description: 组合一个有界的流式 Agent CLI，审核工具调用，并把历史交付给终端回滚区。
contentType: Tutorial
---

# 构建有界 Agent 界面

语言：[English](../agent.md) | 简体中文

本高级教程组合一个流式 Agent CLI，同时不把模型、工具或产品语法放进框架。最终界面会
约束有序文本，在线程所有者上路由离散领域事件，审核工具调用，并把已完成历史释放给
终端回滚区。

完整实现：[`examples/agent`](https://github.com/Tangerg/oolong/tree/main/examples/agent)

## 开始之前

请依次阅读以下指南：

1. [组合一个可换主题的选择器](components.md)，理解无外观组件所有权
2. [渲染 Markdown、代码与数学公式](content.md)，理解被动内容
3. [构建有界流式输出](streaming.md)，理解字节摄入与发布

示例使用确定性的模拟后端。请在应用中把它替换成模型或进程适配器，界面架构无需改变。

## 先定义应用边界

把模型概念留在应用中。连续回答文本使用 `io.Writer`，稀疏状态转换则使用具名方法，
因为它们不是字节。

```go
type Backend interface {
    Run(context.Context, string, Output) error
}

type Output interface {
    io.Writer
    Step(context.Context, StepUpdate) error
    Review(context.Context, ChangeProposal) (bool, error)
    Tool(context.Context, ToolResult) error
}
```

这个边界不包含帧、组件、主题或终端类型。后端可以用普通 fake 测试，也可以脱离 TUI 运行。

不要把审核请求或计划转换编码进文本流。字节可以在任意边界被分块；领域事件需要身份、
确认和类型化结果。

## 为每种生命周期指定所有者

一次 Agent 运行会经过五个所有权区域：

| 状态 | 所有者 | 生命周期 |
| --- | --- | --- |
| 模型请求与传输 | 后端 goroutine | 一次运行 |
| 已接受的回答字节 | `program.ByteIngress` | 直到交付给所有者 |
| 开放的 Markdown 尾部 | 应用所有者 goroutine | 直到块完成 |
| 实时 transcript 与审核 | 无外观树 | 保持交互期间 |
| 已交付行 | 终端 | shell 会话的剩余时间 |

只有后端会并发写入。所有组件和应用实体都在运行时所有者 goroutine 上修改。

## 把会话建模为实体

把不变式收敛到一起：transcript 内容、滚动、选择、置顶提示、Markdown 流和开放块共同
描述一个会话生命周期。

```go
type conversation struct {
    content   headless.Transcript
    scroll    headless.Scroll
    selection headless.Selection
    sticky    headless.Sticky
    view      kit.Transcript

    stream  markdown.Stream
    open    *markdown.Doc
    openID  headless.BlockID
    hasOpen bool
}
```

`Markdown`、`FlushMarkdown`、`Append`、`Retain` 和 `Reset` 等方法应当属于这个实体。
若用自由函数修改其中一半字段，生命周期规则就会散落到多个调用方。

## 启动一次有界运行

在所有者 goroutine 上创建 ingress，再把它的写入端交给后端适配器。字节上限是应用策略，
应根据预期分块方式和可接受内存决定。

```go
ingress, err := program.NewByteIngress(
    runtime.Dispatcher(), 64<<10, agent.accept,
)
if err != nil {
    agent.finishRun(err)
    return
}

ctx, cancel := context.WithCancel(context.Background())
run := &agentRun{cancel: cancel}
agent.run = run
output := &agentBridge{
    ingress: ingress, dispatch: runtime.Dispatcher(),
    owner: agent, run: run,
}
```

在一个 goroutine 中启动后端，并用后端的终止错误关闭 ingress。`CloseWithError` 会在最终
批次之前保留每一个已接受字节。

```go
go func() {
    err := backend.Run(ctx, prompt, output)
    _ = ingress.CloseWithError(err)
}()
```

## 用确认机制桥接离散事件

`Dispatcher.Post` 非阻塞且有意保持通用。对于稀疏领域转换，请发布修改，并等待修改确认、
取消或运行时停止中的任一个结果。

```go
func (b *agentBridge) post(ctx context.Context, fn func()) error {
    applied := make(chan error, 1)
    b.dispatch.Post(func() {
        if b.owner.run != b.run || ctx.Err() != nil {
            err := context.Cause(ctx)
            if err == nil {
                err = context.Canceled
            }
            applied <- err
            return
        }
        fn()
        applied <- nil
    })
    select {
    case err := <-applied:
        return err
    case <-ctx.Done():
        return context.Cause(ctx)
    case <-b.dispatch.Done():
        return program.ErrStopped
    }
}
```

运行身份会拒绝已取消后端在新运行开始后送达的迟到事件。确认机制也会保持更早 ingress
工作与后续转换之间的顺序。

## 把回答分块变成稳定块

Ingress 回调在所有者上运行。把字节交给 `markdown.Stream`，只完成一次稳定块，并且只
替换开放文档。

```go
func (a *agent) accept(batch program.ByteBatch) {
    if len(batch.Data) > 0 {
        a.conversation.Markdown(string(batch.Data))
        a.conversation.Retain(a.runtime)
    }
    if batch.Final {
        a.finishRun(batch.Err)
    }
}
```

开放块保持可变，因为下一个分块可能改变其含义。稳定块会被标记为完成，且永远不会再次
解析。

## 释放已完成历史

为选择和搜索保留一小段交互窗口。通过 `kit.Transcript.CommitN` 交付更早的完成块；
同一个已测量 drawable 会被打印，随后离开实时所有权。

```go
const retainedBlocks = 8

func (c *conversation) Retain(printer kit.Printer) {
    finished := 0
    for i := range c.content.Len() {
        id := c.content.FirstBlock() + headless.BlockID(i)
        if !c.content.Finished(id) {
            break
        }
        finished++
    }
    if excess := finished - retainedBlocks; excess > 0 {
        c.view.CommitN(printer, excess)
    }
    c.scroll.ToBottom()
}
```

不要为了重绘保留已交付行的第二份副本。终端现在拥有它们。实时内存与 resize 工作量必须
取决于活动尾部，而不是会话年龄。

## 把工具审核做成领域握手

审核请求会阻塞后端，而不会阻塞界面所有者：

1. Bridge 把请求发布给所有者
2. 所有者在 `headless.Stack` 中打开 `headless.Form`
3. 允许或拒绝会向请求 channel 发送一个值
4. 后端继续执行或停止提议的操作

```go
type reviewRequest struct {
    proposal ChangeProposal
    answer   chan bool
}

func (a *agent) answerReview(approved bool) {
    request := a.review
    if request == nil {
        return
    }
    a.review = nil
    a.reviewDialog.Dismiss()
    request.answer <- approved
}
```

使用 `core/diff` 和 `kit.Diff` 渲染提议的修改。权限范围、工具名称和允许操作的效果应
保留在应用类型中。它们是产品语法，不是可复用的终端行为。

## 结算每一条终止路径

一个方法应当统一结算成功、取消、后端失败和运行时停止：

- 刷新 Markdown 尾部
- 按策略发布或保留每个已完成块
- 停止定时器与进度更新
- 拒绝任何尚未回答的审核
- 取消后端 context
- 清除当前运行身份
- 在所有者 goroutine 上呈现最终状态

终端写入失败与后端失败必须分开处理。模型可能成功完成，但终端无法接受最终呈现。

## 测试不变式而不是模拟文案

使用三个边界：

| 工具 | 证明内容 |
| --- | --- |
| 后端单元测试 | 取消、事件顺序、审核结果、短写入 |
| `programtest` | 可见计划、补全、审核、取消、最终状态 |
| `ptytest` | 回滚区发布、resize、空闲输出、终端清理 |

运行仓库中的高级切片：

```sh
cd examples
go test ./agent
go test -update ./agent  # 只在审核有意的 golden 修改时执行
```

完成的应用应当满足一个决定性属性：把已完成会话扩大一倍，不会让实时 transcript 也扩大
一倍。请阅读[架构](architecture.md)，了解该属性背后的规范性生命周期、依赖、
失败和 v1 发布规则。
