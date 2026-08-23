---
title: 先行者
description: 审视 Oolong 研究的终端 UI 系统，以及它采用或拒绝的设计选择。
contentType: Conceptual
outline: deep
---

# 先行者：六个终端 UI 家族，以及该从它们那里拿什么

语言：[English](../prior-art.md) | 简体中文

状态：一份附带取舍结论、持续更新的源码审计。它与
[architecture.md §5](architecture.md#5-从前端与-flutter-体系里拿什么)
互为姊妹篇；后者用同样的方法看浏览器和 Flutter：把值得学习的职责与承载它的
机制分开，并明确说出哪一半不采用。

这里没有任何一项自动成为承诺。每个候选项仍然必须通过
[§7.1](architecture.md#71-一个包必须配得上它的名字)和
[§15](architecture.md#15-架构如何生长)的关卡，其中有几项今天仍然过不了。
一份调研一旦变成待办清单，就已经不再是调研。

## 读了什么，读到什么程度

| 项目 | 阅读版本 | 语言 | 是什么 |
| --- | --- | --- | --- |
| [agentui](https://github.com/minoism/agentui) | `ec0414e`，2026-07-28 | Go，约 245 个文件 | 一个 agent CLI，与本仓库有共同祖先。 |
| grok-build | `780d138`，2026-08-03 | Rust，约 2,500 个文件 | 已交付的产品；`xai-ratatui-*` crates 是它的终端层。 |
| [opentui](https://github.com/anomalyco/opentui) | `b55f125`，2026-08-05 | TypeScript + Zig | 一个带 React、Solid、SSH 和 3D 包的终端 UI 平台。 |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | `6fb1f47`，2026-08-04 | Go | Charm 的运行时与渲染器。 |
| [bubbles](https://github.com/charmbracelet/bubbles) | `8cea431`，2026-08-04 | Go | Charm 面向行为的组件目录。 |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | `5696b28`，2026-07-20 | Go | Charm 的字符串样式与布局层。 |
| pi-tui | `7df73a00c`，2026-07-24 | TypeScript | “最小化的差量渲染终端 UI 框架”。 |
| [Codex](https://github.com/openai/codex) | `e4e040881`，2026-08-03 | Rust | 一个基于 Ratatui 自行实现 TUI 的 agent CLI。 |

第一轮是调研。2026-08-09 这一轮是对相关运行时、渲染器、keymap、
editor、completion 与 terminal lifecycle 实现的源码审计。下文引语来自这些
来源；关于**本仓库**的每一项判断都对照代码核验过。

这些项目会继续变化。没有日期的对比，会在无人察觉时停止成立；本仓库已经犯过
一次这种错误，并记录在 [ROADMAP.md](https://github.com/Tangerg/oolong/blob/main/ROADMAP.md) 里。

## 2026-08-09 的可执行决策

本轮选中并完成了三个纵向切片。它们小到可以独立落地，又完整到每个批次结束时
已经可用。

| 切片 | 吸收的职责 | 守住的边界 |
| --- | --- | --- |
| 精确匹配/前缀组合键 | `keymap.Matcher` 拥有单个 reader 的序列状态，并调用一个由调用方提供的 resolver；`Runtime.After` 是标准的 owner-goroutine resolver | `keymap` 不导入 `program`、不拥有时钟、不生长第二棵焦点树 |
| cursor 形态与闪烁 | cursor 外观是 `grid` 已交付帧状态的一部分，像位置和可见性一样做 diff | editor 可以选择外观，但 terminal escape 状态不进入 `headless` 行为 |
| terminal 原生任务进度 | `term` 拥有 OSC 9;4 编码、keepalive 与恢复；`program.Session` 暴露一个可选 host 能力 | 它与绘制在界面内的 `kit.Progress` 保持分离，应用任务 policy 留在下游 |

每个切片都已经在自己的层级有单元测试、有端到端消费方、在适用时有空闲线路
测试，并有 lifecycle 恢复测试。`examples/keys` 消费序列消歧，`Editor` 通过帧发布
cursor 外观，`examples/agent` 把原生进度用于真实任务生命周期。只有公开类型还不算完成。

本轮还关闭了第一轮的一个假缺口。`Container` 与 `Stack` 本来就是 scope 系统：
先把 event 交给焦点子项，子项拒绝后再下落给父级，已交付帧拥有 pointer 身份与
几何。中央 keymap layer registry 会复制这棵树，并制造两个“哪个 scope 拥有事件”的
答案。因此它被否决，而不是暂缓。

## 1. opentui/keymap 回答了本仓库已经写下的一项限制

这个切片落地前，`core/keymap` 自己写明了上限：

> 一个序列如果是另一个绑定的真前缀，就不可达：没有计时器驱动查找时，只要更长
> 的绑定仍可能到来，就不能选择较短的绑定。因此更长序列优先。

`@opentui/keymap` 没有这个上限：

> 可编程的精确匹配/前缀消歧（例如 `g` 与 `gg`），带 `runExact`、
> `continueSequence`、`clear`，以及延迟的 `AbortSignal` + `sleep` 决策；并附带
> Neovim 风格的超时解析器。

值得拿的思想不是“加一个超时”，而是：**消歧是调用方提供的 policy**，超时 resolver 只是
它的一种实现。

这个形状无需反转依赖边就能解决问题。依赖图禁止 `program` 穿过 components
反向触达 `keymap`，也禁止 `headless` 触达 `program`，所以 **matcher 不能拥有 runtime
timer**。取而代之，`Map` 接收一个与 `Runtime.After` 同形的 resolver。matcher 把可取消的
精确 action 交给它，runtime 再把 action 调度回界面 owner。`keymap` 仍然不知道 runtime，
也不知道 goroutine。

还有一条更小的经验值得吸收：序列状态应该属于一个行为对象。旧 `Pending` 只暴露
存储，十三个组件却重复同一套 lookup/dispatch 过程。`Matcher` 拥有推进、取消与
dispatch，因此读取 map 只剩一份实现。

**不采用 scoped layer 与 layer diagnosis。** OpenTUI 需要 registry，是因为它的 binding 可以脱离
renderable ownership 独立存在。Oolong 的 map 由组件树内的 widget 拥有。`Container` 与 `Stack`
已经提供 focus-within、优先级与下落，帧事务也已经阻止输入几何与呈现状态分裂。
再加 layer graph，会让 focus 与 key scope 有机会互相矛盾。为一张我们不应拥有的图做诊断，
是为抽象辩护抽象。

**不采用。** opentui 的 keymap 罗列了十二项主能力。其余部分包括可插拔的绑定语言
（解析器、展开器、变换器、命令解析器、字段编译器）、可注册 schema、带订阅通知的
响应式 matcher，以及 React 和 Solid 绑定。
[§16](architecture.md#16-明确否决的设计)已经否决通用 signals/observables
框架，[§12 规则 8](architecture.md#12-go-api-规则)要求配置与问题成比例。
调用方提供的 resolver 与有状态 matcher，才是诚实的比例。

## 2. opentui/ssh 证实了本仓库已经实现的 host 形状

> `@opentui/ssh` 把一个传入的 SSH session 变成接线完整的 OpenTUI
> `CliRenderer`；输入/输出是 SSH channel，尺寸跟随客户端 PTY。……这个包与
> renderer 无关：只依赖 `@opentui/core`，从不依赖 `@opentui/react` 或
> `@opentui/solid`。

这个形状现在已经在这里实现：SSH channel 藏在 `program.Host` 后面，独立成模块，
永远不进 `core`。应用与 `charm.land/ssh` 继续拥有认证、banner、监听、host key、
连接策略、日志和退出状态。Oolong 只拥有已接受 PTY 的输入解码、窗口流、帧 writer
与终端模式。这是清单所指向的更窄边界，不是第二套 SSH server。

变化在于实现这个边界的成本。最初考虑它时，`program.Host` 有十六个方法，大多数
实现要对其中十几个回答“不支持”。现在必需方法只有三个；可选能力每次只增加一个
方法；`EventSource.Err` 报告传输失败；`FrameWriter` 由消费方定义。SSH host 因而是
一个小模块，而不是一个大工程。

这也关系到
[§13.1 条件 3](architecture.md#131-v1-的兼容性边界)：它要求两个独立维护的
非示例应用。SSH server 是候选之一，也是本文唯一的候选。

## 3. pi-tui：两个小型编辑器行为

**kill ring，现已实现。** `headless.Editor.Yank` 原先只从一个槽里“放回最后一次剪掉
的文本”。现在它拥有一个有界 ring，并实现了 pi-tui 中观察到的行为：

> 连续 kill 可以累积为一个条目。支持 yank（粘贴最近条目）和 yank-pop（轮换到更早
> 的条目）。

私有 ring 接收 `prepend`，因此向后删除会累积到当前条目的前面，向前删除会累积到
后面——这正是让累积感觉正确而不只是“存在”的细节。`YankPop` 只替换紧邻其前的
yank，任何插入其间的语义动作都会结束该序列。ring 拥有自己的字符串，最多保留
十六项。

**大段粘贴成为一个原子事物。** pi-tui 会把超过阈值的粘贴折叠成一个 marker，
编辑器把它视为单一 segment：

```text
[paste #1 +123 lines]
```

> ……paste marker 内部成为单一原子 segment。这使 cursor ……

**本仓库原本已经具备机制；现在 worked example 也已经存在。**
`headless.Editor` 的 element 与 `text.Mark` 提供原子行为，
[`examples/composer`](https://github.com/Tangerg/oolong/tree/main/examples/composer) 则把它用于 `input.Paste`。阈值、marker 文案
以及粘贴原文都由示例拥有，没有一项变成库 policy；这正是本节主张的边界。

### pi-tui 的差量渲染：最有意思的分歧

pi-tui 渲染为 ANSI 样式**字符串**数组，并按**行**做 diff：

> 1. **首次渲染**：输出所有行，不清除 scrollback
> 2. **宽度变化或变更发生在 viewport 上方**：清屏并完整重绘
> 3. **普通更新**：移动到第一条变化行，清除到末尾，绘制变化行

它有三项值得说清的性质：两项有利于本仓库，一项不利。

**单位决定工具。** 因为一行是带样式的字符串，pi-tui 需要 `visibleWidth`、
`truncateToWidth` 与 `wrapTextWithAnsi`——整族函数都在推理埋着转义序列的文本。
绘制到 cell 就完全不需要这族工具：宽 grapheme、hyperlink 和被绘制的 image region
都是结构化值，而不是必须重新解析的字节。这与 [ROADMAP](https://github.com/Tangerg/oolong/blob/main/ROADMAP.md) 对行字符串
diff 的论证相同；pi-tui 是另一边的当代实例。

**策略 3 比听起来更粗。** “移到第一条变化行，清到末尾，再渲染”会重写第一处变化
以下的每一行。两处相距很远的变化，要付出它们之间整段距离的成本；cell diff 只写
两个区域。

**策略 2 检测的是本仓库让它无法表达的状态。** pi-tui 跟踪之前的 viewport 顶部，
并在第一次变化位于其上方时强制完整重绘：

> 差量渲染只能触达真正可见的内容。如果第一处变化位于之前的 viewport 上方，就需要
> 完整重绘。

这对 inline 界面是真风险：已经滚进 scrollback 的行无法再次寻址，对它们做 diff 会
写到别处。`grid.Inline` 无法进入这个状态，因为 block 的高度不是 size：

> 高度是上限而不是尺寸：它是终端能让出的空间，block 实际只占界面画到的高度。

永远不超过终端可见范围的 block 没有会丢失的顶部。让事物有界比检测溢出更好，这也
是 §3.2 对内存所做的选择。

**pi-tui 策略真正买到而本仓库没有的东西。** 从第一处变化清到末尾具有**自愈性**。
cell diff 只有在程序的屏幕模型正确时才正确；任何其他终端写入——游离的日志行、
子进程、通知——都会悄悄让模型失同步，之后每一帧都错，直到某件事强制重绘。本仓库
会在自己知道的时刻这样做：resize、handover、颜色深度变化。它无法回答一次自己从未
看见的写入。pi-tui 每帧都从第一条变化行向下修复，并以更多字节为代价。

这是不对称，不是缺陷，所以记录在这里而不是争辩掉。是否值得回答，要等真实程序
证明未知写入确实发生；目前没人提供这种证据。

**真正值得拿的不是渲染。** pi-tui 的测试终端是 emulator：

> `VirtualTerminal`——用于测试（使用 `@xterm/headless`）

本仓库原本已经能在无终端情况下驱动程序（`core/programtest`），也能通过真实 PTY
驱动（`ptytest`），但两者都不能回答**终端最后显示了什么**：前者断言 frame，后者
断言 bytes。`ptytest.Screen` 现在提供第三种形状：它增量应用两个 Oolong renderer
发出的 cell 文本、移动、擦除、有界滚动、SGR、OSC 与 mode 语法，并暴露固定尺寸的
cell 文本用于断言。它复用 `core/ansi` 与 `core/text`，不再生一套转义扫描器或
grapheme 宽度实现。

边界必须同时说清，因为
[§16](architecture.md#16-明确否决的设计)拒绝终端 emulator。该模型内部保存
写入位置与 margin，只因 renderer 输出用它们寻址 cell；两者都不是公开终端状态。
它不回答 query、不解码输入、不拥有 alternate buffer、不保留 scrollback、不暴露
cursor visibility，也不解释 device-control painter。不支持的完整序列会明确失败。
这些足够回答 renderer 留下了什么 cell 文本，又刻意不足以运行任意终端程序。

## 4. grok-build：反例比借鉴更有价值

`xai-ratatui-inline` 导出两种 resize 策略：

```rust
resize_purge_rerender,
resize_viewport_height,
```

[§3.2](architecture.md#32-活动界面必须保持有界)直接否决第一种：

> 清除 scrollback 并重新发出保留历史的 resize 算法同样不兼容：它拿用户的终端历史
> 与应用的无界内存，换取几何确定性。

一个已交付产品走了另一条路。这不改变这里的决定——理由仍然成立，也是本仓库能够
承诺活界面有界的原因——但此前的决定没有任何反方。带名字的反例会让拒绝更坚实，
所以反例记录在本文，供 §3.2 指向。

该 crate 其余能力在这里都有对应物：`emit_to_scrollback`、
`split_into_line_segments`、`LinkSpan`、`with_synchronized_output`。

## 5. agentui 是应用，不是可借用的库

它自己的包文档已经说明：`btw`“建模 Grok 相关联的 /btw 旁问生命周期”，`catalog`
“建模 Grok 的设置与扩展浏览器”。`goal`、`todo`、`tasks`、`workflow`、`session`
都属同类。[§5.3](architecture.md#53-组件阶梯)把这些全部放在应用层：共享组件
可以提供表达产品语法的 dialog、list 与 editor，但不得给该语法命名。

它的 `scrollback` 是 xai-grok-pager 引擎的 Go 移植，覆盖范围与
`headless.Transcript` 相同。

有一个包包含通用核心。`media.Inspect(source, policy)` 在**不加载媒体**的前提下从
文本找出媒体引用，返回带字节范围和类型的 attachment，并对**拒绝项附上每个候选被
拒绝的原因**。它有 fuzz 测试。由此得到两个观察，只有一个关乎功能：

- `core/link`“找出文本里指向某处的东西”，只报告发现结果。它不能解释**为什么**一个
  看似 link 的东西未被当成 link。能解释拒绝的 detector 才便于人调试、程序报告。
- 拒绝 workspace 以外引用的路径政策，是特定应用的安全决策。agentui 把这条线画在
  `policy` 参数上，这是正确边界：它不属于负责 detection 的包。

## 6. 组件清单

前几节比较机制；本节比较目录。对组件库问“哪些东西不必自己写”是公平问题，并且有
可计数的答案。

| | 组件 | 说明 |
| --- | --- | --- |
| 本仓库 | 约 70 个行为类型（`headless`）之上的 25 个已绘制组件（`kit`） | 另有 `Theme`、`Glyphs`、`Border`、`Cell`、`Column`、`LineNumbers`、`TableLayout`、`Scrim`、`Printer` 等支撑值 |
| opentui | 20 个 renderable | `ASCIIFont`、`Box`、`Code`、`Diff`、`EditBuffer`、`FrameBuffer`、`Image`、`Input`、`LineNumber`、`Markdown`、`ScrollBar`、`ScrollBox`、`Select`、`Slider`、`TabSelect`、`Text`、`TextNode`、`Textarea`、`TextTable`、`TimeToFirstDraw` |
| bubbles | 14 | `cursor`、`filepicker`、`help`、`key`、`list`、`paginator`、`progress`、`spinner`、`stopwatch`、`table`、`textarea`、`textinput`、`timer`、`viewport` |
| pi-tui | 12 | `box`、`cancellable-loader`、`editor`、`image`、`input`、`loader`、`markdown`、`select-list`、`settings-list`、`spacer`、`text`、`truncated-text` |
| agentui | 7 个界面包 | `overlay`、`palette`、`completion`、`history`、`search`、`todo`、`tasks`——后两项是产品语法 |

计数是表里最不重要的部分。下面才是这里尚未以别名存在的内容。

### A. 通用行为，现已实现

这些能力在调研写成时完全缺失；它们都是带 appearance 的 behavior，并且在读过的项目
里至少有两个实现——这正是 [§7.1](architecture.md#71-一个包必须配得上它的名字)
要求的证据。它们现在构成一个已经完成的纵向切片，而不是永久目录愿望。

| 能力 | 谁有 | 本仓库的实现 |
| --- | --- | --- |
| **有界数值** | opentui `Slider` | `headless.Slider` 拥有边界、值的所有权、按键和已提交的拖动几何；`kit.Slider` 是其 appearance。`Progress` 仍是不同的只读比例。 |
| **行号** | opentui `LineNumberRenderable` | `text.Row` 携带逻辑行来源，`headless.RowGutter` 是共享接缝，`kit.LineNumbers` 同时装饰 editor 和 code block。 |
| **代码块** | opentui `Code` | `kit.Code` 组合复制后的 `text.Line`、换行、复制行与可选 gutter，且不依赖可选的 `highlight` 模块。 |
| **按内容定宽的列** | opentui `TextTable` | `kit.Cell` 把首选宽度和绘制放在一起；`Column.Size: layout.Measured(0, 0)` 测量最宽标题或 cell；`TableLayout` 在整帧复用结果。 |
| **设置列表** | pi-tui `settings-list`、agentui `catalog` | `headless.Settings` 在 `List` 上增加选中值动作；`kit.Settings` 提供按内容定宽的 label/value 行，应用数据与修改仍留在下游。 |

### B. 真实的产品组合，但边界继续留在下游

第二轮审计发现，早先的“一个共同 scope 问题”是错误抽象。scope 已经存在于组件树；
下面三项是附着在通用机制上的应用 policy。

| 产品组合 | 谁有 | 本仓库的边界 |
| --- | --- | --- |
| **带作用域的 command palette** | agentui `palette`（`Scope`、`Predicate`、`Registry`） | pane 在同一 subtree 中拥有自己的 `Commands` 与 palette。`Container`/`Stack` 决定哪个 subtree 接收输入；不需要、也不允许第二套 scope registry。 |
| **异步 completion source** | agentui 与 pi-tui 的 file、shell 和 reference provider | `Completion.Offer(token, candidates)` 继续是呈现接缝。应用拥有 I/O、取消，并在 offer 结果前验证 token/editor snapshot。 |
| **file picker** | bubbles `filepicker` | `Tree`、`Editor`、`Completion`、`Scroll` 与 `Selection` 已经提供机制。文件系统根、隐藏文件规则与遍历权限是应用安全 policy，不是共享 widget。 |

因此，`Commands[T]` 只索引可搜索的命令描述与一个由调用方拥有的不透明值。它不规定参数
形状，不存储由自己定义类型的执行回调，也不解析以斜杠开头的产品语言；应用自行选择 `T`
和输入语法。这样，registry metadata 与应用含义仍在一次注册中保持一致，又不会把执行 policy
或语法下沉到 `headless`。

`examples/composer` 与 `examples/agent` 是 `Completion.Offer` 的两个消费方，都能直接
产生由应用拥有的候选。通用 `Source` 会让组件开始 I/O、导入取消 policy，并成为 editor
状态的第二个 owner。agentui 的 generation check 与 pi-tui 的 text/line/column/request-id check
都是把验证留在 source 旁边的证据，而不是把 source 搬进组件的证据。

### C. 只增加数量，不增加能力

加入这些只会抬高数字，不会让人多做成任何事。

| 缺失项 | 谁有 | 为什么不加 |
| --- | --- | --- |
| Paginator | bubbles | 一个“3 / 7”指示器。 |
| Timer、stopwatch | bubbles | 一个 `time.Ticker` 加格式字符串。 |
| ASCII banner font | opentui `ASCIIFont` | 装饰。 |
| Spacer | pi-tui | `layout.Flex(1)`。 |
| Truncated text | pi-tui | 带 `Ellipsis` 的 `kit.Label`。 |
| ScrollBox、TabSelect、Input、Loader | opentui、pi-tui | `Viewport` + `Scroll`、`Tabs`、`Editor`、`Spinner`。 |

### 本节承受的张力

“组件越多越好”是一种目标，但不是本仓库的目标。§15 规定任何阶段都不为以后制造无人
使用的框架；§7.1 要求一个边界先有两个真实消费方；
[§16](architecture.md#16-明确否决的设计)已经拒绝为了数量加入 timer/stopwatch，
也拒绝把文件系统 policy 塞进 file picker。

这些规则并不真的与丰富目录冲突，只要守住分界：**足够通用且有两个消费方的行为进入
`headless` 与 `kit`，其余成为 worked example。** file picker 的 tree、filter 与
selection 属于前者，policy 属于后者；settings list 属于前者，具体 setting 属于后者。
A 组完全属于前者，因此无需导入产品语法就能实现。

顺序先做 A，因为五个完全造不出来的组件，比任意数量的已有能力别名更有价值。A 完成
以后，B 现在是已记录的应用边界，而不是框架 backlog。C 继续由需求驱动；若只是产品
组合，就属于 example。

## 7. Charm：借终端契约，不借执行模型

Bubble Tea v2 在每个 `View` 里明示了两项终端属性：

- `Cursor` 携带位置、形态与闪烁；
- `ProgressBar` 携带 normal、error、indeterminate、warning 状态与有界百分比。

OpenTUI 也独立地把 cursor style 当成 renderer 状态，pi-tui 也独立地使用 OSC 9;4
告诉 terminal window 工作正在进行。这不是“组件数量”能力，而是 cell grid 之外的
呈现状态；除非 session 明确更改或恢复，终端会在帧后继续保留它们。所以可执行决策
在 renderer/session 边界同时吸收两者。

两种 progress 的分开非常重要。`kit.Progress` 在界面内绘制比例，属于 layout 与 theme。
native progress 属于 window 或 taskbar，窗口被遮住时仍然有用，并且必须在 handover 与
close 时清除。两者无法互相实现，所以这不是重复 API。

Pi-tui 后来的 keepalive 修复暴露了一个仅有 encoder 会漏掉的生命周期细节：有些 terminal
会在任务仍活跃时让 OSC 9;4 过期。因此 Oolong 只在原生进度活跃时启动 ticker，在取得
handover watermark 之前暂停它，在重新取得所有权后重述最新值；进度清空时完全不拥有
timer。Keepalive 是 session 基础设施，不是应用计时器。

三项更大的 Charm 选择明确不引入：

- Bubble Tea 的 `Model -> (Model, Cmd)` loop 是一套 immutable-effect 语汇。Oolong 的 owner
  goroutine、`Dispatcher`、有界 `ByteIngress` 与具体 capability value 已经提供显式所有权；
  再加 `Cmd` 会制造第二套并发模型。
- Lip Gloss 样式化 ANSI string，因此必须再拥有 ANSI-aware 宽度、截断、拼接与分层定位。
  Oolong 把 grapheme、style、link 与 painted region 保持为结构化值，直到最后的 encoder。
  退回 styled string 会先丢失信息，再为恢复它付费。
- Bubbles 的 file picker 拥有文件系统遍历 policy。Oolong 保留可复用交互机制，并像
  第 6 节记录的那样，把文件系统权限留给应用。

Bubble Tea 的一次性 `Tick` 确实指向一项缺失的底层便利。keymap resolver 正好需要
一个在界面 owner 上运行的、可取消的一次性 callback，因此为这个具体消费方采用
`Runtime.After`。Timer/stopwatch widget 继续被否决：调度工作与选择产品如何展示时间，
是两种职责。

## 8. Codex：吸收终端行为，不吸收应用语法

Codex 在这里有价值，因为它是一个大型 agent 产品，其终端代码必须面对窄窗口、多个
色深、远程 session 与不同键盘协议的 terminal。它的 command palette、model picker、
审批文案与 agent 状态词汇仍是产品语法；下面六项底层职责不是。

**Markdown 表格把结构保留到宽度已知。** Codex 的 `markdown_render.rs` 保留带样式的
cell 并对 column 分类，然后才选择对齐网格或 key/value records。Oolong 现在也在正确
边界做这项决定：`markdown.Block` 保留表格 cell、alignment 与 style，再在 `Measure`/
`Draw` 中分配可读列；无法保持可扫描性的表格会转为带 label 的 records。解析阶段不再
冻结一份过宽文本，让后续 layout 只剩截断这一条路。

**Diff 内容会换行，而不会被丢掉。** Codex 先为 gutter 留出空间，再换行剩余的 styled
spans，并给 continuation row 一个空 gutter。`kit.Diff` 现在由同一份纯 width-aware
layout 同时负责测量与绘制。line number 在会饿死内容时让位，continuation row 则保留
sign、背景与缩进；不存在用 ellipsis 悄悄删掉 proposed change 的路径。

**键盘增强以 feature 协商。** Codex 处理 Kitty keyboard flags、`modifyOtherKeys`、
terminal-specific exclusion 与对称恢复。Oolong 的 transport-neutral 形状是
`input.KeyboardFeatures`：`term.Config.Modes` 根据真正被驱动 terminal 的环境推导准确
flags；SSH 使用 client 的 PTY 环境，而不是 server process。input package 命名解码后的
能力；只有 `term` 拥有 escape sequence 与兼容性判断。

**远程 clipboard 属于 client。** Codex 在 SSH 下经 terminal copy，把 OSC 52 原始输入
限制为 100,000 bytes，并把 tmux 视为单独的 forwarding 情况。Oolong 现在由
`clipboard.Channel` 拥有这份协议状态：编码、tmux passthrough、唯一未完成 read、answer
关联与过期。本地 terminal 与 SSH host 消费同一个 channel，因此 remote OSC answer 只会
在该 session 发起请求后成为 `input.Paste`。没有引入 native desktop clipboard package
与 WSL PowerShell fallback：它们是应用/平台集成，不是两个 host 共享的 terminal 协议。

**周围 terminal 参与 appearance。** Codex 根据 theme 与 colour level 解析 diff
background，而不是假定一个固定背景上的 truecolor。Oolong 保留显式的 `Dark` 与
`Light` palette；`Suited` 让正文继续使用 terminal 自己的 foreground，并由已报告 ground
推导 neutral surface、line、selection 与 scrim。semantic colour 保持稳定，
`grid.Depth` 只在最终 encoder 降级它们。组件因而只有一套 theme vocabulary，而不是每个
terminal depth 一套 palette。

**视觉回归是选择性的，也是有维度的。** Codex 的 snapshot 集中在窄/宽 table、换行
diff 与有代表性的完整 screen。Oolong 现在在 `examples/internal/visualtest` 中组合
`programtest` 与 `ptytest.Screen`：复杂 agent review 会在 44 和 90 列下检查；truecolor、
256、16 与 no-colour 运行必须得到相同文字几何，同时使用正确编码族。这不是 blanket
snapshot policy。状态转换仍用行为断言；golden 只守住少数“关系本身就是行为”的布局。

## 总结

| 来源 | 采用 | 不引入 |
| --- | --- | --- |
| opentui/keymap | 调用方提供的精确匹配/前缀消歧；每个 reader 一个有状态 matcher | 第二棵 scope tree、layer registry、绑定语言、响应式 matcher、框架绑定 |
| opentui/ssh | 独立模块中、位于 `program.Host` 后面的 SSH channel，与 renderer 无关 | 任何让 `core` 知道某种 transport 存在的东西 |
| pi-tui | 已实现的 `headless.Editor` kill ring；固定尺寸的 `ptytest.Screen` 断言模型 | 把 paste marker 做成库功能——机制已经存在，policy 属于应用；行字符串 diff 及它迫使产生的 ANSI-aware 字符串工具 |
| grok-build | §3.2 的具名反例 | purge-and-re-emit resize |
| agentui | detector 应当报告带原因的拒绝项 | 产品语法、第二套 transcript 引擎、detector 内部的路径 policy |
| Charm | cursor 形态/闪烁、terminal 原生任务进度、一次性 owner 调度 | `Cmd`、styled-string rendering、widget 内的文件系统 policy |
| Codex | 响应式语义表格、不截断 diff、键盘 feature 协商、client clipboard transport、ground-fitted theme 与选择性的多维视觉测试 | agent command 与 status 语法、native desktop clipboard policy、第二套 renderer 或 Ratatui 形状的 widget model |

## 有序候选项

按每项被什么挡住排序，而不是按想要程度排序。

1. **在 §3.2 写入具名反例。** 一段话；把没有反方的断言变成陈述过替代方案的判断。
2. **SSH host 模块：已完成。** 它接收已经授权的 `charm.land/ssh` session，运行普通
   program 契约，以不阻塞 SSH request loop 的方式跟踪最新有效 PTY 几何，并在调用
   期间拥有 terminal mode 与 frame settlement。依赖隔离在 `ssh` 模块，`core` 不增加
   SSH 分支或依赖。
3. **测试用 screen-state 断言：已完成。** `ptytest.Screen` 留在 harness 模块，只复用
   terminal-neutral 的 core primitive。停止边界由行为强制：可以输入固定 cell text；
   terminal query、input、buffer ownership 与任意 device-control output 留在外面。
4. **[A 组](#a-通用行为现已实现)：已完成。** 五项能力作为纵向切片落地：有界值；共享
   row provenance、line number 与 code；随后是按内容定宽的 cell 与 settings list。
   settings 组件路由 action，但刻意不引入 scope 系统。
5. **kill ring：已完成。** 它仍是 `Editor` 内部的私有行为：有界存储、按方向累积、
   `Yank` 与紧邻的 `YankPop`；不增加 package 或公开存储抽象。
6. **精确匹配/前缀组合键、cursor 外观与 native progress：已完成。** 三个可执行
   切片按这个顺序落地。第一项从每个组件中移除了 `Pending` 与重复的 lookup/dispatch
   过程；第二项把 cursor 外观变成已交付且参与 diff 的帧状态；第三项把 native progress
   变成感知 keepalive 的 session 能力，在 handover 间恢复并在 close 时清除。
7. **源自 Codex 的终端切片：已完成。** Markdown table 与 diff 现在根据 width 解析而
   不丢弃内容；keyboard 与 clipboard 协议成为显式 host 能力；`Suited` 跟随 terminal
   ground；代表性 screen 在 width 与 colour depth 两个维度都有守卫。每项职责都进入原有
   owner layer，没有藏在一个 Codex 形状的 facade 后面。
8. **`core/link` 的拒绝报告。**
   [§7.1](architecture.md#71-一个包必须配得上它的名字)要求一个边界有两个
   消费方；目前只有一个假想消费方。
9. **paste-to-chip 示例：已完成。** `examples/composer` 拥有阈值、标签与保留的原文，
   `headless.Editor` 只拥有原子编辑行为。

## 什么会改变本文

带新日期的新一轮阅读，以及以下任一证据：

- 有证据表明组件树路由无法表达真实 key scope；这会重新打开被否决的 layer
  registry，而不是悄悄加入它；
- 拒绝报告出现第二个消费方；它会把候选 7 从愿望变成边界；
- 有证据表明 purge-and-re-emit resize 买到了 §3.2 未权衡的东西；它会重新打开一项
  否决，而不只是给它补注释。
