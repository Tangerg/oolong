---
title: 构建有界流式内容
description: 保持临时 Markdown 可修订，一次性组合最终化与发布。
contentType: How-to
---

# 构建有界流式内容

语言：[English](../../guides/streaming.md) | 简体中文

## 跟随完整纵向实现

运行 `go run ./examples/streaming`，阅读带测试的[流式示例](https://github.com/Tangerg/oolong/tree/main/examples/streaming)。创建 `program.NewInline`，为 Transcript 配置 `Publisher: runtime.Publisher()` 和 `Retention: base.PublishCompleted`，再挂载其稳定元素。

以显式格式创建 `content.StreamDocument`，只调用一次 `Transcript.AddDocument`。所有者侧接收调用 `Append`，源结束调用 `End(cause)`。显式调度 `document.Snapshot().Prepare(ctx, renderer)`，通过 `Runtime.Start` 将类型化结果带回 `document.Accept`。

## EOF 是语义边界

传输 chunk 是任意字节切分，可以截断 UTF-8、围栏、列表、原始 HTML 块或前向引用。因此完整 Markdown 在源 EOF 和最终准备前始终是预览。无论 chunk 如何切分，最终单元格、链接和样式都必须与整源渲染一致。

即使没有新字节，`End` 也推进修订。接受先前预览不能最终化文档。`ErrStalePreparation` 表示结果已过时，应被丢弃，不能绕过验证安装结果。将准备合并到最新快照属于调度选择，不改变文档语义。

## 保留策略组合发布准入

`KeepCompleted` 保留完整领先序列中最近的指定数量；默认零会发布所有符合条件的条目。它必须小于 `MaxItems`，为下一个源保留位置。发布容量不足时仍保留原引用，由同一个控制器在进度通知后重试。

Transcript 订阅文档变更和发布器延迟的进度通知。它只发布连续、已完成的头部，保持顺序。接受后释放正文与源订阅，保留回执；容量拒绝保留引用，并在容量变化后恢复。部分结果默认保留，只有显式 `PublishPartial` 才允许发布。

使用回执区分责任移交与物理写入。发生写入失败或 `UnknownPartial` 后，不要追加补偿性重复内容，因为已接受字节可能已经显示。应用无需手动调用 Changed、Finish、Commit 或恢复发布回调。

## 分阶段限制资源

| 阶段 | 所有者与策略 |
| --- | --- |
| 有序输入字节 | `ByteIngress` 拥有独立容量和协作式写入背压 |
| 保留的源 | `StreamConfig.MaxSourceBytes` 默认 4 MiB，超出时原子拒绝 |
| 预览准备 | 完整不可变源快照，由应用调度或合并作用域工作 |
| 保留的活动引用 | `TranscriptConfig.MaxItems` 默认 64，不静默逐出 |
| 永久捕获 | 发布器限制单元格和单项字节，拒绝原生 painter |
| 待发与已进入写入器的责任 | 发布器限制 item 与总字节，仅在实际写入完成后释放 |
| 编码后的终端帧 | 运行时和写入器边界验证实际编码载荷 |

源上限错误不会静默将 Markdown 降级为纯文本，也不会把语法未完成的前缀声明为最终内容。应用可以报告上限、携带明确的部分失败原因结束，或显式选择其他产品表示。

## 保留阅读意图

Transcript 默认跟随新内容。用户手动滚动后，只要投影文本等价，就通过逻辑阅读锚点跨重排保持位置。选区基于内容身份与逻辑位置，而非物理行号。固定标题、选区与指针映射共享候选布局。
