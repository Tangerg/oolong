# 先行者：四个终端界面，以及该从它们那里拿什么

状态：一份附带取舍结论的调研。它与
[architecture.md §5](architecture.zh-CN.md#5-从前端与-flutter-体系里拿什么)
互为姊妹篇；后者用同样的方法看浏览器和 Flutter：把值得学习的职责与承载它的
机制分开，并明确说出哪一半不采用。

这里没有任何一项自动成为承诺。每个候选项仍然必须通过
[§7.1](architecture.zh-CN.md#71-一个包必须配得上它的名字)和
[§15](architecture.zh-CN.md#15-架构如何生长)的关卡，其中有几项今天仍然过不了。
一份调研一旦变成待办清单，就已经不再是调研。

## 读了什么，读到什么程度

| 项目 | 阅读版本 | 语言 | 是什么 |
| --- | --- | --- | --- |
| [agentui](https://github.com/minoism/agentui) | `ec0414e`，2026-07-28 | Go，约 245 个文件 | 一个 agent CLI，与本仓库有共同祖先。 |
| grok-build | `780d138`，2026-08-03 | Rust，约 2,500 个文件 | 已交付的产品；`xai-ratatui-*` crates 是它的终端层。 |
| [opentui](https://github.com/anomalyco/opentui) | `b55f125`，2026-08-05 | TypeScript + Zig | 一个带 React、Solid、SSH 和 3D 包的终端 UI 平台。 |
| pi-tui | `@earendil-works/pi-tui` | TypeScript | “最小化的差量渲染终端 UI 框架”。 |

这是调研，不是审计：读的是包文档、导出表面，以及各项目解释自身时指向的文件。
下文引语来自这些来源；关于**本仓库**的每一项判断都对照代码核验过。

这些项目会继续变化。没有日期的对比，会在无人察觉时停止成立；本仓库已经犯过
一次这种错误，并记录在 [ROADMAP.md](../ROADMAP.md) 里。

## 1. opentui/keymap 回答了本仓库已经写下的一项限制

`core/keymap` 自己写明了上限：

> 一个序列如果是另一个绑定的真前缀，就不可达：没有计时器驱动查找时，只要更长
> 的绑定仍可能到来，就不能选择较短的绑定。因此更长序列优先。

`@opentui/keymap` 没有这个上限：

> 可编程的精确匹配/前缀消歧（例如 `g` 与 `gg`），带 `runExact`、
> `continueSequence`、`clear`，以及延迟的 `AbortSignal` + `sleep` 决策；并附带
> Neovim 风格的超时解析器。

值得拿的思想不是“加一个超时”，而是：**消歧是调用方提供的值**，超时解析器只是
它的一种实现。

这个形状能解开本仓库无法从内部绕出的僵局。依赖图禁止 `runtime` 触达
`interaction`，也禁止 `headless` 触达 `runtime`，所以库内**没有任何一层有资格
拥有计时器**。解析器并不需要拥有它：`Pending` 报告歧义存在以及有哪些选择，能看见
时钟的调用方给出答案。库仍然无需知道时间是什么。

另有两件值得拿的东西，它们彼此独立：

- **带作用域、按优先级排序且可以下落的层。** 本仓库每种 widget 只有一张 map，
  没有作用域概念；应用若想让一个绑定只在某个 pane 聚焦时生效，只能在 `Handle`
  里手写条件。
- **诊断。** opentui 报告遮蔽、不可达绑定、未激活原因，并对层图运行 lint 式分析。
  一张能回答“这个绑定为什么永远不触发”的 keymap 才容易调试；本仓库的还不能。

**不采用。** opentui 的 keymap 罗列了十二项主能力。其余九项包括可插拔的绑定语言
（解析器、展开器、变换器、命令解析器、字段编译器）、可注册 schema、带订阅通知的
响应式 matcher，以及 React 和 Solid 绑定。
[§16](architecture.zh-CN.md#16-明确否决的设计)已经否决通用 signals/observables
框架，[§12 规则 8](architecture.zh-CN.md#12-go-api-规则)要求配置与问题成比例。
十二项里取三项，才是诚实的比例。

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
[§13.1 条件 3](architecture.zh-CN.md#131-v1-的兼容性边界)：它要求两个独立维护的
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

**本仓库已经具备机制。** `headless.Editor` 的 element 与 `text.Mark` 正是为此存在：
“prompt 里代表文件的 chip 是原子的：不能把文本键入其中……”。缺的不是能力，而是
还没有人把它用于 `input.Paste`。因此它应当是 worked example，而不是库改动——阈值
和 marker 文案都是产品决策，本来就属于应用。

### pi-tui 的差量渲染：最有意思的分歧

pi-tui 渲染为 ANSI 样式**字符串**数组，并按**行**做 diff：

> 1. **首次渲染**：输出所有行，不清除 scrollback
> 2. **宽度变化或变更发生在 viewport 上方**：清屏并完整重绘
> 3. **普通更新**：移动到第一条变化行，清除到末尾，绘制变化行

它有三项值得说清的性质：两项有利于本仓库，一项不利。

**单位决定工具。** 因为一行是带样式的字符串，pi-tui 需要 `visibleWidth`、
`truncateToWidth` 与 `wrapTextWithAnsi`——整族函数都在推理埋着转义序列的文本。
绘制到 cell 就完全不需要这族工具：宽 grapheme、hyperlink 和被绘制的 image region
都是结构化值，而不是必须重新解析的字节。这与 [ROADMAP](../ROADMAP.md) 对行字符串
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
[§16](architecture.zh-CN.md#16-明确否决的设计)拒绝终端 emulator。该模型内部保存
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

[§3.2](architecture.zh-CN.md#32-活动界面必须保持有界)直接否决第一种：

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
都属同类。[§5.3](architecture.zh-CN.md#53-组件阶梯)把这些全部放在应用层：共享组件
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
里至少有两个实现——这正是 [§7.1](architecture.zh-CN.md#71-一个包必须配得上它的名字)
要求的证据。它们现在构成一个已经完成的纵向切片，而不是永久目录愿望。

| 能力 | 谁有 | 本仓库的实现 |
| --- | --- | --- |
| **有界数值** | opentui `Slider` | `headless.Slider` 拥有边界、值的所有权、按键和已提交的拖动几何；`kit.Slider` 是其 appearance。`Progress` 仍是不同的只读比例。 |
| **行号** | opentui `LineNumberRenderable` | `text.Row` 携带逻辑行来源，`headless.RowGutter` 是共享接缝，`kit.LineNumbers` 同时装饰 editor 和 code block。 |
| **代码块** | opentui `Code` | `kit.Code` 组合复制后的 `text.Line`、换行、复制行与可选 gutter，且不依赖可选的 `highlight` 模块。 |
| **按内容定宽的列** | opentui `TextTable` | `kit.Cell` 把首选宽度和绘制放在一起；`Column.Fit` 测量最宽标题或 cell；`TableLayout` 在整帧复用结果。 |
| **设置列表** | pi-tui `settings-list`、agentui `catalog` | `headless.Settings` 在 `List` 上增加选中值动作；`kit.Settings` 提供按内容定宽的 label/value 行，应用数据与修改仍留在下游。 |

### B. 真实存在，但被同一个共同问题挡住

以下三项看起来像三个组件，其实是一个尚未回答的设计问题——**作用域**——戴着三顶
帽子。分别实现会制造三种互不兼容的作用域概念。

| 缺失项 | 谁有 | 问题是什么 |
| --- | --- | --- |
| **带作用域的 command palette** | agentui `palette`（`Scope`、`Predicate`、`Registry`） | `headless.Commands` 有 `Add`、`Remove`、`Lookup`、`Used` 与 `Find`，但不能表达 command 只在某个上下文可用；调用方只能在 `Find` 后过滤，而且每个调用方各写一套。 |
| **completion source** | agentui `completion`（file、shell、`@` reference） | `Completion.Offer(token, candidates)` 从调用方接收候选。按 context 产出候选的 `Source` 是通用的；file 与 shell source 本身属于应用。 |
| **file picker** | bubbles `filepicker` | 行为——tree、filter、selection——属于 `headless`；哪些目录可见是**安全决策**。agentui 把它放在 `policy` 参数，而不是 picker 内部；这条线是正确的，也正是它不能直接成为 widget 的原因。 |

[§1](#1-opentuikeymap-回答了本仓库已经写下的一项限制)的 keymap scope 是第四顶帽子。
无论怎样回答其中一个，都应当同时回答四个。

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
[§16](architecture.zh-CN.md#16-明确否决的设计)已经拒绝为了数量加入 timer/stopwatch，
也拒绝把文件系统 policy 塞进 file picker。

这些规则并不真的与丰富目录冲突，只要守住分界：**足够通用且有两个消费方的行为进入
`headless` 与 `kit`，其余成为 worked example。** file picker 的 tree、filter 与
selection 属于前者，policy 属于后者；settings list 属于前者，具体 setting 属于后者。
A 组完全属于前者，因此无需导入产品语法就能实现。

顺序先做 A，因为五个完全造不出来的组件，比任意数量的已有能力别名更有价值。A 完成
以后，B 仍然是一件工作，因为 scope 仍然是一个问题。C 继续由需求驱动；若只是产品
组合，就属于 example。

## 总结

| 来源 | 采用 | 不引入 |
| --- | --- | --- |
| opentui/keymap | 调用方提供的消歧；带作用域和优先级的层；遮蔽与不可达绑定诊断 | 可插拔绑定语言、可注册 schema、响应式 matcher、框架绑定 |
| opentui/ssh | 独立模块中、位于 `program.Host` 后面的 SSH channel，与 renderer 无关 | 任何让 `core` 知道某种 transport 存在的东西 |
| pi-tui | 已实现的 `headless.Editor` kill ring；固定尺寸的 `ptytest.Screen` 断言模型 | 把 paste marker 做成库功能——机制已经存在，policy 属于应用；行字符串 diff 及它迫使产生的 ANSI-aware 字符串工具 |
| grok-build | §3.2 的具名反例 | purge-and-re-emit resize |
| agentui | detector 应当报告带原因的拒绝项 | 产品语法、第二套 transcript 引擎、detector 内部的路径 policy |

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
6. **[B 组](#b-真实存在但被同一个共同问题挡住)与 keymap scope，作为一件工作。**
   scoped palette、completion source、file picker 的 policy 接缝与 keymap layer，是一个
   问题的四顶帽子。回答四次，正是仓库得到四种不兼容 scope 概念的方式。
7. **`core/link` 的拒绝报告。**
   [§7.1](architecture.zh-CN.md#71-一个包必须配得上它的名字)要求一个边界有两个
   消费方；目前只有一个假想消费方。
8. **paste-to-chip 示例。** 属于 `examples`，不属于库。

## 什么会改变本文

带新日期的新一轮阅读，以及以下任一证据：

- 这里出现一个真实界面，确实被前缀限制卡住；它会把候选 4 提前；
- 拒绝报告出现第二个消费方；它会把候选 7 从愿望变成边界；
- 有证据表明 purge-and-re-emit resize 买到了 §3.2 未权衡的东西；它会重新打开一项
  否决，而不只是给它补注释。
