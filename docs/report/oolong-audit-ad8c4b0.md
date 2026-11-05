# Oolong 全仓库所有权与正确性审计

日期：2026-11-05。仓库：[Tangerg/oolong](https://github.com/Tangerg/oolong)。审计基线：[`ad8c4b07ff17e9c1638288ade2a7698f14de586f`](https://github.com/Tangerg/oolong/commit/ad8c4b07ff17e9c1638288ade2a7698f14de586f)。本地工作分支：`audit/2026-11-05`。本轮没有推送远端。

## 结论

确认并修复 9 项正确性、所有权或入口契约问题；另完成 3 类有明确理由的结构性清理。最严重的问题是已经识别为 X10 鼠标报告的字节，在分片、超时或 EOF 路径中重新获得键盘含义。直接违反 one fact one owner 的问题是 Composer 和其 Editor 分别持有可写的同一个行数上限，再用较小值裁决冲突。

修复已包含实现、受影响调用者、针对性回归、英文/中文指南和破坏性 API 记录。没有保留旧字段、双写、兼容入口或数据库迁移。

**最终接口复核：** Markdown 的 Extension 是解析器拥有的封闭语法标识；注册未知标识属于程序配置错误。最终实现保留 SetRenderer 的 void 签名，在任何修改前 panic，渲染与输入失败继续走既有 error 通道。此前增加返回值导致的 examples 发布依赖冲突已从接口设计上移除；最终独立构建结果见第 6、7 节。

本报告中的“未发现”表示在列明范围和证据下没有确认新的缺陷，不表示对所有输入、终端实现、平台或并发调度的穷尽证明。“Go 第一梯队”是工程目标，不能由一次审计或 lint 结果认证。

## 1. 范围、依据与方法

以根 [AGENTS.md](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/AGENTS.md) 和 [PROJECT_RULES.md](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/PROJECT_RULES.md) 为约束，没有发现嵌套 AGENTS。重点规则是：每个事实只能有一个可推进它的拥有者；派生快照和缓存可以存在，但不能发起竞争性转换；API 变更必须同步调用者、测试和文档；开发阶段不维护兼容路径。

基线有 733 个受版本控制的 Go 文件，其中生产文件 291 个、52,147 行，测试文件 442 个、76,824 行。这是本次基线快照的统计，不是新增的可执行仓库目录清单。

模块由 `scripts/modules.sh` 从 `go.work` 派生，共 7 个：examples、extensions/highlight、extensions/latex、extensions/markdown、extensions/mermaid、extensions/ssh、ui。没有手工维护第二份构建清单。

| 审查领域 | 覆盖内容 | 审查方式 |
| --- | --- | --- |
| 控件与外观 | base 52 个、kit 34 个基线生产文件 | 逐文件阅读，追踪会话、控制器、场景、投影、调用者和测试 |
| 核心协议与基础能力 | 输入、ANSI、字符几何、终端、图片、进程、文本、内容、发布、布局和调度基础 | 追踪跨分片、超时、取消、清理及测量/绘制共用规则 |
| 场景、运行时、自动化 | 附着身份、动作、键盘/指针路由、帧提交、任务、能力撤销、Driver | 追踪唯一写入口、生命周期与完成条件；关键改动另做独立复核 |
| 外部扩展与示例 | 五个扩展的生产 Go/JavaScript、全部示例和内部组合辅助 | 对照锁定依赖源码检查 SDK 调用、异常、取消、资源所有权 |
| 工程门禁 | 模块脚本、依赖、架构测试、lint、发布/API 台账、双语文档 | 使用仓库原有命令；没有降低规则或新增忽略来制造通过 |

没有声称逐行人工复审全部 442 个测试文件。现有测试用于理解契约，整套测试和静态门禁用于核验覆盖的行为；新回归针对可观察结果，避免只复制实现结构。

## 2. 已确认的问题及根因修复

严重程度按本项目中的实际影响排列：P1 为可污染正常输入的高影响问题；P2 为可触发的功能错误、所有权冲突或公共入口契约缺陷；P3 为已确认但不单独构成功能故障的维护问题。

### F1 · P1：鼠标报告在等待过程中重新变成键盘输入

**基线证据：** [input/parser.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/ui/core/input/parser.go) 的 Ambiguous、advance、decodeControl，以及 [input/mouse.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/ui/core/input/mouse.go) 的 decodeX10Mouse。

完整识别 `ESC [ M` 后，旧实现仍把 introducer 和未完成的三字节 payload 留在一般 Escape 判定路径中。下一段读取晚于 Escape 截止时间、或输入在报告中间结束时，本应只属于鼠标协议的字节可能变成 `alt+[`、`M` 等按键，进入当前编辑控件。

**根因：** “这批字节已经属于哪个协议帧”没有由一个互斥状态拥有。对原始字节的一般解释仍能与已识别的协议解释竞争。

**修复：**

- 一个私有 `frameKind` 表示无帧、paste、X10、OSC、DCS、APC，替代彼此独立的 paste/string 状态。
- X10 introducer 被识别后立即消费；后续三字节由鼠标 payload 状态独占。
- 等待 payload 不再构成 Escape 歧义；EOF 截断报告被丢弃，不产生键盘事件。
- 保留现有正常输入、SGR 鼠标、paste 和控制串契约。

**证据：** 新的 `TestX10MouseBodyDoesNotExpireAsTyping`、`TestX10MouseBodyAtEOFDoesNotBecomeTyping` 在旧实现上失败，在修复后通过；分片不变性 fuzz 也通过。这里证明的是输入解释错误，没有将它夸大为远程代码执行。

### F2 · P2：任意文本后的 VS16 都被当作 emoji 展示序列

**基线证据：** [grid/surface.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/ui/core/grid/surface.go) 的 ClusterWidth：只要 FE0F 出现在第一个字节之后，便提前返回至少两列。

旧实现把 `A\uFE0F` 测为两列，把只有组合字符的 `\u0301\uFE0F` 也扩为两列，还接受并非直接相邻的 base/selector。错误会影响后续文本坐标、换行、裁剪和残留单元格。

**修复：** 检查原始字素中直接相邻的 base + FE0F，且 base 必须出现在 Unicode 官方 emoji variation 数据中。列宽仍唯一由 `grid.ClusterWidth` 决定，text 和控件不维护第二套宽度判断。

资格数据固定在与当前 go-runewidth 宽度表一致的 Unicode 17，带原始数据、许可证、离线 generator、生成表及全 Unicode scalar 的一致性测试。来源为 [Unicode 17 emoji variation sequences](https://www.unicode.org/Public/17.0.0/ucd/emoji/emoji-variation-sequences.txt)。生成表是来源的只读派生物；修改路径是更新来源并重新生成，不是手工编辑两个真相。

### F3 · P2：emoji 的提前返回吞掉有间距标记的列宽

同一函数在决定 emoji 宽度后立即返回，绕过 U+FF9E/U+FF9F 的独立列宽。按本库既有的“每个 halfwidth sound mark 独占一列”策略，`⚠️ﾞﾟ` 应测为四列，旧实现只给两列，后面的 `x` 也随之错位。

**修复：** 先取得基础字素的展示宽度，再组合有间距标记的列数；资格判断仍在原始字素上进行，避免剔除标记后伪造相邻关系。最终复核还将“计数”和“剔除”的重复分类合并为一次处理，使用具名的 Unicode 标记常量。

F2/F3 的表格回归及实际绘制后续列的回归均暴露过旧实现失败。修复没有增加第三方依赖或另一套字符几何拥有者。终端宽度仍是库的明确估计策略，不能保证所有物理终端采用相同 Unicode 渲染。

### F4 · P2：Composer 与 Editor 竞争拥有行数上限

**基线证据：** [kit/composer.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/ui/kit/composer.go) 中公开的 Composer.MaxRows、Controller 返回的 Editor.MaxRows，以及 HeightForWidth 的二次取小值。

构造 `NewComposer(ComposerConfig{MaxRows: 2})` 后，Editor 自己仍报告上限 0。随后把 Editor 改为 4 或不限行数，Composer 的独立上限继续暗中截断。这符合“两个表示都能前进，再制定胜负规则”的双 owner 定义。

**修复：** 删除 `Composer.MaxRows`。配置只在初始化时播种 Editor；默认 8 行也只在该边界应用一次。运行时上限由 `composer.Controller().MaxRows` 独占，设为 0 后遵循 Editor 的不限行数语义。

首次测量仍为只读：HeightForWidth 不为了套用默认值而创建场景/主题拥有者或修改 Editor。专门的零值测量回归保护这条边界。使用原文件的 Go overlay 执行新所有权回归时，旧实现确实失败。

### F5 · P2：较早的长搜索匹配被视口查询漏掉

**基线证据：** [base/transcript.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/ui/base/transcript.go) 的 MatchesIn 在按起始行二分后，只检查紧邻的前一个匹配。公开契约并未禁止重叠。

对起始行已排序的匹配 `[0,5)`、`[1,2)`、`[2,5)`、`[5,6)` 查询视口 `[4,5)`，旧实现从结果 2 开始，丢掉结果 0。

**修复：** 私有 `transcriptSearch` 聚合拥有来源、匹配快照和当前匹配。由已接纳结果派生“前缀最大结束行”索引，二分定位所有早于视口开始但仍然相交的结果。返回最小的连续原结果区间，保留 `first+i` 的当前结果身份。

该返回区间可包含两个相交结果之间的不相交结果。复杂度为 O(log N + 返回区间与 spans 大小)，并非所有重叠输入下都只与视口高度相关。没有对性能作未经测量的更强承诺。

### F6 · P2：搜索结果入口允许算法无法安全解释的状态

同一旧入口 SetMatches 只校验来源身份/版本/宽度，没有建立其后所有二分和揭示逻辑需要的行与 span 不变量。

**修复：** 在局部候选状态中完整验证，再一次性安装：

1. 匹配行在已提交 source 的行空间中，按非递减顺序排列。
2. 每个匹配至少有一个 span，整段 span 不跨过 source 末行。
3. cell 的列和宽度非负，端点加法不会溢出。
4. 已接纳结果及嵌套 span 切片均为独立副本。

新增 `base.ErrInvalidMatches`；失败时保留旧匹配、当前索引和场景通知状态。外来或过期 source 仍返回 `ErrStaleMatches`。

8 类非法结果的旧实现回归全部失败。两项测试及一项基准曾在空 transcript 中安装非空行匹配，已补齐真实行空间，保留原断言与基准目标，没有放宽生产验证。

这里没有宣称验证“真实文本字数”或强制 cell 端点小于视口宽度：零宽命中与非负的视口外范围仍合法，后者由绘制剪裁。

### F7 · P2：Sticky 接受违背查找/回收前提的身份集合

**基线证据：** [base/sticky.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/ui/base/sticky.go) 的 SetBlocks/Add 与 pinnedAt/nextAfter/discardBefore。

读取和回收算法依赖严格递增且非零的 BlockID，但入口允许零、重复、降序或无序追加。`[3,1]` 可以让读取提前停止；旧批次 `[1,3]` 后追加 `[4,2]` 会发布不能按有序前缀回收的集合。

**修复：** 两个写入口在任何修改/通知之前使用同一验证逻辑检查完整批次。替换必须严格递增且非零；追加还必须晚于当前最后一项。对这类程序员违反 void 命令前置条件的情况在边界 panic，公开契约与现有 panic 架构门禁同步记录。没有自动排序、吞掉元素或分段写入再报错。

7 类原始非法批次回归均在旧实现失败；修复后还验证了旧集合保持不变、没有通知。顺序合法但尚不存在的 ID 继续遵循原布局查询契约，不作未经依据的拒绝。

### F8 · P2：Driver 构造入口延后报告缺失能力

**Automation：** [automation.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/ui/automation/automation.go) 的 NewDriver 收到 nil、typed nil 或缺失能力时返回非 nil 的空 Driver，让调用者到 Observe 时才发现配置错误。现在返回 `(*Driver, error)`，缺少 session/scene/dispatcher 时当场返回 `nil, ErrInvalid`。能力存在规则只由 Driver.valid 维护，构造器在局部候选通过验证后才返回；能力存入接口后的 typed nil 检查也由同一规则负责。现有缺失能力测试已改为直接检查构造边界。

这只验证能力存在，不承诺 Runtime 此刻或未来始终存活。运行中的停止、撤销等事实仍归原调度器，操作时返回相应错误。

### F9 · P2：Markdown 扩展配置入口静默接受无效注册

**基线证据：** [markdown.go](https://github.com/Tangerg/oolong/blob/ad8c4b07ff17e9c1638288ade2a7698f14de586f/extensions/markdown/markdown.go) 的 SetRenderer 可以接纳任意 Extension，而渲染只会分派 FencedCode 和 DisplayMath。未知 key 看似注册成功，实际永远不会调用；移除未知 key 也静默成功。

最终的 `(*Look).SetRenderer` 保留 void 签名，在复制/修改 registry 前验证标识；未知标识在入口 panic，添加和移除共用这条前置条件。Extension 选择的是 Markdown 拥有的语法种类，注册不执行渲染，也不接纳用户定义的语法；因此未知标识是程序装配错误。实际 source、renderer 和 backend 失败继续返回 error。回归覆盖零、未定义值及最大 byte 值，并确认失败不影响两个有效 renderer。保留私有 typed registry 与 copy-on-write，它们有真实的扩展职责。

F8 的证据为契约重塑及源码路径追踪，没有把无法直接编译旧签名的新测试描述为已执行的旧版失败复现。F9 的最终回归已直接在原始 void API 上执行，六类非法注册/移除组合均以“unrecognized extension was accepted”失败；最终实现要求在变更前拒绝，回归通过。

## 3. 结构性清理与严格所有权判断

### S1：删除不承担保证的私有透传

删除 `text.clusterWidth`、`Session.host`、测试专用的 `program.handle` 三个私有透传。调用直接到已有 owner，原能力生命周期检查仍在 Runtime.services；测试仍执行同样的 CaptureInput + handleInput 顺序。

没有按仓库调用次数删除公共 API，也没有把 Controller 访问器、平台适配器、权限/生命周期边界一概视为空包装。

### S2：按独立变更原因整理运行时文件

将帧合成、接纳、发布结算、最终排空放入 `core/program/compositor.go`。run.go 从 725 行降到 456 行，新文件 270 行；这是责任分组，不以减少总行数为成绩。

独立复核将基线对应声明块拼接后与新文件主体比较，逐字一致。状态依旧属于同一 `*program`，没有新的 compositor 状态对象、管理器或接口层。

### S3：消除规则的重复写法

有间距 Unicode 标记的计数和剔除由同一判断完成；Driver 的能力存在验证由 valid 方法独占，构造器与操作入口复用它。搜索索引只由接纳结果生成；Unicode 资格表只由固定来源生成。删除重复事实后，保留必要的编码与投影，并明确它们没有独立写入口。NOTICE 同步记录 Unicode 数据来源，并把已经失效的 primitives 归属路径更正为当前 ui/core 路径，保留原有第三方署名。

### 关键事实的最终 owner

| 事实 | 唯一可推进的拥有者 | 允许保留的投影/输入 |
| --- | --- | --- |
| Composer 编辑区行数上限 | retained Editor.MaxRows | ComposerConfig 是一次性构造输入，测量只读取 |
| 输入当前协议帧 | Parser.frame | 缓冲字节承载该帧的 payload，不重新决定语义 |
| 终端字素列宽 | grid.ClusterWidth | text/控件直接使用；资格表从官方数据派生 |
| Transcript 搜索状态 | Transcript 内的 transcriptSearch | 分离的 Matches 返回值、不可独立编辑的 prefixEnds |
| Transcript 滚动位置 | 既有 Scroll | Sticky 和阅读 anchor 只提供几何/意图 |
| 文档内容与撤销历史 | 既有 EditSession | Editor 维护各自 caret/selection/视图 |
| 附着身份与动作目标有效性 | 既有 scene Root/Attachment | automation 的 target/observation 为有版本、有限生命周期的证据 |
| 任务接纳和完成 | 既有 Runtime/Task 所属边界 | receipt 表示该边界已发生的转换，不代替应用结果 |
| 终端图片资源/子进程生命周期 | 既有 terminal/process owner | 示例持有的资源句柄不伪造终端确认 |

## 4. 对所列代码坏味道的逐项结论

| 指标 | 本轮发现与处理 | 判断边界 |
| --- | --- | --- |
| implicit coupling | 修复 X10 的超时路径耦合、搜索窗口对前一个结果的隐藏假设、Sticky 对输入顺序的隐藏前提 | 生命周期/帧提交的依赖通过原有公开契约及测试核对 |
| layer-first folders | 检查真实依赖图，保留 core/base/kit 的协议/行为/外观分界 | 文件夹名字不构成缺陷；没有再增加 service/manager/utils 层 |
| duplication | 删除 Composer 双上限，合并 Unicode 标记规则，删除私有宽度别名 | 不把不可变快照、必要的 wire 编码和不同平台实现当作同一事实的 owner |
| representable illegal states | 用互斥 frameKind 收缩状态空间；搜索、Sticky、renderer、Driver 在入口拒绝非法状态 | Go 的公开 enum/切片不能单靠类型封闭，需在真正接纳入口建立不变量 |
| god files | 拆出有独立责任的 compositor 和搜索聚合 | editor.go 1357 行仍是热点，但不能仅凭行数断言一个内聚对象是 god object |
| shotgun surgery | 行数策略、列宽、搜索状态均有单一修改点；更新破坏性 API 的所有真实调用者 | 改签名后多处调用者处理 error 是必要迁移，不等同于重复业务规则 |
| layering violations | 运行原有实际 import DAG/架构测试和跨平台静态检查 | 没有用注释中的高层名称误报 import 违规 |
| pass-through wrappers | 删除三个没有额外契约的私有别名 | 保留有适配、生命周期、权限或行为组合职责的边界 |
| errors / validation not at boundary | 修复 SetMatches、Sticky 写入口、NewDriver、SetRenderer | 不是所有错误都在构造期可知；异步失败仍由其真实 owner 报告 |

进一步的编码要求也纳入审查：

- **OOP 与充血模型：** 把不变量收进真实的状态拥有者；保留 Go 的小类型、方法和配置组合。纯算法保持普通函数，没有为形式上的面向对象引入基类或万能 manager。
- **抽象合理性：** 搜索聚合拥有完整不变量，frameKind 表达真实互斥阶段；没有为搬文件新增接口，也没有把必要扩展 registry 展开成随处可写字段。
- **注释：** 新增注释解释所有权、生命周期、为什么需要前缀最大值、错误/EOF 的语义和 Unicode 来源。保留许可证、生成指令及公共边界的必要契约；没有增加逐语句旁白。
- **复杂度：** 使用普通分支、互斥状态、一次性验证和现有标准库能力；遵守仓库 gocyclo/gocognit 门禁，没有提高阈值或对新实现添加豁免。
- **魔法值与命名：** 协议阶段、Unicode 标记、搜索 source/index 都有对应语义；生成表有权威来源。registry 的 key 使用 Extension。没有把本地可见文案和一次性布局示例全部提升成无意义全局常量。
- **组合优先：** Composer = Editor + prompt/help + scene composition；程序的能力保持小边界组合，没有继承链或影子行为模型。
- **彻底修复：** 不保留 Composer.MaxRows 镜像，不双写、不加仲裁器、不排序吞掉错误批次、不制造旧 API 转发入口。

## 5. 第三方 API/SDK 检查

| 集成 | 对照的版本/来源 | 重点结论 |
| --- | --- | --- |
| Unicode / runewidth | go-runewidth v0.0.30；Unicode 17 官方 variation 数据 | 修复不合法 VS16 扩宽和 spacing mark 组合，单一几何 owner |
| Goldmark / Markdown | goldmark v1.8.6 的实际下载源码 | 检查解析、扩展分派、不可变文档投影、准备/绘制边界与错误身份；修复无效扩展注册 |
| Chroma | chroma/v2 v2.27.0 | 检查 token 对源文本的消费、空 token 边界及异常恢复，未确认新的接入错误 |
| LaTeX | [go-latex v0.3.0](https://pkg.go.dev/codeberg.org/go-latex/latex@v0.3.0) | 核对 backend 自有可变缓存和字体 buffer，检查解析预算、fallback 与字素测量；没有将共享字体数据误报为 race |
| SSH | charm.land/ssh v0.4.3、x/crypto v0.57.0 | 对照 PTY 接纳、窗口 channel、session 生命周期及写入转换，现有适配器与这些契约一致 |
| Mermaid CLI | @mermaid-js/mermaid-cli 11.17.1；实际安装的 Puppeteer 25.16.0 | 源码核对 renderMermaid/import/request interception；模拟后端、进程与取消测试通过，真实浏览器运行受环境限制 |
| 系统调用 | Go 1.27.3 标准库源码及平台实现 | 对照 Windows flags、进程 Job 与 Unix process group；通过跨平台 vet/build，不宣称跨平台原生测试 |

Mermaid 图片节点支持 URL，官方 [image 文档](https://mermaid.js.org/syntax/flowchart.html) 与仓库 `TestOfficialCLI/cancel after browser request` 均说明这条能力。其 [securityLevel 配置](https://mermaid.js.org/config/schema-docs/config.html) 不等于网络隔离。因而没有把已声明的网络能力重新包装为本轮发现的 SSRF 漏洞，也没有声称完成过被环境阻塞的真实浏览器网络复现。

`govulncheck` 在 7 个模块中均未报告可达漏洞。SSH 依赖图存在一个 **module-only** 提示 [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)，指向未维护的 x/crypto/openpgp；本项目未导入受影响的 OpenPGP package，也没有命中受影响 symbol。因此不能将它写成 oolong 存在可利用漏洞，也不能写成“依赖图毫无通告”。本轮没有为消除一个不相关 package 提示而盲目升级整套依赖。

## 6. 实际验证

验证使用官方 Go 1.27.3，Go 运行与构建的并行度均限制为 2；另按仓库声明验证 Go 1.27.0 语言/工具链下限。工具版本沿用 CI 固定值，完整命令见随附验证记录。表格只记实际执行的结果。

| 检查 | 状态与范围 |
| --- | --- |
| 工作区所有模块 race 测试 | 7/7 通过；包括架构、语义、过程、终端与所有扩展测试；原生 Mermaid 环境变量未设置，其安装后端用例按已有规则跳过 |
| golangci-lint v2.14.0 | Linux/Darwin/Windows × 7 模块，共 21/21 通过，0 issues；已有“规则未命中”警告不等于缺陷被忽略 |
| 可达性 | 原脚本 21/21 平台/模块批次通过，无 actionable finding；保留原 noCopy 静态标记豁免 |
| 跨平台 vet + build | 6 目标 × 7 模块 × 2 操作 = 84/84 通过；最终列宽清理后只重验受影响 ui 的 12 项 |
| API 迁移台账 | 从实际模块 tag 比较，在 Linux/Darwin/Windows 上提取 API；退出 0 |
| 格式/模块清单 | gofumpt、shfmt、sync-workspace --check 通过 |
| govulncheck v1.8.0 | 所有模块退出 0；0 可达漏洞，保留上文 module-only 提示 |
| 已发布依赖独立构建 | 7/7 通过；最终 Markdown 和 examples 的完整 GOWORK=off -mod=readonly 测试已重跑，均通过；没有修改依赖声明或使用本地 replacements |
| 本地声明依赖、下限、tidy/sync | 隔离副本中 GOWORK=off：7 模块 Go 1.27.0 原生测试、14 个 Windows/Darwin 测试源码编译批次全部通过；7 模块 tidy -diff 无变化，work sync 无清单/校验和漂移；最终列宽小改动另补验受影响包 |
| 全部原生 fuzz 目标 | 26/26，各运行 30 秒；各目标最终有效运行合计 10,558,126 次；最终列宽清理后重验两个 grid 目标，均通过；目标由实际编译测试文件派生，无新增失败 corpus |
| 文档完整门禁 | npm audit、拼写、57 份 Markdown 的格式检查、架构图来源检查、VitePress 构建全部通过 |
| 补丁应用与差异检查 | git diff --cached --check 通过；包含全部新增文件的补丁通过原始 ad8c4b0 干净工作树上的 git apply --check |

最终接口复核后，只重验受影响路径：Markdown/examples 的工作区 race 与已发布依赖测试共 4 批次、三平台 lint 共 6 批次、六目标 vet/build 共 24 项全部通过；Driver 的既有测试、race、独立依赖、三平台 lint 和六目标 vet/build 通过。Go 1.27.0 隔离依赖图同步了这两项最终调整，原生测试及 Windows/Darwin 测试源码编译也通过。其他源码及依赖未变，沿用已完成的全模块验证，没有把历史候选的 examples 失败当作最终状态。

有针对性的原实现失败证据覆盖 X10、Unicode 测量/绘制、Composer、重叠搜索、8 类搜索非法输入及 7 类 Sticky 非法批次。重构部分另用主体逐字比较、现有测试和静态门禁核验。没有为提高测试数量补无意义的实现镜像测试。

**浏览器限制：** 官方 CLI 安装成功，精确浏览器版本下载成功，但 Chromium 拒绝 root 身份下带 sandbox 的启动；普通 unprivileged 执行又被环境的组切换权限阻止。因此实际 Mermaid PNG 渲染、主题像素断言、真实浏览器取消路径未在本环境通过。没有添加 `--no-sandbox`，没有改宿主机安全策略。

**平台限制：** 本地原生执行为 Linux。Darwin、Windows、FreeBSD、WASI、JS/Wasm 的结果为源码检查/编译证据，不冒充物理平台终端运行。短时 fuzz 提供增量覆盖，不代表输入空间穷尽。

## 7. 破坏性变化与独立依赖闭合

| API | 新契约 | 调用者动作 |
| --- | --- | --- |
| automation.NewDriver | `(*Driver, error)` | 接收并处理构造错误；运行时停止仍由原 owner 报告 |
| kit.Composer.MaxRows | 删除 | 构造用 ComposerConfig.MaxRows；运行时用 Controller().MaxRows |
| Transcript.SetMatches | 签名不变，新增 ErrInvalidMatches | 提交与当前 source 一致且合法的结果，区分 stale 与 malformed |
| Sticky.SetBlocks / Add | 签名不变；非法身份批次在任何修改或通知前 panic | 提供非零、严格递增的身份；追加晚于旧末项 |
| markdown.Look.SetRenderer | void 签名不变；未知 Extension 在变更前 panic | 只用 FencedCode 或 DisplayMath；渲染错误仍按原 error 契约处理 |

上一轮候选曾为 SetRenderer 增加 error 返回值，并因此让 examples 的独立依赖图出现 `no value used as value` 编译错误。最终复核结合 API 实际语义、现有 Sticky/scene 前置条件和两次独立评审，选择在公开入口用文档化 panic 拒绝未知封闭标识。

这项调整保留全部 F9 校验，同时删除仅为处理返回值而新增的测试 helper 和调用者分支。没有恢复静默接纳，没有双接口、反射探测、旧 API 转发或兼容路径，也没有伪造版本、提交本机 replace、修改依赖声明或放宽 CI。合法注册方法的类型保持不变，examples 因而可以同时消费当前模块源码和真实声明的已发布模块。

独立验证命令由仓库模块清单派生，对每个模块执行：

```sh
GOWORK=off go test -mod=readonly -count=1 ./...
```

源码工作区验证负责当前所有模块的协调行为，已发布依赖验证负责消费者声明能否实际满足 API；两者都保留。新增 panic 是对非法配置的行为收紧，已记录在 CHANGELOG，调用者应在正确的装配位置传入有效的语法标识。最终不会为了这个修复而要求用户先执行一次额外的远端推送或发布。

## 8. 持续关注点

- `base/editor.go` 仍有 1357 行，`base/transcript.go` 修复后 850 行。它们是集中变更热点，但本次没有发现足以支持整体重写的新增独立生命周期冲突。后续新增能力应优先沿已有 edit session、layout、selection、clipboard、search 责任扩展；若跨责任改动持续发生，再以具体变更证据拆分。
- `core/input/parser.go` 的合法复杂性来自多个真实协议。现在互斥帧状态已集中；后续新增协议应加入同一 framing 状态及分片/超时回归，避免重新增加独立布尔开关。
- 公共切片与类型化 enum 的非法值仍可能由 Go 调用者构造。正确边界是在 owner 接纳时完整拒绝，而不是在所有绘制/读取函数中层层补丁式校验。
- 文档或代码的可读性没有单一数值阈值。没有用大量重命名、清空公共契约注释、取消有用封装来获得表面的“干净”。
- 原生 Mermaid 验证仍受当前环境限制，应在具备正常 Chromium sandbox 的 CI 环境执行；本地模拟后端、源码追踪和交叉编译不能替代真实浏览器运行。

UX 的收益是减少错误输入、错位绘制和漏高亮；DX 的收益是配置失败更早、所有权更明确、错误可定位；AX 的收益是自动化继续调用同一真实动作/调度拥有者，不引入影子状态，并让构造失败在代理开始工作前可见。
