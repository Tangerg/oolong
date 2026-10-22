---
title: 构建有界的流式输出
description: 接入有序字节源，同时约束待处理工作并释放已完成输出。
contentType: How-to
---

# 构建有界的流式输出

语言：[English](../streaming.md) | 简体中文

本指南把一个有序的后台字节源接入内联界面。待处理输入始终有界，未完成内容只在界面所有者上变换，完成的内容则发布到终端回滚区。

完整实现位于 [`examples/streaming`](https://github.com/Tangerg/oolong/tree/main/examples/streaming)。下面的片段只保留应用必须维护的所有权边界。

## 开始之前

请先阅读[组合一个可换主题的选择器](components.md)，理解所有者侧组件状态。
如果数据源输出 Markdown，再阅读[渲染 Markdown、代码与数学公式](content.md)；
纯字节流不需要可选内容模块。

## 分开四种生命周期

一个流式界面同时包含四种不同的状态：

1. 源 goroutine 拥有网络、进程或模型 I/O
2. `program.ByteIngress` 拥有已经接收、等待界面处理的字节
3. 界面 goroutine 拥有组件和仍然开放的变换尾部
4. `InlineRuntime.Print` 接受之后，发布缓冲区拥有已完成内容

```mermaid
flowchart LR
    source["源 goroutine"] --> ingress["有界 ByteIngress"]
    ingress --> owner["界面所有者"]
    owner --> open["开放尾部"]
    open --> committed["完成块"]
    committed -->|接受| publication["发布缓冲区"]
    publication -->|投递| terminal["终端回滚区"]
```

`ByteIngress.Write` 上的背压可以阻塞源 goroutine，以限制待处理输入；它不会阻塞界面
goroutine，也不会给运行时的通用调度器增加任意容量限制。终端投递在输出 writer 的 goroutine 上执行。

## 在字节边界定义数据源

让生产者与帧和组件保持独立：

```go
type replySource func(
    ctx context.Context,
    prompt string,
    dst io.Writer,
) error
```

HTTP 响应体、`exec.Cmd.Stdout` 或模型流都可以适配这个形状。数据源必须响应 `ctx`，维持字节顺序，并返回终止流的错误。

对于命令数据源，把同一个摄入 writer 交给 `exec.Cmd.Stdout` 和 `Stderr`，让命令统一拥有输出复制与进程等待。
run 示例把 `WaitDelay` 设为一秒：直接进程退出或取消后，即使后代继续持有输出管道，也不会让源无限等待。
未完整收集输出会返回错误；直接进程成功退出后的这种情况返回 `exec.ErrWaitDelay`，不会被报告为完整输出。

## 启动有界摄入

在界面 goroutine 上创建摄入对象。容量表示已经接收但尚未被所有者消费的字节数：

```go
ingress, err := program.NewByteIngress(program.ByteIngressConfig{
    Dispatcher: runtime.Dispatcher(),
    Limit:      64 << 10,
    Consume:    chat.accept,
})
if err != nil {
    chat.finish(err)
    return
}

ctx, cancel := context.WithCancel(context.Background())
chat.ingress = ingress
chat.cancel = cancel

go func() {
    err := chat.source(ctx, prompt, ingress)
    _ = ingress.CloseWithError(err)
}()
```

生产者只关闭摄入对象一次。`CloseWithError` 会先交付所有已经接收的字节，再交付唯一的最终批次。如果界面先停止，写入会返回 `program.ErrStopped`，待处理字节也会被释放。

## 只在所有者上执行变换

回调运行在界面 goroutine 上，因此可以直接更新组件而不需要互斥锁：

```go
func (c *chat) accept(batch program.ByteBatch) {
    if len(batch.Data) > 0 {
        c.consumeMarkdown(string(batch.Data))
    }
    if batch.Final {
        c.finish(batch.Err)
    }
}
```

`markdown.Stream.Feed` 会分离稳定块和开放尾部。稳定块只追加一次；只要后续字节还可能改变内容，就只替换开放文档。最终批次到达时调用 `Flush`。

`Open` 返回空结果时也必须替换已有尾部：原本可见的前缀可能成为不可见的 HTML 注释。
`Flush` 即使返回空结果，也结束了已有 transcript 尾部；必须完成该块，避免阻塞后续回滚区发布。
开放文档是否存在就是其生命周期的记录，不需要另设一个标志。

渲染返回前，Stream 独占未发布的源文本和开放缓存。renderer 回调可以读取 `Look`，
但在同一个 Stream 上重入 `Feed`、`Open`、`Flush`、`Reset` 或 `SetLook` 会 panic。
可以操作另一个 Stream，或在当前操作返回后再变更源文本。renderer panic 不会丢弃源文本。

已发布的链接引用定义由 Stream 保留到 `Flush` 或 `Reset`，后续块可以跨发布切点使用这些定义。
`Open` 使用临时解析上下文，不会提前提交尚未结束的定义。Stream 只保留定义本身，沿用解析器
对重复标签采用首个定义的规则，不保留已发布源码或语法树。链接发布之后才到达的定义，不能
回头修改已经移交的块。

解析器会保留尚未结束的列表和引用容器，包括项目之间的空行。很长的松散列表可能形成
很长的开放尾部；逐项独立发布会使编号和嵌套关系受输入分块方式影响。

## 交付完成内容的所有权

在活动 transcript 中保留少量完成块，以支持选择和搜索；通过 transcript 渲染器的发布操作交付更早的块。
`kit.Transcript.Commit` 只在 `Printer.Print` 返回 nil 后释放对应的块，并返回已接受数量和拒绝错误；
被拒绝的内容保持存活。同一个 drawable 会被测量并绘制为发布缓冲区中的不可变单元格，后者接手投递或失败报告。
零值或已停止的 inline runtime 返回 `program.ErrStopped`。

`headless.Transcript.Commit` 在调用下一个块的回调前，立即移除已接受块。
回调只允许读取 transcript；修改、暂存布局或再次 Commit 都会 panic。回调异常退出时，
它的输出结果未知，该 transcript 不得继续修改或发布；更早的已接受块仍保持移除状态。
即使异常退出或视图字段被替换，kit 操作仍会按已接受的行结算原始 scroll、selection 与 sticky 模型。

`grid.Inline` 在回调执行期间拥有整个 Print、Append、Flush 或 Finish 操作。
回调可以读取画布、绘制传入的 view；进一步的画布操作须安排在外层返回之后。
重入修改或发布会 panic，嵌套输出不能再替换外层操作的临时绘图状态或终端锚点。
传入的绘制 view、它的副本与子视图只在该次回调内有效。需要之后继续绘制时保留内容；
通过保存的回调 view 写入会 panic，即使之前的回调异常退出也一样。

Scrollback 接收单元格内容。原生绘制区域需要实时帧拥有其放置与移除，`Print` 和 `Append`
会以 `errors.ErrUnsupported` 拒绝这类内容。保留该块，或先显式替换成单元格内容。
`Append` 向回调提供当前行的剩余宽度；返回 true 继续下一行，false 表示完成。grid 一次性
接受这次调用的全部行；不支持的区域或回调异常退出都不会发布其中任何一行。
在完整空行上没有绘制内容却仍要求继续，会按同样规则返回 `io.ErrNoProgress`。

Flush 按当前宽度对待发单元格换行，并按当前色深编码。宽度变化会关闭开放的追加行，
待发的续接内容不能使用旧终端列号，之后的 Append 从新行开始。宽度为零时延后输出，
保留已接受的单元格。若单个显示原子比整个终端还宽，Flush 在写出前返回
`errors.ErrUnsupported` 并保留全部待发内容；恢复合适的宽度后可以重试。投递成功后释放单元格。

调用 `grid.Inline.Finish` 前先用 Flush 投递全部已接受内容。Finish 在队列非空时返回错误，
不写出，也不改变终端锚点或开放行；先投递，再重试收尾。零宽度下 Flush 延后投递，
即使返回 nil，也不意味着待发队列已经为空。

不要在应用状态中重建已经接受的行。发布链路已拥有它们，`program.Run` 会报告后续投递失败。
重新构建既会让内存和 resize 工作随着会话年龄增长，也可能重放已经投递的输出。

## 结算取消与失败

取消是数据源的终止结果，不是第二条交付路径：

- 由界面所有者取消数据源 context
- 让数据源返回 `context.Canceled`
- 用这个结果关闭摄入对象
- 根据最终 `ByteBatch` 结算界面状态
- 其他 goroutine 需要释放数据源资源时，等待 `ByteIngress.Done`

示例只将 `context.Canceled` 本身视为取消。包装或合并后的错误会显示完整诊断，
取消不能掩盖其他失败。

终端写入失败与数据源失败必须分开处理。数据源可能成功结束，但呈现过程仍可能失败。
正常完成时，以 nil 或 `io.EOF` 本身关闭摄入。包装或合并后的错误会原样进入最终批次，
与 EOF 合并的其他失败也会保留。

## 验证完整路径

使用 `programtest` 验证所有者侧状态；使用 PTY 测试验证永久发布、空闲输出、resize 和
终端模式清理。[测试指南](testing.md)定义了这两条边界。继续阅读
[构建有界 Agent 界面](agent.md)，在这条字节路径周围添加类型化领域事件与工具审核。
