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

初始设置放入组件的 `Config`。后续通过 `SetLabel`、`SetRows`、`SetLook`、`SetKeys` 等命令修改控制器，并通过对应的 `Label()`、`Rows()`、`Look()`、`Keys()` 读取。命令检查场景的变更阶段，并使对应元素失效，因此标签、几何与代理观察不会绕过模型修订而改变。呈现期间或帧候选尚未结算时，修改会在设置改变前被拒绝。复合控制器将共享设置交给已有所有者：Text 使用内部 Editor 的编辑设置，Table 使用内部 List 的行绘制、循环导航与绑定。

使用 `NewCompletion(CompletionConfig)`、`NewList(ListConfig[T])`、`NewSettings(SettingsConfig[T])`、`NewSticky(StickyConfig)` 提供初始设置，替代带字段的控制器字面量；零值控制器仍可使用。`Completion.CurrentToken()` 在只读借用中查询配置的 token 来源，返回 token 及其是否可用；`SetCurrentToken` 替换该来源。

`Group` 和 `Overlay` 始终是 scene 机制。对话框焦点策略、标签激活和列表选择复用这些机制，不另建挂载或路由树。更换外观保留行为部件的元素身份与动作绑定。

## 类型化集合与控制器动作

List、Filter、Select、MultiSelect、Tree、Table、Settings 和 Completion 在已有元素上提供 `base.ActionReadItems`。传入 `CollectionQuery{ContentRevision: observed.ContentRevision, Start: start, Limit: limit}`，得到复制的 `CollectionPage[T]`、当前选择与总条数。零 limit 使用 `DefaultCollectionPageSize`，超过 `MaxCollectionPageSize` 的请求不可用。条目内部引用沿用控制器普通快照 API 的所有权合同。Tree 返回当前显示的 `Shown[T]`，Filter 返回排序后的匹配项，MultiSelect 返回包含选项值、标签和已选状态的 `ChoiceState[T]`。Completion 返回 `Candidate`，其匹配偏移切片与所有者存储分离。

`CollectionIndex{ContentRevision: observed.ContentRevision, Index: index}` 仅指向该修订顺序中的条目。替换条目、过滤、排序或展开分支都会使旧位置地址失效。无效或过期地址返回 `scene.ErrUnavailable`，不会钳制到另一条目。重试前先观察当前语义修订，再读取新页。

| 控制器 | 动作 | 参数 | 结果 |
| --- | --- | --- | --- |
| List、Filter、MultiSelect、Tree、Table、Settings、Completion | `scene.ActionSelect` | `CollectionIndex` | `CollectionSelection[R]`，其中 `R` 是页面条目类型 |
| Select | `scene.ActionSelect` | `T` | 已选值 `T` |
| Completion | `scene.ActionActivate` | `CollectionIndex` | 已接受的 `Candidate` |
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

Completion 激活动作在一次操作中接受地址指定的候选，不依赖当前选择。后续 offer 会使旧地址失效，因此自动化不需要可能跨越候选版本的“先选择、再接受”流程。物理 Enter 仍接受当前候选。候选已关闭或编辑位置已过期时，所有集合动作均不可用。

展开或收起其他树分支时，当前选择仍指向同一节点。收起当前节点的祖先时，选择转移到该祖先。可见列表独占选择状态；分支转换把节点身份映射到新行，并在通知订阅者前撤销待处理按键序列。

Button 和 Confirm 即使在后续步骤返回错误时，也会发布已接受的 `Applied` 或 `Started` 结果。物理输入还会发布其保留的 UI 错误，包括值未改变但旧错误被清除的情况。`Button.Press`、`Confirm.Request` 与语义动作直接返回错误，并保留上一次物理输入的反馈。Confirm 将同步领域通知延迟到请求结果和 UI 反馈结算后统一发布，答案始终仅由领域所有者持有。

UI 反馈属于最近一次 UI 请求。回调同步发起新请求后，较早请求的返回不能覆盖新反馈。Form 在既有提交身份内按相同规则记录取消反馈，成功重试会清除之前的取消错误。类型化的程序请求与语义请求仍直接返回自己的结果和错误。

`Table.SetSortColumns` 接受列所有者提供的查询，由列索引取得纯三路比较器。返回 nil 表示该列不可排序，直接排序和语义动作都在同一边界拒绝它。列是否受支持和如何比较来自同一个查询，Table 不再维护另一份列数或 schema。支持的列或比较规则变化时应替换查询；活动排序列被移除时，清除排序描述并保留当前行序。

Form 即使同时收到错误，也保留回调报告的 `Applied` 或 `Started` 结果及提交 ID。`Started` 继续等待结算，其伴随错误可通过 `Submission` 和 `Error` 查询。显式取代旧提交时返回 `ErrSubmissionStale`，但不抹掉旧回调已经接受的效果。

`Form.Set` 在改变字段关系前校验完整替换。替换成功会撤销表单旧的延迟快捷键和未完成键序列，即使当前焦点字段仍然挂载。被拒绝的替换保留原有关系与键序列。发起提交会先发布新的空闲尝试和已清除的反馈，再进入外部验证。即使验证异常退出，观察者仍能看到旧尝试已经结束；随后返回的验证错误另行发布自己的反馈。

按键序列通过 `Matcher.Handle` 的生命周期谓词捕获 `scene.KeyboardLease`。键盘路由内的接收者在焦点移动于其后代之间时保留凭证；显式转交给路由外接收者的输入（例如补全弹层）借用当前输入起点的生命周期。没有焦点时，活动模态层或 root 内容提供这个起点。离开作用域、卸载或关闭 root 会永久撤销旧凭证，包括已排队的 resolver；焦点回来不会恢复旧输入。未挂载控件可以接收新的本地输入，直到下次挂载。独立 matcher 可传 nil，由直接调用方管理生命周期。

`SettingsConfig.Change` 回调返回 `scene.ActionResult[bool]`：`Value` 表示是否接受输入，`State` 由领域所有者报告实际结果。到达值边界但仍接受按键时返回 `Value: true, State: scene.Unchanged`。通过 `Settings.SetChange` 替换回调。Settings 不复制领域值，也不根据输入是否被消费猜测值是否改变。

## 编辑器的插入位置

`base.Caret{Line, Byte}` 表示逻辑行内的字节偏移；`base.Point{Row, Col}` 表示布局中的终端单元格。折行和 Unicode 文本都保持这两个单位明确。垂直移动保留希望到达的单元格列，包括零列；遇到原子元素时按移动方向吸附到边界，布局查询不会重置该意图。

多行编辑器启用 `text.LayoutConfig.ReserveCaret`。逻辑行恰好填满末行时，投影添加空续行容纳末尾插入位置。高度、滚动、垂直移动、命中和选区共用该不可变投影，行号槽和缩放同样遵守此映射。普通文本布局仅在明确请求这项编辑策略时才改变被动行数。

## 富内容行为

`Transcript.AddDocument` 只连接一次 `StreamDocument`。此后的变更、EOF 准备、部分结果策略、发布准入、退休和订阅清理，由所属对象推进。调用方无需重复 Feed/Changed/Finish/Commit 协议。

`Transcript.Close` 释放自己持有的已提交几何、呈现快照、内容条目与订阅，即使后续不再绘制也完成释放。调用方此前保留的布局继续保持不可变，并按自己的生命周期保留内容。

`Selection` 存储相对内容的逻辑端点；`Scroll` 拥有阅读意图；`Sticky` 拥有可固定内容身份。Transcript 的帧候选组合它们的投影，绘制不推进这些状态。`Scan` 是有界同步搜索，可由调用方作为作用域工作调度，并仅接受匹配内容修订的结果。

`ScrollLayout.Reveal`、`Resize` 与 `Anchor` 只细化候选几何，不改变末尾跟随意图。该转换属于显式的 `Scroll` 命令，取消候选帧也不能改变它。晚到的发布拒绝在保存错误后使 Transcript 场景元素失效，同时保留原有内容修订与选区。

公开控制器命令在整个变更完成后，通过同一个元素发布失效通知。复合操作在内部组合原子能力，由外层完整操作统一发布；调用方无需维护第二条通知路径。没有改变值的命令不会推进模型修订。

Transcript 滚动的是完整视口。固定标题覆盖顶部行，不缩小滚动范围或翻页步长。自定义皮肤从 `TranscriptView.BodyFrom` 开始绘制 `TranscriptView.Body`；这个正文起点包含被覆盖的行，因此与 `Scroll.Offset` 不同。指针映射和阅读锚点消费同一投影。从缩小后的正文起点重新选择标题，会在章节边界形成循环布局决策。

## 大数据集

`VirtualWindow.Resolve` 在绘制前的所有者阶段显式运行。`VirtualSource` 提供数据集修订、稳定 key 与纯高度测量。窗口仅保留已实例化元素和有界测量；key 留在窗口内时复用同一元素。离开窗口会卸载 target；之后重新出现拥有新的挂载生命周期。

所有相关高度已知前，总高度明确标为估计。元数据索引与保留的测量记录受 `MaxItems` 限制，键存储受 `MaxKeyBytes` 限制，每次解析的新测量工作受 `MaxMeasurementsPerResolve` 限制。同一源修订和宽度下，滚动保留已知高度，前缀位置不会退回估计值。挂载元素仅保留可见与预取窗口。[历史与虚拟化](../guides/history.md) 说明归档内容的独立所有权。

未配置 Build 时，虚拟窗口可以只测量范围，不挂载行，scene 布局也只描述真实子项。高度估算被修正时，原始滚动请求保持不变；最终 offset 从计算出的范围读取。

## Transcript 搜索

先保留实际搜索的布局：`source := transcript.Layout()`。从 `source.Rows(...)` 计算匹配，再调用 `transcript.SetMatches(source, matches)` 并处理返回错误。宽度变化、正文替换或切换 transcript 都可能让物理行列失效。收到 `base.ErrStaleMatches` 时，取当前布局重新计算。自定义绘制器应向 `MatchesIn(source, from, to)` 和 `CurrentMatch(source)` 传入本次正在绘制的布局。
