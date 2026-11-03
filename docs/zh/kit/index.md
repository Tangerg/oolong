---
title: 外观与组合
description: 应用主题或替换皮肤，同时保留行为和动作身份。
contentType: Conceptual
---

# 外观与组合

语言：[English](../../kit/index.md) | 简体中文

## 行为之上的默认外观

Kit 提供主题、字形选择、被动内容块和常用复合控件。它组合 base 控制器与 scene group，不另建焦点路由、选择模型或语义注册表。平台 → Base UI → shadcn 式层次由普通、可版本化 Go 包表达。

## 稳定的行为部件

交互组件使用显式 `Config` 创建。`Controller()` 返回实际行为所有者。`Element()` 返回其 scene 挂载元素；包含多个实际部件的复合组件则返回场景 group。重复调用不会创建新元素。

构造后始终通过指针持有 live 皮肤。已安装的控制器回调保留该外观所有者；复制结构体会让副本的可编辑字段与真正被测量、绘制的对象分离。这些类型声明不可复制契约，由 `go vet` 检查。需要独立组件时，使用 Config 重新构造；被动内容和不可变主题快照仍保留普通值语义。

自定义皮肤可以在挂载前填入 base 元素的绘制与测量槽。挂载期间修改这些槽会被拒绝。Theme 和 glyph 输入只改变外观，不能决定字段焦点、已提交值或动作可用性。

宿主字形能力只解析一次。需要截断标记时，将 `glyphs.Ellipsis` 传给 `ButtonConfig.Ellipsis`、`Paragraph.Ellipsis` 和 `Label.Ellipsis`。空标记表示静默截断。测量与绘制使用同一个标记；更改 Paragraph 的标记也会改变其布局缓存身份。

Kit 替换绘制槽后仍保留控制器的类型化集合动作和带修订的条目地址。`SettingsConfig.Change` 返回 `scene.ActionResult[bool]`，皮肤不会将消费按键擅自解释为值改变。详见[类型化控制器动作](../base/index.md#类型化集合与控制器动作)。

Box 标题和页脚的最小宽度、可用文字宽度与对齐起点，都由同一个边框标签内缩策略推导。截断与对齐始终保留角点及其相邻边框。

## 主题所有权

通过 `kit.NewThemeOwner(initial)` 创建应用的动态调色板，将 `owner.Scope()` 传入组件 Config，并在 UI owner turn 调用 `owner.Set(next)`。所有者使已挂载消费者失效重绘，不重建元素或控制器。`scope.With(kit.ThemeOverride{...})` 创建不可变子树分支；`Appearance` 提供实例覆盖。解析顺序为应用 → 祖先 scope → 当前 scope → 实例。

覆盖字段使用指针，显式零样式可恢复终端默认值；构造时复制指针指向的值。组件也可接收静态 `Theme`；非零 Theme 与 Scope 不能同时提供。展示期间或存在未提交帧时，调色板修改会被拒绝，与控制器修改共用纯度边界。

适配终端的中性色角色使用具名对比策略。选区、新增内容和删除内容即使当前比例相同，也各自拥有独立语义策略；上下文和分割线分别派生自弱化文字与边框角色。

## Transcript 示例

`kit.NewTranscript(TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})` 返回皮肤与错误。通过 `skin.Controller().AddDocument(document)` 添加源文档，再挂载 `skin.Element()`。控制器拥有阅读、选择、固定标题与保留策略；kit 根据同一份候选布局绘制选区、匹配标记与固定标题。

渲染器不会因为内容滚出屏幕就让其退休，主题也不会最终化流。参见带测试的[流式示例](https://github.com/Tangerg/oolong/tree/main/examples/streaming)和[流式指南](../guides/streaming.md)。
