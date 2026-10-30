---
title: 行为组件
description: 组合场景原子能力，并让每个值只有一个控制器所有者。
contentType: Conceptual
---

# 行为组件

语言：[English](../../base/index.md) | 简体中文

## Base 的职责

Base 提供更换外观后仍可复用的行为：编辑、选择、列表、表单、对话框、标签、滚动、会话和虚拟窗口。它导入 core 合同，不导入 kit 或应用运行时。Base 组件暴露稳定的 `Element()` 与具体类型化操作，皮肤绘制同一个行为部件。

## 值与部件

字段控制器拥有值；表单输出与语义值是投影。验证和提交使用类型化操作并返回显式结果，不通过回调读取另一份可写副本。`EditSession` 拥有文本与修订映射；编辑器组合文档并拥有视图交互。

`Group` 和 `Overlay` 始终是 scene 机制。对话框焦点策略、标签激活和列表选择复用这些机制，不另建挂载或路由树。更换外观保留行为部件的元素身份与动作绑定。

## 富内容行为

`Transcript.AddDocument` 只连接一次 `StreamDocument`。此后的变更、EOF 准备、部分结果策略、发布准入、退休和订阅清理，由所属对象推进。调用方无需重复 Feed/Changed/Finish/Commit 协议。

`Selection` 存储相对内容的逻辑端点；`Scroll` 拥有阅读意图；`Sticky` 拥有可固定内容身份。Transcript 的帧候选组合它们的投影，绘制不推进这些状态。`Scan` 是有界同步搜索，可由调用方作为作用域工作调度，并仅接受匹配内容修订的结果。

## 大数据集

`VirtualWindow.Resolve` 在绘制前的所有者阶段显式运行。`VirtualSource` 提供数据集修订、稳定 key 与纯高度测量。窗口仅保留已实例化元素和有界测量；key 留在窗口内时复用同一元素。离开窗口会卸载 target；之后重新出现拥有新的挂载生命周期。

所有相关高度已知前，总高度明确标为估计。元数据索引与前缀遍历受 `MaxItems`、`MaxKeyBytes` 限制，测量工作受 `MaxMeasurements` 限制。[历史与虚拟化](../guides/history.md) 说明归档内容的独立所有权。
