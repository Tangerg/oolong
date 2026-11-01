---
title: 组合行为与外观
description: 使用稳定的场景部件，独立选择外观与交互策略。
contentType: How-to
---

# 组合行为与外观

语言：[English](../../guides/components.md) | 简体中文

## 从带测试的组合开始

运行 `go run ./examples/picker`，阅读 [picker 示例](https://github.com/Tangerg/oolong/tree/main/examples/picker)。应用选择数据与产品行为；base 拥有选择、导航和动作；kit 提供外观；scene 拥有实际结构。

程序化修改使用控制器的类型化操作。例如，`base.NewButton(ButtonConfig)` 通过其元素暴露唯一激活动作，表单暴露值与提交操作。不要为了让动作可发现，在皮肤里再创建第二个语义动作。

## 回执归签发它的所有者

表单调用 `Submit(id)` 时传入不透明的 `base.SubmissionID`。同步成功返回 `scene.Applied`，已接受的异步任务返回 `scene.Started`。回调返回后，在所有者回合通过 `form.Settle(id, outcome)` 交付结果。其他表单、已被取代的提交以及仍在执行提交回调的请求都不能使用该回执结算。

编辑器的元素快照保存捕获时的文本；即使文档已修改，仍可用 `element.Text()` 读取旧内容。关联的业务数据以不透明的 `base.ElementID` 为键；只有共享签发该标识的 `EditSession` 的编辑器能用它操作元素。调用 `tabs.SetRule(glyph)` 会同时更新分隔线和它占用的空间，空字符串移除该行。

## 装配实际部件

`scene.Group` 接收带 key 的 `scene.Item` 子项与布局函数。覆盖层使用 `scene.Overlay` 提供实际挂载与模态边界。绘制、焦点、路由、生命周期和语义观察由这些相同部件决定。

创建一次并保留组件实例。重复调用 `Element()` 是安全的；每次绘制都重建控制器，会丢失身份与手势。在 `Prepare` 前的所有者阶段更新 group 成员，然后绘制稳定的结果树。

## 替换皮肤

保留 base 控制器，在未挂载元素上设置绘制或测量槽。使用 core 的 text、grid、layout 原子能力绘制传入的 `scene.Frame`。从控制器读取值与状态。绘制使用候选几何，事件通过 `scene.Event.Read` 读取到达帧投影。

[Base 参考](../base/index.md)与 [kit 参考](../kit/index.md)定义两侧职责。[State 示例](https://github.com/Tangerg/oolong/tree/main/examples/state) 展示显式所有者更新与归一化，不保留竞争性的 accessor 存储。

若外观只属于某一次组合，在子关系的 `scene.Item.Paint` 上提供画法；移出子项后，元素自己的画法仍然有效。若一个控制器需要同时替换多个兄弟组的子项，父组可用 `CheckChildItems` 和 `SetChildItems` 对直属子组整体预检、一次安装。

`state` 示例包含四个自有草稿：提交时归一化，拒绝时保留草稿；另有 `ConfirmControl` 以同一领域所有者与外部重置动作共享答案，并演示动态 ThemeOwner 与 ThemeScope 组合。
