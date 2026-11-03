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

## 类型化集合与控制器动作

List、Filter、Select、MultiSelect、Tree、Table 和 Settings 在已有元素上提供 `base.ActionReadItems`。传入 `CollectionQuery{ContentRevision: observed.ContentRevision, Start: start, Limit: limit}`，得到复制的 `CollectionPage[T]`、当前选择与总条数。零 limit 使用 `DefaultCollectionPageSize`，超过 `MaxCollectionPageSize` 的请求不可用。条目内部引用沿用控制器普通快照 API 的所有权合同。Tree 返回当前显示的 `Shown[T]`，Filter 返回排序后的匹配项，MultiSelect 返回包含选项值、标签和已选状态的 `ChoiceState[T]`。

`CollectionIndex{ContentRevision: observed.ContentRevision, Index: index}` 仅指向该修订顺序中的条目。替换条目、过滤、排序或展开分支都会使旧位置地址失效。无效或过期地址返回 `scene.ErrUnavailable`，不会钳制到另一条目。重试前先观察当前语义修订，再读取新页。

| 控制器 | 动作 | 参数 | 结果 |
| --- | --- | --- | --- |
| List、Filter、MultiSelect、Tree、Table、Settings | `scene.ActionSelect` | `CollectionIndex` | `CollectionSelection[R]`，其中 `R` 是页面条目类型 |
| Select | `scene.ActionSelect` | `T` | 已选值 `T` |
| Filter | `scene.ActionFill` | 模式 `string` | 当前模式 |
| MultiSelect | `scene.ActionSetValue` | 已选值 `[]T` | 按选项顺序返回的实际已选值 |
| Tree | `scene.ActionExpand`、`scene.ActionCollapse` | `CollectionIndex` | 当前分支展开状态 |
| Table | `base.ActionSort` | `SortOrder` | 脱离所有者存储的当前 `*SortOrder` |
| Table | `base.ActionClearSort` | `struct{}` | `struct{}` |
| Settings | `scene.ActionIncrement`、`scene.ActionDecrement`、`scene.ActionActivate` | `CollectionIndex` | 领域是否接受操作的 `bool` |
| Viewport、Transcript | `base.ActionScroll` | `ScrollRequest` | 当前行偏移 |
| Form | `base.ActionReadSubmission` | `struct{}` | 当前 `Submission`，包含其不透明 ID |
| Form | `base.ActionCancel` | 提交返回的 `SubmissionID` | 当前 `Submission` |

动作组合键盘和指针使用的同一所有者操作。重复设置相同 `SetSort` 顺序不改变状态；交互式 `SortBy` 仍切换升降序。MultiSelect 在变更前拒绝不可用值和超限选择，等价选项别名仍表示同一个选择。表单取消请求在应用结算前报告 `Started`；旧 attempt ID 不能取消后续提交。

Table 列索引的含义属于 `SetLess` 比较器。行控制器不拥有列 schema：排序可用性检查索引非负、存在比较器且行非空，调用方必须使用该比较器支持的列。

`Settings.Change` 返回 `scene.ActionResult[bool]`：`Value` 表示是否接受输入，`State` 由领域所有者报告实际结果。到达值边界但仍接受按键时返回 `Value: true, State: scene.Unchanged`。Settings 不复制领域值，也不根据输入是否被消费猜测值是否改变。

## 编辑器的插入位置

多行编辑器启用 `text.LayoutConfig.ReserveCaret`。逻辑行恰好填满末行时，投影添加空续行容纳末尾插入位置。高度、滚动、垂直移动、命中和选区共用该不可变投影，行号槽和缩放同样遵守此映射。普通文本布局仅在明确请求这项编辑策略时才改变被动行数。

## 富内容行为

`Transcript.AddDocument` 只连接一次 `StreamDocument`。此后的变更、EOF 准备、部分结果策略、发布准入、退休和订阅清理，由所属对象推进。调用方无需重复 Feed/Changed/Finish/Commit 协议。

`Selection` 存储相对内容的逻辑端点；`Scroll` 拥有阅读意图；`Sticky` 拥有可固定内容身份。Transcript 的帧候选组合它们的投影，绘制不推进这些状态。`Scan` 是有界同步搜索，可由调用方作为作用域工作调度，并仅接受匹配内容修订的结果。

公开控制器命令在整个变更完成后，通过同一个元素发布失效通知。复合操作在内部组合原子能力，由外层完整操作统一发布；调用方无需维护第二条通知路径。没有改变值的命令不会推进模型修订。

Transcript 滚动的是完整视口。固定标题覆盖顶部行，不缩小滚动范围或翻页步长。自定义皮肤从 `TranscriptView.BodyFrom` 开始绘制 `TranscriptView.Body`；这个正文起点包含被覆盖的行，因此与 `Scroll.Offset` 不同。指针映射和阅读锚点消费同一投影。从缩小后的正文起点重新选择标题，会在章节边界形成循环布局决策。

## 大数据集

`VirtualWindow.Resolve` 在绘制前的所有者阶段显式运行。`VirtualSource` 提供数据集修订、稳定 key 与纯高度测量。窗口仅保留已实例化元素和有界测量；key 留在窗口内时复用同一元素。离开窗口会卸载 target；之后重新出现拥有新的挂载生命周期。

所有相关高度已知前，总高度明确标为估计。元数据索引与保留的测量记录受 `MaxItems` 限制，键存储受 `MaxKeyBytes` 限制，每次解析的新测量工作受 `MaxMeasurementsPerResolve` 限制。同一源修订和宽度下，滚动保留已知高度，前缀位置不会退回估计值。挂载元素仅保留可见与预取窗口。[历史与虚拟化](../guides/history.md) 说明归档内容的独立所有权。

未配置 Build 时，虚拟窗口可以只测量范围，不挂载行，scene 布局也只描述真实子项。高度估算被修正时，原始滚动请求保持不变；最终 offset 从计算出的范围读取。

## Transcript 搜索

先保留实际搜索的布局：`source := transcript.Layout()`。从 `source.Rows(...)` 计算匹配，再调用 `transcript.SetMatches(source, matches)` 并处理返回错误。宽度变化、正文替换或切换 transcript 都可能让物理行列失效。收到 `base.ErrStaleMatches` 时，取当前布局重新计算。自定义绘制器应向 `MatchesIn(source, from, to)` 和 `CurrentMatch(source)` 传入本次正在绘制的布局。
