---
title: 按所有者排查问题
description: 先找到失败合同，再决定是否需要重试或同步。
contentType: Troubleshooting
---

# 按所有者排查问题

语言：[English](../../guides/troubleshooting.md) | 简体中文

## 内容可见但尚未永久发布

检查 `StreamDocument.Current`：源 EOF、准备修订和结果状态不同。源结束时调用 `End`，并接受该最终快照的准备结果。预览有意保持不可发布。检查 Transcript 保留策略、item 错误和发布回执。部分结果必须有明确策略。

## 查询看到比终端更新的状态

分别观察模型、布局与 written 进度。准备后的布局可能等待写入准入，已接受帧也可能等待物理写入。测试需要终端外观时，应等待相关 written sequence。不要在观察回调中强制渲染。

## Target 或选区过时

Target 标识一个挂载代次。移除或虚拟窗口变化后，使用 locator 重新解析意图。逻辑选区标识内容身份和修订；编辑或不同文本投影可能使其失效。不要把旧字节端点重新解释为替换文档中的单元格。

## 发布被拒绝

`ErrCapacity` 将内容留给调用方，并可在写入进度推进后恢复。`ErrTooLarge` 要求明确调整大小或表示。`errors.ErrUnsupported` 可能表示原生 painter 或超过目标宽度的原子。`ErrClosed` 保留已知关闭或传输原因。已接受的传输部分失败后盲目重试可能产生重复输出。

## 取消后工作仍忙碌

取消是协作请求。分别检查工作结果与交付状态。如果 `Run` 忽略 context 或阻塞在外部代码中，Oolong 不能如实将其标成完成。即使如此，关闭作用域仍会撤销向已卸载界面应用迟到结果的资格。
