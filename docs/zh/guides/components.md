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

## 装配实际部件

`scene.Group` 接收带 key 的 `scene.Item` 子项与布局函数。覆盖层使用 `scene.Overlay` 提供实际挂载与模态边界。绘制、焦点、路由、生命周期和语义观察由这些相同部件决定。

创建一次并保留组件实例。重复调用 `Element()` 是安全的；每次绘制都重建控制器，会丢失身份与手势。在 `Prepare` 前的所有者阶段更新 group 成员，然后绘制稳定的结果树。

## 替换皮肤

保留 base 控制器，在未挂载元素上设置绘制或测量槽。使用 core 的 text、grid、layout 原子能力绘制传入的 `scene.Frame`。从控制器读取值与状态。绘制使用候选几何，事件通过 `scene.Event.Read` 读取到达帧投影。

[Base 参考](../base/index.md)与 [kit 参考](../kit/index.md)定义两侧职责。[State 示例](https://github.com/Tangerg/oolong/tree/main/examples/state) 展示显式所有者更新与归一化，不保留竞争性的 accessor 存储。

`state` 示例包含四个自有草稿：提交时归一化，拒绝时保留草稿；另有 `ConfirmControl` 以同一领域所有者与外部重置动作共享答案，并演示动态 ThemeOwner 与 ThemeScope 组合。
