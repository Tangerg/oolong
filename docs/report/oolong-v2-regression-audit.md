---
title: Oolong v2 回归审计与增量修复
description: 核对已应用版本的 CI、逐条追踪 0.25.0 Fixed 的回归保护，并记录本轮修复及验证边界。
contentType: Reference
---

# Oolong v2 回归审计与增量修复

审计日期：2026-10-31。旧版本基线为 [042eaa1](https://github.com/Tangerg/oolong/commit/042eaa1c6b13bd863f157b48f715386e8e6b61f3)，用户已应用的新架构基线为 [13c87df](https://github.com/Tangerg/oolong/commit/13c87dfa5b28d5834b2889d77da5bd9fc3251f4b)。本报告和增量补丁以后一提交为起点。

## 结论与范围

用户指出的风险成立：测试函数总量下降，需要核对仍然存在的失败场景；旧提交的 CI 也确实没有全绿。本轮发现了生产代码缺陷和遗漏的具体回归断言，已将修复放回对应事实的所有者。

审计单位是**触发条件、可观察结果和失败后的重试保证**。同名函数、相似文件名以及测试数量都不能单独证明等价迁移。这里逐条核对 [0.25.0 Fixed](https://github.com/Tangerg/oolong/blob/main/CHANGELOG.md#0250--2026-10-22) 的全部 21 项，并额外追查用户点名的删除文件和没有同名新路径的 headless 测试。没有宣称已经逐个证明全仓 403 个消失旧函数名的所有子用例都被迁移。

层级继续为 `ui/core` 的原子能力、`ui/base` 的无样式行为、`ui/kit` 的外观组合，扩展保留独立依赖边界。没有恢复 headless 路由树、通用受控 accessor、旧 Inline 队列或 Markdown 前缀解析协议，也没有改动工程约束、降低门禁或增加兼容层。

## 1. 已应用提交的真实 CI

主工作流 [41105246578](https://github.com/Tangerg/oolong/actions/runs/41105246578) 对应精确提交 `13c87df`，结果为失败：20 个 job 中 16 个成功、4 个失败。独立的[文档部署工作流](https://github.com/Tangerg/oolong/actions/runs/41105245592) 成功。

| 检查 | 该提交的远端结果 | 已确认的信息 |
| --- | --- | --- |
| Linux，Go 1.27.0 / 1.27.x | 成功 | 含测试及 race 检查 |
| macOS，Go 1.27.0 / 1.27.x | 失败 | `TestReleaseGatesKeepTheSelectedModule` |
| Windows，Go 1.27.0 / 1.27.x | 失败 | 同一个测试，不只 macOS 受影响 |
| lint、govulncheck、API ledger、reachability | 成功 | 没有把“未在用户本机执行”误写成“CI 没有执行” |
| 独立模块、fuzz、格式和模块检查、文档、交叉构建 | 成功 | 这些结果属于旧提交，不代替本轮补丁验证 |

已读取 macOS job `124233186126` 和 Windows job `124233186049` 的完整日志。`gorelease` 实际进入了正确的 `ui` 目录，产生的唯一 tag 也是 `ui/v0.1.0`；失败来自夹具直接比较 shell 的 `$PWD` 与 Go 的临时目录字符串。macOS 的 `/private/var/...` 与 `/var/...`、Windows 的 `/c/...` 与 `C:\...` 可以表示同一个目录，却不满足字符串相等。

修复后的夹具在 `gorelease` 的实际执行目录读取 `go.mod` 的模块身份，断言为 `example.invalid/oolong/ui`，继续核对唯一发布 tag。新增 symlink 场景让 Linux 也能覆盖逻辑路径与物理路径不同的情况。原直接路径测试没有跳过；只有新 symlink 场景在系统不支持创建符号链接时沿用仓库已有的处理方式。

还在隔离副本中故意把被检查模块换成 `examples`，确认新断言仍然失败。它消除了错误的路径假设，没有放宽“必须检查所选模块”的保证。

## 2. 测试统计的正确解释

统计由两个提交的 Git 文件树重新计算。Test / Fuzz 数量使用 Go AST 中的顶层声明，并以独立扫描交叉核对；代码行数是生产 `.go` 文件的物理行，包含注释、空行及示例，不是可执行语句数量。

| 指标 | 042eaa1 | 13c87df | 差值 |
| --- | ---: | ---: | ---: |
| Test 函数 | 2,100 | 1,900 | -200 |
| Fuzz 函数 | 22 | 23 | +1 |
| Test + Fuzz | 2,122 | 1,923 | -199，约 -9.4% |
| 测试文件 | 291 | 309 | +18 |
| 生产 Go 文件 | 221 | 241 | +20 |
| 生产 Go 物理行 | 44,373 | 47,413 | +3,040 |

按旧、新逻辑包路径归一后，减少主要集中于 base：706 → 486；kit：221 → 183；Markdown：73 → 51；grid：152 → 145。新增保护分布在 scene、publication、content、program 等新所有者。这解释了变化发生在哪里，不能证明覆盖已经足够。

Git 的重命名识别也会影响直观判断：以默认 50% 相似度查看，标记为删除的 62 个测试文件中，41 个在新结构下仍有对应路径。headless 的 56 个删除项实际是 **37 个测试文件和 19 个生产文件**，不是 56 个测试文件；旧 headless 的 76 个测试文件中，61 个仍有同名 `ui/base` 文件，15 个没有。

用户特别提到的文件：

- `inline_finish_test.go`：新的 grid 和 PTY 路径均存在；需要逐场景核对，不能只看删除标记。本轮由此补出单槽 writer 的真实退出缺陷。
- `stream_reference_test.go`：旧文件的四个函数没有原样保留。新协议消除了前缀缓存，但独立的 URL、首定义优先和样式断言需要补回，见 F16。
- `render_callbacks_internals_test.go`：迁到 `ui/internal/arch`，两个 Test 函数和核心检查函数的函数体完整保留。相邻 render effect 检查也保留，没有丢掉回调阶段登记约束。

## 3. 根因修复与所有权

### 3.1 最后一帧与清理帧必须按实际完成顺序交接

原退出路径先接纳最后的 live / publication frame，随即尝试接纳清理帧。对于只有一个普通槽位的合法 writer，最后一帧仍占据容量；清理因此返回 `term.ErrQueueFull`，光标恢复可能没有送出。大容量或调度较快的夹具会掩盖这个问题。

`program.finishBlock` 现在先 Drain 已接受输出并结算 Written 水位，再准备和接纳清理。末尾的 Drain 仍负责清理帧本身。失败保留真实原因，未接纳的清理仍可重新准备。没有增加第二队列、清理状态镜像或盲目重试。

`TestInlineShutdownDrainsItsLastFrameBeforeCleanup` 使用只在 Drain 后释放容量的单槽 writer，检查内容在清理之前、光标恢复确实送达、原 receipt 达到 Written、待交付责任清空。将同一测试放到精确旧基线，编译成功但以 `writer queue is full` 失败；只替换本轮 `run.go` 后通过。`TestInlineCleanupPreservesADrainFailure` 另行保护错误原因及清理的可重试性。

### 3.2 捕获阶段由 scene 的组合所有者统一约束

旧 Transcript 的 `publishing bool` 只挡住自身 Add / Close。Scroll、Selection、Sticky 共享 Element，却无法从 scene 得知正在捕获；detached Element 更直接被视为可变。因此捕获回调能修改随后将用于退休的阅读状态。

新增 `Element.ReadOnly(func())`，由 scene 拥有同步只读借用：mounted composition 复用 Root 的阶段，detached composition 使用组合树顶端节点。借用期间不能通过挂载或更换父节点改换所有者；嵌套借用恢复外层阶段，panic 和 goroutine 退出同样通过 defer 释放。直接 Element.Measure 复用这个能力。

Transcript 用该原子能力包围捕获，删除独立 `publishing` 字段；成功接纳后再退出借用并退休原前缀。Scroll.Wheel 进入既有 mutation guard。Selection 的公开 `Clicks` 可变字段收为私有，配置通过 `SetClickInterval(time.Duration)` 进入相同边界。每个控制器没有新增自己的捕获锁。

同一条调用方行为测试在旧基线上可以清空 selection，并因 `panic=<nil> active=false` 失败；只覆盖修复后的生产文件便通过。新增测试同时覆盖 mounted / detached、嵌套、正常退出、panic、goroutine 退出，以及非法修改失败后可以合法重试。

复审还发现只禁止模型 setter 不够：捕获回调可以构造另一棵 Root，调用同一个 Transcript / Scroll 的 Stage 再 Commit，使读取的 geometry 在借用中改变。隔离复现中，原两行前缀退休之后只剩一行，但 scroll offset 错误地变成 2。

因此 `Element.CheckProjection(frame)` 同时约束阶段和生产者身份：有 Element 的控制器必须已挂载，且只能使用同一 Root 的帧；Snapshot 的 staging 和 refinement 都咨询该事务的 Root 阶段。pending candidate 与新的只读借用互斥，拒绝发生在借用入口，不能让已经接纳输出后的 Commit 再失败。这里增加的是对同一事实所有者的引用和检查，没有新增一套 pending 状态。

ScrollLayout 原先还保留一份可写候选 state：先改副本，再调用 Staged.Set。操作被拒绝并 recover 后，副本的 offset 可能已改变，而实际 snapshot 没变。本轮删除这份副本，局部计算仅在 Set 成功时发布；Offset 从同一个 staged value 读取。对应借用在 Prepare 结束时失效，之后读取实际 Scroll 控制器的位置。这是删除第二个可推进事实的路径，而非给两个值增加同步规则。

对应测试夹具改为挂载控制器真实 Element，并在挂载前配置 paint slot，保留原来的几何、异常、重复 staging 和竞争 Root 断言。没有用测试专用旁路或临时自动挂载维持旧 wrapper 用法。该破坏性调整让生产组合与测试组合遵循同一所有权关系。

这一边界的直接保护包括 `TestProjectionRequiresItsMountedElementAndActiveFrame`、`TestPendingCandidateRefusesProjectionBeforeCallingApplication`、`TestNestedReadOnlyCannotStageOrRefineActiveGeometry`，以及 Transcript 的 `TestTranscriptStageRequiresItsOwnMountedSceneBeforeMeasurement`、`TestTranscriptCaptureCannotReplaceItsCommittedReadingProjection`。外部包测试 `TestReadOnlyRefinementCannotPublishASecondScrollCandidate` 同时检查 Resize / Reveal / Anchor 拒绝后的 offset、后续翻页行为和 Prepare 结束后的借用失效。

**迁移要求：** `selection.Clicks.Within = interval` 改为 `selection.SetClickInterval(interval)`。非正间隔使用默认值；独立的 `base.Clicks` 手势原子类型继续存在。真实鼠标事件测试以受控时钟验证默认、负间隔和短间隔的选词行为，没有保留兼容字段。有 Element 的自定义控件须通过真实 Element 参与 scene；未挂载时可以编辑或测量，不能借无关 Root 发布几何。

## 4. 0.25.0 Fixed 的 21 项逐条对照

编号严格按旧 CHANGELOG 的 Fixed 顺序。表中的测试名均指当前源码中的真实断言；“已保留”意味着本轮核对了相关失败条件与断言，不表示逐行审计了所有相关生产代码。

| 项目 | 仍需保护的失败场景 | 当前测试证据与本轮处理 |
| --- | --- | --- |
| F01 | finishing 不越过未结算输出，拒绝后 anchor 和清理能力保留 | `ui/core/grid/inline_finish_test.go` 的 `TestInlineFinishCannotOvertakeAnUnsettledFrame`、`TestInlineAbortedFinishLeavesItsAnchorAndCleanupAvailable`，及 `ui/uitest/ptytest/inline_finish_test.go` 的真实终端行断言已保留。新增 program 单槽退出与 Drain 失败回归，修复最终帧占满容量时清理被拒的问题。 |
| F02 | 变窄重排中的尾部空白列、整段空白必须覆盖旧终端文字 | `ui/uitest/ptytest/inline_resize_test.go` 的 `TestResizedInlineRowsClearBlankColumnsBeforeWrapping` 预先留下旧文字，比较完整终端行；空白尾和空白段两个分支均保留。 |
| F03 | 已接受 cells 按当前宽度和色深投影；最右字符、链接、样式和 live anchor 正确；零宽或过宽 atom 拒绝不丢内容 | `ui/core/grid/color_test.go` 的 `TestCapturedInlineRowsUseTheDepthAtComposition`，grid 的 full-width 测试，以及 PTY 的 `TestCapturedInlineOutputUsesTheCurrentTerminalWidth`、`TestCapturedInlineRowsWrapWithoutChangingTheLiveAnchor`、`TestCapturedInlineOutputSurvivesAZeroWidthRefusal`、`TestInlineRefusesAnUnrepresentableCapturedAtomBeforeWriting` 已保留。扩展 program 的 `TestRefusedFrameAdvancesNoneOfTheComposedOwners`，明确断言 scene、receipt、writer 都不推进，扩大宽度后恰好发布一次。 |
| F04 | standalone checkout 经 symlink 运行不能误判仓库或模块归属 | `ui/internal/arch/arch_internals_test.go` 的 `TestRepositoryChecksUseThePhysicalRoot` 已保留；physical root 仍由真实路径解析得出。另修本轮发现的 release 夹具路径假设。 |
| F05 | 绝对行坐标溢出不能发布部分 placement；必须包括已退休原点 | `ui/base/transcript_extent_test.go` 补齐首次、新增 source、reflow 的拒绝及合法重试，新增 `TestTranscriptOverflowIncludesRetiredRowOrigin`。`transcript_identity_internals_test.go` 新增 `TestTranscriptRetirementPreservesTheLastRepresentableOrigin`，以一行捕获验证 `MaxInt-1` 到 `MaxInt`，不分配巨大内容。失败保留 source 与原已提交 geometry，各自遵循自己的生命周期。 |
| F06 | 应用 recover 内层测量 panic 后仍完成外层 frame，也不能提交空 Transcript | `ui/base/transcript_measurement_test.go` 的 `TestRecoveredTranscriptMeasurementCannotCommitAnEmptyLayout` 已保留：旧宽度、高度、body 引用不丢，后续合法布局成功。无需重复补一个仅检查普通自动 abort 的测试。 |
| F07 | draw 回调中收到完成水位，返回 accepted sequence 后不能丢失或倒退 | `ui/core/present/present_test.go` 的 `TestWriterReceiptDuringDrawingCannotBeLost` 函数除 import 路径外保留，成功和失败 draw 分支及旧 / 新 receipt 均有断言。 |
| F08 | 同一 ANSI 输入既有超长序列错误又有 visitor 错误，两者都要保留 | `ui/core/ansi/stream_test.go` 的 `TestScannerPreservesFramingAndVisitorFailuresTogether` 已保留；分别检查两条错误、访问次数及后续不重放。 |
| F09 | draw 返回 error 和非零 accepted sequence 时，重绘必须等该输出完成 | `ui/core/present/present_test.go` 的 `TestAFailedDrawStillWaitsForItsAcceptedOutput`、`TestAFailedDrawDoesNotClaimTheFrame` 已保留；水位 6 不能释放等待 7 的帧，7 到达后完整重绘。 |
| F10 | cancellation 不能掩盖示例的真实后端 / source 失败 | `examples/agent/main_test.go` 的 `TestCancellationCannotHideABackendFailure` 与 `examples/streaming/main_test.go` 的 `TestCancellationCannotHideASourceFailure` 从 joined 扩展为 joined / wrapped；必须显示 failed 和诊断。既有纯 cancellation 成功分支保留。 |
| F11 | 只有 bare EOF 正常；wrapped / joined EOF 保留失败，已接纳字节和 pending ESC 先交付 | `ui/core/program/ingress_test.go` 的 `TestByteIngressNormalizesOnlyBareEOF`、`TestByteIngressPreservesDataAndFinalResultOrder`，`ui/core/term/term_internals_test.go` 的 EOF 测试，`extensions/ssh/input_result_test.go` 的 `TestRunNormalizesOnlyBareInputEOF` 均保留。 |
| F12 | 保存的 View、副本和子视图不能越过 publication / ownership transfer / 下一次捕获继续写 | `ui/core/grid/view_lifetime_test.go` 的 `TestCanvasRetainedViewsCannotMutateDuringPublication`、`TestCanvasFrameViewsEndAtOwnershipTransfer`、`TestCaptureDrawingViewsEndWithTheirOperation`、`TestCaptureCannotReuseAnEarlierCapturesView` 覆盖正常、panic、goroutine 退出；旧逐 Append 行的寿命已改为整次 Capture 寿命。 |
| F13 | Build 中发布也必须使用启动时的 ground 和选定色深 | `ui/core/program/runtime_lifetime_test.go` 既有 environment / ground 断言保留；新增 `TestInlineStartupPublicationUsesTheSelectedColorDepth`，检查主机 16 色、显式真彩、显式无色的真实输出，并在 Build 内改变主机返回值，防止误读之后的环境。 |
| F14 | 第一块已测得新高度，第二块 panic / goroutine 退出，不能发布半份行空间 | `ui/base/transcript_measurement_test.go` 的 `TestTranscriptRemeasurementPublishesOneCompleteRowSpace` 已保留：异常后两个旧 extent 不变，合法重试才整体更新；goroutine 退出 没有被吞成正常返回。 |
| F15 | Select / MultiSelect 的 identity 准备失败不能改变 options 或答案 | `ui/base/options_publication_test.go` 保留两控件 × panic / goroutine 退出、原选择和合法重试的断言。删除误导性的 local / controlled 重复循环：该 controlled 分支实际也是本地赋值。旧 SetBinding / Accessor / Sync 已消失，不能宣称其失败矩阵仍原样存在。显式领域控制由 `ui/base/confirm_control_test.go` 的版本、Started、只读阶段测试保护。 |
| F16 | 早先定义用于后续 Markdown 块、规范化重复定义首个优先、半份定义不能永久发布、不同文档不共享定义 | 新增 `extensions/markdown/stream_reference_test.go` 的 `TestStreamingReferencesKeepTheirDocumentMeaning`：四种分块大小，直接断言 URL、链接文本和下划线。既有 `TestStreamingFinalSemanticsEqualWholeSource`、`TestLateReferenceRevisesProvisionalContent`、`TestRendererOwnsAppearanceAndDoesNotShareDocumentState` 保留。EOF 后必须重新 Prepare 才能永久发布；旧前缀缓存协议的删除见下文。 |
| F17 | 原生图片不能静默变成永久 cells；多行捕获拒绝、panic、goroutine 退出 不接受局部前缀；临时绘制引用释放 | grid 的 `TestCaptureSettlesAllRowsTogether`、`TestCaptureRejectsNativePaintBeforePublication` 和 program 的 later-row 拒收已保留；临时存储释放补 goroutine 退出。新增 `ui/kit/transcript_publication_test.go` 的 `TestTranscriptRetainsNativeImagesThatScrollbackCannotOwn` 使用真实 kit.Image，检查 `errors.ErrUnsupported`、图片与后缀保留、live repaint 正确。旧名含 Unsupported 的测试实际上只测容量和关闭，不能替代它。 |
| F18 | 零 / stopped / known-failed runtime 在回调前拒收；只退休已接受前缀；异常后结算原阅读状态 | program 的 zero / ended 测试增加 measurement 也不得调用的断言；known-failed 覆盖保留。新增 `ui/base/transcript_publication_test.go` 的 `TestTranscriptRetirementSurvivesCaptureAbnormalExit`，覆盖先接纳前缀后 panic / goroutine 退出、原 reading owners、保留后缀和恰好一次重试。`TestTranscriptCaptureRejectsEveryOwnedReadingMutation` 揭示并保护本轮 scene 阶段修复。 |
| F19 | 同宽 Stage 也必须占用唯一 live frame；zero、过期、重复、竞争根不能绕过事务 | `ui/base/transcript_frame_test.go` 的 `TestTranscriptPlacementRequiresOneLiveFrameOwner` 完整保留这些分支、旧 geometry 及合法重试；scene 的 snapshot producer / borrow 寿命测试补充底层保证。 |
| F20 | 从仓库 symlink 或模块 symlink 工作目录调用 inventory，仍识别物理 workspace | `ui/internal/arch/arch_internals_test.go` 的 `TestModuleScriptResolvesItsPhysicalRepository` 保留，包含显式逻辑 PWD；`scripts/modules.sh` 仍是唯一可执行 inventory，没有新增模块名单。 |
| F21 | 文档工具链包含指定安全修复，不压低审计门槛 | `package-lock.json` 中 Vite 8.3.5 保留，未变更依赖或 audit threshold。旧提交的文档 CI 成功；本轮另执行锁定安装和 `npm run docs:check`，见验证记录。 |

### 4.1 消失的协议不等于未迁移的失败场景

旧 Markdown 的 Flush / Reset 用于结束并清空前缀解析器的 reference cache。当前 renderer 每次准备完整、不可变的 source snapshot；StreamDocument 拥有单个文档的 revision、EOF 和准备结果，不能把半份解析永久发布。文档之间的状态隔离有直接测试；Transcript 退休释放 payload、placement 和引用也有 retention 测试。没有声称旧“已发布前缀随即释放源码”的内存行为仍存在：当前文档在它的有界生命周期内保留完整 source，长期归档仍归应用。

新增目的地断言做过独立有效性验证：在隔离副本把共享 Renderer 的 `/first` 改成 `/broken` 后，原“流式等于整份”的测试仍通过，因为两条路径共同出错；新增测试则按真实 URL 错误失败。恢复 Renderer 后同一测试通过。这证明补回的是独立的语义约束，而不只是增加同源比较次数。

旧 Append 的可继续回调、open row、无限空 continuation 和 `io.ErrNoProgress` 也已经删除。新的 Capture 只测量和捕获一个完整、有预算限制的 Drawable，Frozen contributions 各占独立行。应保留的是预算拒绝、整次原子性、临时存储释放、终端重排及拒绝后的有效重试，而非重新引入旧循环来恢复测试名。

旧通用 accessor 允许替换任意受控值，并由 wrapper 各自同步焦点和路由。新模型将编辑交给 EditSession，关系交给 scene.Root，领域控制显式消费版本化 snapshot。旧 accessor 的读写、normalization、重绑定交叉矩阵不是当前支持的协议；其中仍适用的“通知前完成事实推进”“旧回调不能覆盖新 owner”等保证必须继续由当前 owner 的测试保护。

### 4.2 无同名新路径的 headless 文件

以下归属核对补充 Fixed 表，不将各行宣称为旧文件全部子用例的一对一证明。

| 旧 `components/headless/` 测试文件 | 当前合同与证据 |
| --- | --- |
| `binding_publication_test.go`、`controlled_state_test.go` | 任意 Accessor / SetBinding 已删除；`ui/base/confirm_control_test.go` 检查版本化领域答案、异步 Started 不写本地镜像及 projection 拒绝请求。其它本地控件并没有被描述成持续受控组件。 |
| `binding_reentry_internals_test.go`、`choice_reentry_test.go` | binding 转移协议删除；仍需先发布事实再取消 matcher，由 `ui/base/cancellation_internals_test.go` 的 `TestCollectionReplacementPublishesBeforeMatcherCancellation`、`TestOptionReplacementCancelsAfterRestoringSelection` 保护。options 异常见 F15。 |
| `container_test.go` | `scene.Group` / `Root` 统一 children、focus、测量和坐标；`TestKeysUseOneFocusAndOnlyDeclinedKeysReachParentPolicy`、`TestReorderAndUnrelatedChildrenDoNotReassignFocus`、`TestGroupMeasuresChildrenAndKeepsExplicitZeroSlots`、`TestOneElementCannotHaveTwoParentsOrRoots` 有直接行为断言。 |
| `field_focus_reentry_test.go`、`focus_reentry_test.go`、`focus_test.go` | 旧 accessor normalization 触发器消失；scene 的 `TestFocusNotificationReentrySeesSettledState`、`TestHoverLeaveCannotActivateAnElementItRemoved`，base 的 form / validation / tabs / dialog 测试检查当前焦点、检查结果和模态归属。没有把正常焦点测试冒充旧 accessor × focus 全矩阵。 |
| `frame_internals_test.go` | scene 的 `TestFrameCommitIsAtomicAndAbortDoesNotUndoModel`、`TestPanicAbortsEveryNestedSnapshot`、`TestSnapshotProducerAndBorrowLifetimeAreExplicit`，base 的 `TestScrollCandidateRefinementPublishesOnlyAsACompleteFrame` 以及 F06 / F14 / F19 保护原子提交和投影借用。 |
| `keyboard_reentry_test.go` | scene 的 `TestFocusTransferAndRoundTripEndOldKeyPropagation` 同时验证移走及移走再回来不复活旧传播；`TestPreviewCannotResumeAnAttachmentItRemoved` 验证撤销 attachment 后不继续旧路由。 |
| `pointer_region_test.go`、`pointer_routing_test.go` | 独立 PointerRegion owner 消失；`TestPointerUsesArrivalWrittenGeometryAndCurrentAttachment`、`TestCaptureKeepsForeignReleaseAndEndsOnDetach`、`TestHoverExitPreservesCaptureAndNewPressCancelsIt` 检查物理输入到达时的 geometry、foreign release、capture 和 hover。 |
| `routing_publication_test.go`、`routing_reentry_test.go` | scene 的 `TestInputReadsItsWrittenProjectionRatherThanNewAdmission`、`TestAbortedReplacementDoesNotReviveRevokedAttachment`、`TestAbnormalCallbackDoesNotClearNewerCapture`、`TestReleaseDuringPressDoesNotRestoreCapture`、`TestCancellationMayStartANewerCapture` 直接检查旧输入和旧 unwind 不能覆盖新归属。 |
| `transcript_test.go` | `TestTranscriptOwnsOneCoordinateProjectionAcrossRowsAndDrawing` 与 `TestATranscriptAnswersEveryQuestionFromOneNumbering` 检查各读模型使用相同坐标；本轮补齐的实际遗漏集中于 F05 / F17 / F18。 |

## 5. 验证记录

本轮使用 Go 1.27.3 和最低版本 Go 1.27.0，以及仓库固定的 golangci-lint v2.14.0、gofumpt 指定版本、shfmt v3.13.1、govulncheck v1.8.0、deadcode v0.50.0 和指定 apidiff。模块集合从 `scripts/modules.sh` 派生，没有维护第二份门禁名单。

已完成的定向 race 覆盖 scene、base、kit、grid、program、present、term、PTY、Markdown 和两个 cancellation 示例。release、physical root、render callback phase、panic 文档的定向架构测试通过。三个关键因果验证分别证明：旧路径假设在 symlink 下失败且错误模块仍被拒绝；旧退出顺序在单槽 writer 下失败；旧捕获阶段允许修改 selection。后两项均以同一测试在旧代码上行为失败、修复后通过为证据，不以编译失败代替回归复现。

证据版本也明确区分：退出顺序和 selection 捕获测试的修复前版本是纯 `13c87df`；foreign frame / pending candidate 的补充证明在 `13c87df` 加初版 ReadOnly 修复的隔离快照上执行，用来发现初版阶段检查仍不完整。后者的第二个用例依赖新增 ReadOnly API，不能冒充纯旧基线已能编译运行的测试。最终实现让同一补充断言通过。

最终生产代码和测试冻结后，以下检查实际执行通过。仅本报告的验证记录在测试快照之后更新，最终文档另行检查。

| 门禁 | 本轮实际结果 |
| --- | --- |
| Go 1.27.3 全模块 `go vet ./...` | 7 / 7 通过 |
| Go 1.27.3 全模块 `go test -race -count=1 ./...` | 7 / 7 通过，包含真实本机 SSH、Mermaid 和 PTY 夹具 |
| pinned golangci-lint | 7 / 7 通过，UI 最终输出 `0 issues.` |
| gofumpt / shfmt | 全仓 Go 和脚本均无格式差异 |
| 独立模块 `GOWORK=off go test` | 7 / 7 通过；仅在隔离副本添加仓库脚本派生的 local replacements |
| 独立模块 `go mod tidy -diff` | 7 / 7 无差异；主树 manifest 未修改 |
| `go work sync` 与完整副本比较 | 无任何文件内容变化 |
| 最低版本 Go 1.27.0 的 Linux 原生测试 | 7 / 7 通过 |
| Go 1.27.0 的 Windows / Darwin 源码和测试编译 | 14 / 14 通过；编译测试二进制，没有在这两个系统执行测试 |
| 最终源码 reachability | Linux / Darwin / Windows 全 workspace 通过 |
| 最终源码 govulncheck | 7 / 7 通过，无可达漏洞；模块级提示见下文 |
| API ledger / workspace inventory | 命令通过；六个新公开模块没有已发布基线，API 比较按原合同跳过，不能据此宣称兼容性已证明 |
| Markdown 主动 fuzz | 30 秒、117,043 次执行，通过；全模块测试同时回放已提交语料，本轮没有重复主动运行其余所有 fuzz 目标 |
| `npm ci`、`npm run docs:check` | 锁定安装、安全审计、拼写、Markdown、架构图一致性及 VitePress 构建通过；npm audit 为 0 vulnerabilities |

最初执行时，默认隔离环境禁止本机监听端口和访问 Go 校验 / 漏洞数据库；这些阻碍在允许对应隔离测试及只读访问后重跑解决，没有通过跳过测试或关闭校验制造成功。

govulncheck 的 SSH 结果另包含 `GO-2026-5932` 模块级提示，涉及所需 `golang.org/x/crypto` 中未导入的 `openpgp`：受影响的可达 symbol 为 0，已导入 package 为 0。这里的结论是门禁通过、没有可达漏洞，没有把它写成整个依赖模块集合不存在任何 advisory。

macOS / Windows 的原生执行仍须由应用补丁后的远端 CI 确认。交叉编译和 Linux 的 symlink 回归能够分别验证可编译性及路径身份修复，但不代替真实平台执行；本轮没有将新的本地通过结果冒充成已重新推送的远端绿灯。

## 6. 增量交付与审查边界

此次以 `13c87df` 为基线交付一个包含多个提交的 patch，避免再次混入目录迁移。步骤如下，底层能力先于组件组合落地。

| 提交顺序 | 改动边界 | 主要验证 |
| --- | --- | --- |
| 1 | release 夹具检查模块身份 | 直接路径、symlink 和错误模块拒绝 |
| 2 | 最终 inline 帧到清理帧的交接 | 单槽 writer、Drain 失败及 receipt 结算 |
| 3 | scene 只读借用、frame 生产者与候选阶段 | 外部调用方的阶段、寿命、异常和 pending 排他测试 |
| 4 | base / kit 复用 scene 所有者 | 捕获原子性、真正 mounted Element、候选滚动状态、对应夹具和双语合同 |
| 5 | 独立语义回归及完整审计 | Markdown 目的地、启动色深、取消诊断、临时引用释放与 21 项对照 |

补丁按提交顺序重放，逐步验证各边界，最终文件树再与已验证工作区比较。每一部分解释自己的失败条件与验证，不把修复藏进新的总体重写。

UI 仍为单一发布模块，import 图继续约束内部层级。是否在 1.0 之后拆出可单独版本化的 core，需要真实独立消费者和版本节奏作为依据，本轮不为假设中的需求改变模块边界。
