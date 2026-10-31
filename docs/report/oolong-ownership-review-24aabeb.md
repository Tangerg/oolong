---
title: Oolong 所有权复审（24aabeb）
description: 核对已应用的版本，记录能复现的所有权分叉、修复、迁移和验证边界。
contentType: Reference
---

# Oolong 所有权复审（24aabeb）

审计日期：2026-10-31。基线是 [24aabeb601739df5eb192f0a9956f77056da7e60](https://github.com/Tangerg/oolong/commit/24aabeb601739df5eb192f0a9956f77056da7e60)，Git tree 为 `fffe7afc873875f21a39edb383b9a3d96af85755`。它与上一轮交付的最终树完全相同。[主 CI](https://github.com/Tangerg/oolong/actions/runs/41214766298) 的 20 个任务全部成功，包括 macOS 和 Windows 原生测试；[文档部署](https://github.com/Tangerg/oolong/actions/runs/41214766346) 也成功。这些远端结果属于补丁之前的基线。

## 范围和判定方法

基线有 7 个 workspace 模块，673 个跟踪文件，其中 241 个生产 Go 文件分布在 55 个目录，另有 314 个测试文件。按实际包边界检查了 core、base、kit、automation、uitest、五个可选扩展、示例、仓库命令及发布工作流。模块名单仍由仓库脚本派生，这里只记录本次范围。

审查从 [AGENTS.md](https://github.com/Tangerg/oolong/blob/main/AGENTS.md) 的规则出发，沿写入入口、回调、发布点和拒绝路径追踪事实。重点问三件事：谁能推进事实，失败或重入时哪部分已经生效，下一个消费者能否把旧投影误当成当前状态。全仓机械检查覆盖所有模块；人工追踪和反例集中在下面列出的边界。通过测试不能证明所有分支无缺陷。

## 已修复的证据

| 所有者 | 修复前的具体行为 | 修复及可追查的回归 |
| --- | --- | --- |
| 自动化观察证据 | 最后一页只包含一个按钮时，Locator 可能把它当成全局唯一；长名称显示为短前缀时可触发错误动作。另一方向上，查不到隐藏的完整名称会误报确定缺失。 | 观察记录哪些文本字段被截断、页面是否遗漏节点。匹配区分已排除、可能、精确；证据不足返回 `ErrIncompleteObservation`。见 `ui/automation/evidence_test.go`。 |
| scene 可用性 | Semantic 可声称节点禁用，同时该节点的动作仍能执行。 | 节点和动作状态从 scene 的实际资格派生；应用通过 Enabled 表达节点是否可用。测试同时比对观察结果与实际 Invoke。 |
| Markdown blocks 与几何 | 一个嵌入块在测量中替换文档，旧测量完成后仍能把旧行写入新文档缓存；多个合法子高度的和也可能溢出。 | 每版不可变 blocks 拥有自己的布局缓存。每个读者持有完整的宽度投影；发布前检查高度和分隔行的总和。拷贝后追加只允许一个分支复用尾部，其他分支分离。见 `extensions/markdown/document_projection_test.go`。 |
| 可复制文本 | 截断后缀里的控制字符和畸形字节不占显示宽度，却能出现在 `text.Truncate` 返回的字符串中。 | 源文本和后缀使用同一套可绘制字符规则。测试直接比较返回值，而非只检查最终画面。 |
| 共享文档与历史 | 重复文字存在多组相同的前后缀，由最终字符串反推会把开头插入解释为末尾插入，其他编辑器的光标随之映射错误。独立的输入合并状态还可能在其他视图清理历史后吞掉一次 undo。 | 增量编辑提供真实字节区间；历史持有正反映射、输入合并归属。只有缺少编辑意图的全文替换才比较完整值。订阅者获得提交后的标记和选择。见 `ui/base/edit_session_atomic_test.go`。 |
| 元素插入与重入 | 插入占位文本先触发订阅者，之后才安装元素标记；订阅者重置文档后，旧操作仍可修改新文档。剪贴环续写有相似问题。 | 插入、元素身份、历史和续写状态在通知前结算；回调后不再由旧调用补写。 |
| 共享密码策略 | 一个编辑器遮住密码，另一个共享视图仍可在 AX 中披露明文；直接 Session.Reset 还可写入换行。 | mask 和单行策略归 EditSession。所有视图与写入入口遵循同一策略；仅更换掩码唤醒视图，不虚增文本 revision。需要独立披露策略时使用不同会话。 |
| scene 子关系 | Settings 先取内部 List、后取外层 Element 可产生两套身份。Panel、Viewport、Dialog 的 child 字段可能与实际挂载关系分叉：焦点谓词 panic 之前新 child 已挂载，查询仍返回旧 child。 | 让控制器从同一个 Element 开始组合，已挂载关系直接查询 Group。关闭的 Dialog 只保留待挂载内容。已生效的模型变更先发布 revision 和唤醒，再结算焦点。见 `TestSettingsListCannotCreateASecondSceneIdentity` 和 `TestChildQueriesFollowTheActualRelationDuringFocusSettlement`。 |
| 虚拟窗口 | 没有 builder 时测量范围非空、实际 children 为空，Prepare 仍按项目数分配矩形而 panic。高度细化会在估计值上过早裁掉原始滚动请求。 | Range 是有界内容投影；Group 单独拥有挂载关系。矩形数取实际 children，测量细化始终从原请求计算。见 `ui/base/virtual_test.go` 的无 builder、焦点观察、视口细化用例。 |
| 输出责任与写入水位 | 生产者拿到完整 publisher 后可以自行调用 Written；保存它还能绕过会话停止和 writer 失败检查。仅获取句柄也可能把已经写完的 receipt 降为 UnknownPartial。 | `runtime.Publisher()` 是具体生产者能力，只接纳内容和订阅进展。真正的 writer 水位先结算；只有其余未确定输出进入不确定状态。运行时预算不再接受随后会被覆盖的宽度和颜色环境。见 `ui/core/program/producer_test.go`。 |
| scene 运行时权限 | `Runtime.Scene()` 公开完整 Root，普通调用者可伪造物理写入或私下 Prepare/Close；在 Build 取到的 nil 也不会随挂载生效。 | 会话句柄只开放 Observe、Invoke 和 Changes；未挂载、活动和停止状态有明确结果。独立 compositor 仍可自行拥有真正的 Root 和 Publisher。 |
| 任务、重绘与入口寿命 | writer 拒收时显式重绘仍可能回复成功；包装或合并的取消错误被归为纯取消；已停止的 ByteIngress 还通过真正的 reservation 留住队列。 | 重绘回复实际拒收原因，复合失败保留诊断；结算时释放实际持有队列的引用。相应用例在 program 的 compositor、task 和 ingress 测试中。 |
| 两个流式示例 | 第一轮在 UI 侧提前结束并启动第二轮后，旧入口队列中的数据或 EOF 仍交给当前 run，可能污染新文档或把新任务改成 cancelled。 | 字节、结束和准备结果绑定产生它们的 run 或 document 身份。旧回调不能替继任者结算。见 `TestLateIngressCannotFinishTheNextAgentRun` 和 `TestLateIngressCannotChangeTheNextReply`。 |
| workspace 清单 | 生成替换指令时丢弃 marker 后的整个文件后缀，合法的后续 use 指令被删除；脚本另行解析 use 块还会漏掉独立的 use 行。 | 使用 Go 解析的成员事实；生成器只替换它拥有的 replace 指令，保留其他指令，以及受管片段之外的原始字节。LF、CRLF 的真实文件都能同步并通过第二次检查。见 `TestSyncPreservesMembershipAfterTheGeneratedDirective`。 |
| 标签与 API 账本 | 从标签字符串截取目录不能识别实际的主版本子目录。同一次全仓校验还可能给两个独立发布的模块套用同一个迁移章节。 | 从公开模块集合按唯一的正向标签规则解析 owner；全仓按模块选择章节，显式发布只核对该标签的模块。见 `TestTagContractSelectsTheDirectoryWhoseManifestOwnsTheTag`、`TestIndependentModuleLedgersForWorkspaceAndOneRelease`。 |
| 已发布 API 的对比基线 | 继承显式开发 GOWORK 时，真实 apidiff 的旧导出和新导出可能都解析到新代码，删除符号没有报告。 | 旧导出固定在独立的已发布模块图，新导出固定到当前 checkout 的 workspace。真实故障注入能重新报告被删除的符号；见 `TestBaselineAndCurrentExportsResolveDifferentModuleTrees`。 |

## 迁移

- 永久发布：用 `runtime.Publisher().TryPublish(body)`，处理 admission error，并从 receipt 观察后续结果。运行时配置使用 `publication.Limits`；几何属于主机和 compositor。
- 代理调用：`runtime.Scene()` 是受会话寿命约束的语义端口，可以在 Build 保存，挂载后使用。不要从应用端推进 Root 的帧或 writer 水位。
- 名称查询：`SemanticNode.TruncatedText` 用字段标志替代原布尔值；`Observation.Next` 表示下一页，`Observation.Truncated` 表示该页缺少节点。证据不足和确定不存在是不同结果。
- 编辑会话：共享会话也共享密码策略。显示差异不改变每个视图独立的 caret、selection、scroll；不要依赖另一个共享视图来显示明文。
- 发布工具：使用 `scripts/module-tag.sh --resolve TAG` 查找发布 owner。全仓 ledger 默认按模块选择，显式发布由 `-release TAG` 选定模块和章节。

## 验证边界

核心修复有修复前的行为失败与修复后的回归断言。未将编译失败算作旧版故障证明；未因不可用真终端、平台或不存在的发布基线而伪造验证结果。模块发布脚本在隔离 fixture 中执行，没有推送标签。

| 检查 | 实际结果 |
| --- | --- |
| Go 1.27.3：vet、race、固定版本 lint、govulncheck | 全部 7 个模块，各项 7/7 通过。SSH 无可达漏洞或已导入的受影响包，有一项未使用包的依赖模块级公告。 |
| 模块脱离 workspace 独立测试、`go mod tidy -diff` | 各 7/7 通过。仅在独立副本中生成本地替换，实际 manifest 未改。 |
| workspace、API ledger、可达性、格式 | 通过。6 个新公开模块无已发布基线，真实仓库 ledger 按合同跳过差异比较；漏报修复另外使用有发布基线的 fixture 验证。 |
| Go 1.27.0 最低版本 | 全部 7 个模块 Linux 原生测试通过；Windows、Darwin 共 14 项生产代码和测试交叉编译通过。 |
| 主动 fuzz | 编辑器状态、共享会话光标映射、Markdown 分块语义三个目标各用 30 秒预算，合计 113,602 次执行，全部通过。其余目标的已提交语料随全仓测试回放。 |
| 文档 | npm 锁定安装、依赖审计、拼写、Markdown、架构图和 VitePress 构建通过。 |
| 分步验证 | 前 6 个实现提交在各自的干净 checkout 上通过受影响包测试。第 7 个提交只包含审计记录及入口。 |

集中测试的首个隔离快照有 77 项检查，其中 76 项通过；可达性检查指出 Markdown 的公开 Len 缺少可执行调用方覆盖。现有回归现同时核对重入替换后的 **2 个逻辑块与 3 个物理行**，最终三平台可达性检查通过，未删 API 或降低门槛。CRLF 修复后又针对最终工作区工具执行 race、lint、最低 Go 版本测试和 Windows／Darwin 编译。示例中两处测试夹具改为确定性调度，最终测试在精确旧版本上因行为失败，在修复版本的 race 下通过。

按 Go AST 统计，Test 声明从 1,921 增至 1,952，Fuzz 声明保持 23，合计 1,944 → 1,975。数量只作审查参考；同一缺陷的旧版失败、修复后成功以及拒绝后的状态才是验收依据。

原生 macOS / Windows 的远端成功属于基线；新补丁仍须在推送后由对应 runner 执行。
