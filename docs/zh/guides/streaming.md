---
title: 构建有界流式内容
description: 保持临时 Markdown 可修订，一次性组合最终化与发布。
contentType: How-to
---

# 构建有界流式内容

语言：[English](../../guides/streaming.md) | 简体中文

## 跟随完整纵向实现

运行 `go run ./examples/streaming`，阅读带测试的[流式示例](https://github.com/Tangerg/oolong/tree/main/examples/streaming)。创建 `program.NewInline`，将它的 publisher 与示例的准备队列组合，为 Transcript 配置 `Retention: base.PublishCompleted`，再挂载其稳定元素。队列的发布适配器在永久捕获前拒绝尚未准备的宽度，准备结算后再通知保留控制器重试。

以显式格式创建 `content.StreamDocument`，只调用一次 `Transcript.AddDocument`。所有者侧接收调用 `Append`，源结束调用 `End(cause)`。显式调度 `document.Snapshot().Prepare(ctx, renderer)`，通过 `Runtime.Start` 将类型化结果带回 `document.Accept`。

共享的 `examples/internal/docprep` 组合最多准入一个实际执行中的准备任务，每个文档只保留最新待处理快照。`Task.Cancel` 请求取消，不能证明已进入的解析器已经退出。只有前一个 `Run` 真正返回并交付所有者侧完成回调，才启动下一个准备。关闭作用域会撤销交付，不能用每个 chunk 都关闭并重建作用域的方式代替执行准入。

## 绘制前准备目标宽度

`markdown.NewRenderer` 返回被动的 `markdown.Snapshot`。解析本身不代表宽度相关工作已经完成。在 worker 中调用 `Snapshot.PrepareWidth(ctx, width)`，预先完成换行和文本行投影。`PreparedLayout(width)` 只读取已经准备的不可变投影，不启动工作；即使有界宽度缓存随后被替换，永久捕获仍持有自己取得的投影。

安装一个签名为 `func(width, height int) (bool, error)` 的 `Runtime.SetFramePreparation` 检查，使用 compositor 提供的尺寸检查 Transcript 保留的正文。宽度变化时调度后台准备并返回 `(false, nil)`，由任务交付或 `Refresh` 唤醒所有者再试。就绪时返回 `(true, nil)`；不可恢复的准备失败返回 error，让 `Run` 结束并报告原因。检查不能阻塞，只负责就绪条件：scene 仍拥有候选几何和帧的原子准入，`StreamDocument` 仍拥有源修订与 EOF。旧宽度结果可以保留已解析语法，由队列准备请求的宽度，无需制造新的源修订或重新解析。

有序重绘请求等待准备就绪并实际写出后才确认成功。等待期间收到输入源 EOF，会先以 `program.ErrStopped` 和零尺寸结算该重绘，再按序处理已经准入的输入前缀，最后返回输入源结果。退出时不等待交付资格已经撤销的 worker。如果准备仍在等待且还有待写的永久捕获，这些捕获按 FIFO 顺序替换旧活动块，临时活动界面随之丢弃，防止旧视图里已经退休的条目再次输出。没有待写捕获时，退出则结束并保留已经准入的活动块。

## EOF 是语义边界

传输 chunk 是任意字节切分，可以截断 UTF-8、围栏、列表、原始 HTML 块或前向引用。因此完整 Markdown 在源 EOF 和最终准备前始终是预览。无论 chunk 如何切分，最终单元格、链接和样式都必须与整源渲染一致。

即使没有新字节，`End` 也推进修订。接受先前预览不能最终化文档。`ErrStalePreparation` 表示结果已过时，应被丢弃，不能绕过验证安装结果。将准备合并到最新快照属于调度选择，不改变文档语义。

## 保留策略组合发布准入

`KeepCompleted` 保留完整领先序列中最近的指定数量；默认零会发布所有符合条件的条目。它必须小于 `MaxItems`，为下一个源保留位置。发布容量不足时仍保留原引用，由同一个控制器在进度通知后重试。

Transcript 订阅文档变更和发布器延迟的进度通知。它只发布连续、已完成的头部，保持顺序。接受后释放正文与源订阅，保留回执；容量拒绝保留引用，并在容量变化后恢复。部分结果与失败结果默认保留，只有显式 `PublishPartial` 才允许发布。没有正文的准备失败会按同一顺序获得空捕获及立即写出的回执，不输出字节，并保留原错误，避免后续诊断被失败前缀永久堵住。

使用回执区分责任移交与物理写入。发生写入失败或 `UnknownPartial` 后，不要追加补偿性重复内容，因为已接受字节可能已经显示。应用无需手动调用 Changed、Finish、Commit 或恢复发布回调。

## 分阶段限制资源

| 阶段 | 所有者与策略 |
| --- | --- |
| 有序输入字节 | `ByteIngress` 拥有独立容量和协作式写入背压 |
| 保留的源 | `StreamConfig.MaxSourceBytes` 默认 4 MiB，超出时原子拒绝 |
| 预览准备 | 一个实际 worker、每文档最新待处理快照、显式有限准入预算 |
| 宽度布局与文本投影 | 一个派生 Markdown 宽度缓存，由帧就绪检查与不可变捕获消费 |
| 保留的活动引用 | `TranscriptConfig.MaxItems` 默认 64，不静默逐出 |
| 永久捕获 | 发布器限制单元格和单项字节，拒绝原生 painter |
| 待发与已进入写入器的责任 | 发布器限制 item 与总字节，仅在实际写入完成后释放 |
| 编码后的终端帧 | 运行时和写入器边界验证实际编码载荷 |

源上限错误不会静默将 Markdown 降级为纯文本，也不会把语法未完成的前缀声明为最终内容。应用可以报告上限、携带明确的部分失败原因结束，或显式选择其他产品表示。

## 保留阅读意图

Transcript 默认跟随新内容。用户手动滚动后，只要投影文本等价，就通过逻辑阅读锚点跨重排保持位置。选区基于内容身份与逻辑位置，而非物理行号。固定标题、选区与指针映射共享候选布局。
