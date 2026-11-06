---
title: 构建代理界面
description: 让人工输入和自动化共享类型化动作、可用性和结果生命周期。
contentType: How-to
---

# 构建代理界面

语言：[English](../../guides/agent.md) | 简体中文

## 从真实应用开始

运行 `go run ./examples/agent`，阅读 [agent 示例](https://github.com/Tangerg/oolong/tree/main/examples/agent)。领域任务、工具事件和持久化仍是应用策略。StreamDocument 拥有内容语义，Transcript 拥有阅读与保留，运行时拥有工作交付与终端输出。

使用 `/plan fast` 或 `/plan careful` 选择下一次请求的规划深度。`/help` 列出当前会话注册的命令。

## 观察不产生变更

使用 `driver, err := automation.NewDriver(runtime)` 构造，并在保存 driver 之前处理错误。缺失会话、场景或 dispatcher 会在构造时返回 `automation.ErrInvalid`。Driver 将查询和动作调度到 UI 所有者；可以在 Build 阶段保存语义句柄，挂载后再查询。它无法准备帧、推进写入水位或关闭程序的 Root。`Observe` 读取现有语义投影，不启动绘制、不填充隐藏内容、不创建后台工作。

`scene.Query` 限制深度与节点数量。`TextLimit` 限制每个显示字段，`ByteLimit` 限制返回节点与动作中全部字符串的总字节数，`DetailLimit` 限制动作和有效关系的合计数量。零值使用包内默认政策，正值不超过对应的包级上限。这些预算约束观察投影；应用回调内部的分配及传输序列化仍由各自边界负责。

把不透明的 `Observation.Next` 原样传回 `Query.Cursor`。游标绑定场景、模型修订、实际子树及深度；串用不同遍历会返回 `ErrObservationChanged`。每页预算可以调整，但节点及其动作、关系列表始终整体接纳。如果第一个节点也无法放入空页，返回 `ErrObservationBudget`，让调用方提高对应预算，而不会将缺失字段的节点伪装成完整证据。Key、target 的 part、内容 ID、动作 ID 和类型名不会截断；动作标签与可用性原因的截断会记录在 `ActionDescription.TruncatedText` 中。

零值游标开始新遍历，敏感值会被隐藏。`Progress` 分别报告模型、已准备布局和已写入帧进度；观察到模型不代表它已经出现在终端上。

## 定位并调用唯一当前目标

无参数动作可使用 `driver.Locate(query, scene.Match{Key: "submit"}).Invoke(ctx, scene.ActionSubmit, nil)`。Locator 保存意图，在同一个所有者轮次内解析唯一当前匹配并调用。缺失、歧义、截断、已卸载和被模态阻挡，都是显式结果。

需要参数的动作接收具体参数类型。鼠标、键盘与语义调用共享动作可用性和所有者行为。不能直接修改语义值或控制器内部状态来绕过禁用动作。需要观察后续完成时，应保留返回的任务与提交身份。

## 等待实际需要的事实

`Driver.Wait` 在所有者上观察并注册订阅，然后在变化时重新判断。谓词表达具体要求：值改变、某个目标消失、某项任务结果已应用，或 written 水位达到相关 sequence。它不等待应用未来可能创建的全部任务。

取消请求、工作线程完成、UI 交付、发布接受和终端写入保持独立。被取消的视图不能复活卸载 target；后台操作成功时，UI 交付仍可能已经撤销。

分页可能省略当前页之前或之后的节点；要翻页，请读取 `Observation.Next`。如果缺页、深度限制或被截断的候选名称妨碍证明唯一性或缺失，`FindUnique` 返回 `ErrIncompleteObservation`。显示出来的名称前缀不能授权精确名称动作；只有描述被截断不会妨碍完整名称匹配。节点是否可用与输入、动作调用共用同一条 scene 规则。

发现动作时不会编造参数来运行其可用性回调。若 Go 参数类型允许 nil，实际调用可以传入 nil，与类型化动作相同。语义关系只指向当前场景中仍挂载的目标；目标可以位于本次查询的子树之外。
