---
title: 构建代理界面
description: 让人工输入和自动化共享类型化动作、可用性和结果生命周期。
contentType: How-to
---

# 构建代理界面

语言：[English](../../guides/agent.md) | 简体中文

## 从真实应用开始

运行 `go run ./examples/agent`，阅读 [agent 示例](https://github.com/Tangerg/oolong/tree/main/examples/agent)。领域任务、工具事件和持久化仍是应用策略。StreamDocument 拥有内容语义，Transcript 拥有阅读与保留，运行时拥有工作交付与终端输出。

## 观察不产生变更

`automation.NewDriver(DriverConfig{Root: runtime.Scene(), Owner: runtime.Dispatcher()})` 将查询和动作调度到 UI 所有者。应在场景挂载后创建。`Observe` 读取现有语义投影，不启动绘制、不填充隐藏内容、不创建后台工作。

`scene.Query` 限制深度、节点数量和文本。分页绑定模型修订，拒绝混合修订的后续页面。敏感值会被隐藏。`Progress` 分别报告模型、已准备布局和已写入帧进度；观察到模型不代表它已经出现在终端上。

## 定位并调用唯一当前目标

无参数动作可使用 `driver.Locate(query, scene.Match{Key: "submit"}).Invoke(ctx, scene.ActionSubmit, nil)`。Locator 保存意图，在同一个所有者轮次内解析唯一当前匹配并调用。缺失、歧义、截断、已卸载和被模态阻挡，都是显式结果。

需要参数的动作接收具体参数类型。鼠标、键盘与语义调用共享动作可用性和所有者行为。不能直接修改语义值或控制器内部状态来绕过禁用动作。需要观察后续完成时，应保留返回的任务与提交身份。

## 等待实际需要的事实

`Driver.Wait` 在所有者上观察并注册订阅，然后在变化时重新判断。谓词表达具体要求：值改变、某个目标消失、某项任务结果已应用，或 written 水位达到相关 sequence。它不等待应用未来可能创建的全部任务。

取消请求、工作线程完成、UI 交付、发布接受和终端写入保持独立。被取消的视图不能复活卸载 target；后台操作成功时，UI 交付仍可能已经撤销。
