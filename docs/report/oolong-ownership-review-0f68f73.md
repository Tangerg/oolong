---
title: Oolong 所有权复审（0f68f73）
description: 已应用版本的核对、可复现的所有权冲突、破坏性迁移及验证范围。
contentType: Reference
---

# Oolong 所有权复审（0f68f73）

审查基线是 [0f68f7348fac796edbb1fff3d4540046840cb08a](https://github.com/Tangerg/oolong/commit/0f68f7348fac796edbb1fff3d4540046840cb08a)。Git tree 是 `0fcdb6925a9dba373c8a84843d5d72771a587a71`，与上轮补丁交付树完全一致。上轮修复已经应用，本报告只记录本轮增量。[基线 CI](https://github.com/Tangerg/oolong/actions/runs/41254604556) 与[文档部署](https://github.com/Tangerg/oolong/actions/runs/41254604553) 均已成功，主 CI 的 20 项任务全部通过；这些远端结果不代表本地新补丁。

## 范围和依据

依照当前仓库的 [AGENTS.md](https://github.com/Tangerg/oolong/blob/0f68f7348fac796edbb1fff3d4540046840cb08a/AGENTS.md) 和 [PROJECT_RULES.md](https://github.com/Tangerg/oolong/blob/0f68f7348fac796edbb1fff3d4540046840cb08a/PROJECT_RULES.md)，从实际包和导入关系梳理所有工作区模块。基线有 691 个跟踪文件；检查覆盖 core、base、kit、automation、uitest、可选扩展、示例、构建与发布工具。人工追踪以能被调用者观察到的失败为中心；没有把全仓工具检查等同于逐行证明。模块名单由仓库脚本派生，没有增设架构清单。

本轮用三个问题定位第二个所有者：一份投影能不能反过来改变正式结果？一次拒绝、取消或失效的操作是否仍能推进状态？一个业务结果是否由另一个层次的成功来代替？以下证据分别来自公开 API、小型程序、真实子进程管道、网格、事件路由和已提交的回归测试。书面约定只读、命名为“快照”或测试通过，本身不能回答这些问题。

## 输入、场景和自动化

| 事实 | 基线的失败现场 | 修复与可追查的验证 |
| --- | --- | --- |
| 挂载身份由 Root 发放 | 保存 A 的旧目标，卸载再挂载同一元素，把旧目标公开的 Generation 加一，旧句柄便能成功执行新挂载上的动作。 | Target 的节点、代次、部件改为只读访问器。身份仍可比较、记录，但不能自行续期。基线探针实际得到一次调用及 nil 错误；保留原有卸载、重挂、替换和代次检查。 |
| 代理必须在被操作会话自己的序列上运行 | Driver 可以同时接受 A 的场景和 B 的 dispatcher。独立双所有者探针从 B 成功读了 A，构造、调度、观察均没有报错。 | `automation.NewDriver(runtime)` 从同一会话派生两项能力；独立实现也由其会话提供它们。真实 program 测试覆盖 Build 时绑定、挂载后观察和停止后的拒绝，见 `ui/automation/session_test.go`。 |
| 到达时的输入与预算 | `*input.Paste` 等可变指针既能被保留、事后改写，也可被当作仅占一单位的输入；带长属性的终端答复没有计入载荷。测试宿主还会先报告发送成功。 | `input.ByteSize` 是规范事件值及其计费的唯一判定，覆盖真实宿主、场景和测试入口。无效表示和有效但超限分开报告。见 `ui/core/program/input_internals_test.go`、`ui/uitest/lifetime_internals_test.go`。 |
| 物理写入确认对应的几何 | 连续接纳布局 1、2、3，只确认第 1 帧时，进度报告已写序列 1，却仍无第 1 帧可用于命中。几何显式失效后，AX 仍报告旧 bounds 可用。 | 待确认布局按写入序列保留；同一水位上的候选合并。确认只发布它实际覆盖的最新布局，不给跳过的帧编造时间。独立预算满时在调用绘制器之前拒绝；失效的 bounds 在重新提交前不可用。见 `TestIntermediateWatermarkPublishesExactlyItsAdmittedLayout` 等。 |
| 指针属于原内容和坐标帧 | 同一节点由文档 A 换成同修订号的 B，旧点击仍可触发 B。父预览中换正文、让几何失效，甚至在捕获取消回调里准备新帧，旧事件也会继续投递或抛出错误。 | 检查内容标识及修订，并在可能执行用户代码的路由边界重新核对目标、几何与所属阶段。未改变内容的正常点击仍然送达；处理器内部换掉内容后也不能凭旧事件取得焦点或捕获指针。见 `TestPointerRouteRequiresTheOriginalContentAtDelivery` 及 `ui/core/scene/reentry_test.go`。 |
| 纯投影不能结算或修改候选帧 | 父元素先暂存值 1，自定义布局回调可合法拿到旧暂存句柄并把最终值改成 2。确认时间回调也能提交或撤销另一个待结算候选；焦点通知新建候选后还能开始处理输入。 | 布局、语义和时钟查询的只读边界落实到暂存、候选结算和路由入口。正常绘制仍可暂存；纯阶段中的变更在发布之前被拒绝。见 `ui/core/scene/authority_test.go`、`settlement_test.go`。 |
| 活跃弹窗拥有输入及对应关闭事件 | 外层预览抢到弹窗的 Esc。另一个方向，即使代理成功调用语义 Dismiss，业务上等待的审批仍然悬挂；流式示例的未提交内容也没有正确释放。 | 预览与冒泡使用同一弹窗边界。弹窗关闭后从 `Dismissed` 通知业务所有者；审批回复一次，未提交的请求撤销并保留草稿。键盘、直接关闭和语义动作共用同一条业务路径。见 agent 和 streaming 的 `TestSemanticDismiss…`。 |

## 文本、组件和终端输出

| 事实 | 基线的失败现场 | 修复与可追查的验证 |
| --- | --- | --- |
| 已解码文本和流尾 | 改写 `Decoder.Open()[0].Text` 后，`Flush` 竟交出调用方伪造的文本。普通文本以半个 UTF-8 字符结束时，已读字节像未完成转义一样被静默丢弃。 | Open 返回独立 span 切片；解码器保有自己的输出。EOF 区分不完整文本与转义：前者按已有坏字节规则产生替换字符，后者仍不执行、不显示。见 `ui/core/text/decode_test.go`。 |
| 流文档的完成与订阅 | 先注册的观察者已见 Final，但同一次通知中的 transcript 条目仍显示 Live 和旧正文；初次挂载的谓词异常还可能留下永不接收文档更新的条目。终态观察者抛错会吞掉其后的订阅。 | 文档拥有正文、修订与完成结果，transcript 直接投影，并在公布条目前订阅。回调异常仍传播，未取消的后续订阅者继续收到该次完成。见 `ui/core/content/stream_test.go`、`ui/base/transcript_authority_test.go`。 |
| 可见文本、复制、选区与拖动 | 已提交布局在查询 Rows 时又向外部取文，和显示/命中不一致，甚至能复制块高度以外的隐藏行。两个 transcript 有相同数字 ID 和文本端点时，A 的选区能复制 B 的不同中段。结束选择后另一份拖动标志仍消费事件。 | 只从按实际块高准备的文本映射取行、命中和复制。端点绑定 transcript 身份；Selection 独占拖动状态。见 `TestTranscriptQueriesReadOnlyCommittedRows`、`TestTranscriptPointsCannotResolveInAnotherTranscript`。 |
| 搜索结果属于计算它的布局 | resize 后新帧仍在旧物理行列画高亮；同宽替换正文后仍能跳转旧匹配。旧任务迟到还可覆盖新结果。 | 匹配绑定现有的 transcript 身份、内容修订及宽度。安装过期结果返回 `ErrStaleMatches`，不会覆盖当前结果；绘制查询传入本次候选布局。见 `ui/base/transcript_authority_test.go`、`ui/kit/transcript_test.go`。 |
| 外观和语义动作是行为的投影 | Completion、Filter 将自己持有的匹配偏移切片交给自定义绘制器；它能在绘制结束后改写下一帧。取消旧快捷键时若重入并换了候选，旧接受回调一次也没执行，AX 却报告 Applied。 | 给外观回调可独立保留的偏移；用行为所有者的实际接受结果生成语义回执。见 `ui/base/render_receipt_test.go`、`cancellation_internals_test.go`。 |
| 编辑通知与主题唤醒 | 已完成的编辑可因第一个订阅者退出而不通知后继；重入后注册的订阅者可能收到早于自己的变化。共享编辑器或主题的第一个场景谓词异常，其他已受影响的根不再刷新。 | 在提交时确定本次交付对象；异常和重入不能重复或吞掉已提交消息。主题更新安排唤醒所有相关根。见 `ui/base/subscription_delivery_test.go`、`ui/kit/theme_scope_test.go`。 |
| 画布存储与原生绘制输出 | 缩小 Surface 后，旧 View 仍允许对已删除格子写入，Fill 可越界崩溃。原生 Painter 保存 writer，在另一个 painter 回调里用它写入，旧字节真正进入了新帧。 | 旧视图保留布局大小，但实际可写区与当前存储取交集。原生 writer 只在当前 Paint/Erase 回调有效；逾期写入返回 `io.ErrClosedPipe`。全屏与 inline 都有输出字节断言，见 `ui/core/grid/owner_boundary_test.go`。 |

## 调度、关闭、扩展与测试工具

| 事实 | 基线的失败现场 | 修复与可追查的验证 |
| --- | --- | --- |
| dispatcher 与字节入口的实际接纳 | Invoke 在认领前已取消，回调未执行，模型修订仍从 1 增到 2。dispatcher 已停止后，入口还能返回写入成功、部分成功或 EOF 成功。 | 认领并执行的任务才报告效果；取消不产生变更。字节、结束标志的接纳由真实队列预留及其寿命决定，移除第二份 posted 状态。见 `ui/core/program/invoke_internals_test.go`、`ingress_internals_test.go`。 |
| 终止是最后一次清理机会 | 第一个 Detached 回调 panic 或主动结束 goroutine，后继节点永久带着旧挂载的清理义务；发布回执也可能永远停在 Accepted。取消捕获的回调关闭 Root 后，失效路径还能再次关闭同一广播通道而 panic。 | 在进入用户回调前公布几何失效；会话停止分别结清已接纳发布及附件清理。普通通知中断可以续送；关闭时不依赖另一个未来轮次。见 `TestClosingRootFinishesEveryRevokedLifetime` 和 `TestStopSettlesEveryPublisherWhenUserCleanupUnwinds`。 |
| agent 的运行结果 | 最终 Markdown 准备失败，示例仍显示完成；前一次源超限还能把失败带进下一次独立运行。 | 由发起命令接收自己的错误。是否完成取自本次文档最终修订的源错误和渲染错误，不把“结果属于这个修订”误当作“渲染成功”。见 `examples/agent/conversation_test.go`。 |
| Mermaid 诊断的内存限额 | 配置保留 3 字节，使用真实 OS 管道复制 27 字节竟全部留下：嵌入的 Buffer 提升了另一条写入口，绕过有限额的 Write。 | 只暴露带限额的写入能力。仍消费全部管道输入，以免阻塞子进程；只保留预算内的诊断字节。见 `TestBackendPipeCannotBypassDiagnosticLimit`。 |
| 测试终端的显示证据 | 更改滚动区域后，旧模拟器把下一个字符断言在第三行；实际应该归位到首行。未建模的模式切换或窗口操作也可被跳过，产生文本匹配的假阳性。 | 按终端规则处理滚动区域与光标归位，区分不支持的显示操作，不再假装已模拟。见 `ui/uitest/ptytest/screen_test.go` 及 `ui/uitest/programtest_internals_test.go`。 |

## 调用方迁移

1. **自动化与目标：**用 `automation.NewDriver(runtime)`。从当前挂载获取目标，使用 `target.Node()`、`Generation()`、`Part()` 读取身份，不再从数字组装目标。
2. **搜索：**保存 `source := transcript.Layout()`；对 `source.Rows(...)` 搜索，将结果传给 `transcript.SetMatches(source, matches)`。收到 `base.ErrStaleMatches` 时用新布局重算。绘制时向 `MatchesIn` 和 `CurrentMatch` 传入正在绘制的布局。
3. **宿主和合成器：**提交 `input.Key`、`input.Paste` 等值。非法载体与超出字节预算是不同错误。自定义合成器在写端推进时报告 `Wrote`；待确认帧达到 `MaxPendingFrames` 时等待确认，再调用 `Prepare`。
4. **弹窗：**将应在弹窗内生效的快捷键安装到该作用域。键盘、代理及程序化关闭都必须结算的业务，绑定到“已经关闭”的通知；业务请求仍由应用持有。

## 验证边界

这轮没有为相似语法、不可变快照或不同消费者恰好相同的数值引入新的统一管理层。输入历史、待写布局、业务结果、发布回执各有独立含义，不要把其中一个的成功写成另一个已经完成。Go 测试和 fuzz 入口从 2,011 增至 2,053，净增 42；这些数量不代表测试覆盖率。针对性负例、全仓门禁和补丁回放是三种不同证据。

在新克隆中从基线按顺序检出 13 个代码及契约提交，对每个提交编译其影响的模块及测试；涉及共享 UI 时编译全工作区。85 次“提交 × 模块”检查全部通过。这验证中间版本可构建；全套行为测试针对最终源码运行。针对编辑器状态、Markdown 分块与文本跨 span 的三个主动 fuzz，各用 30 秒预算，共执行 258,890 次，全部通过；其余已提交语料随 Go 测试回放。

Go 1.27.3 下，七个模块分别通过 vet、Linux race、固定版本 lint、govulncheck。SSH 有一项依赖模块级公告，但没有涉及导入的受影响包或可达调用。可达性、格式、workspace 清单和 API ledger 检查通过；当前模块路径没有可对比的已发布 tag，不能把跳过的 ledger 比较记作“API 兼容”。首次运行中，架构检查发现新的关闭回调缺少阶段分类、候选帧内部的异常防护未关联公开契约；两项均补充了登记，未放宽原检查。

关闭 workspace 后，七个模块分别通过独立测试及 `go mod tidy -diff`；隔离副本的 workspace 同步未产生漂移。Go 1.27.0 的 Linux 原生测试以及 Darwin/amd64、Windows/amd64 的交叉编译也全部通过。最终交付源码的仓库与模块检查合计 77 项，失败 0 项。

本机使用 Linux，实际运行了带竞态检测的测试以及真实子进程管道。macOS/Windows 的本轮本地证据是交叉编译，不能代替原生运行。Mermaid 的管道与 Node 测试也不等于运行官方 Chromium。未向 GitHub 推送本轮修改。
