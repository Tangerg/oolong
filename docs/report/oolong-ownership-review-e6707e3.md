---
title: Oolong 所有权复审（e6707e3）
description: 已应用版本、实际失败场景、修复所在的所有者、调用方迁移和验证边界。
contentType: Reference
---

# Oolong 所有权复审（e6707e3）

基线：[e6707e39006557a823c29f1e41bc4e246ae97488](https://github.com/Tangerg/oolong/commit/e6707e39006557a823c29f1e41bc4e246ae97488)。Git tree 为 `3cb8d228d02b316a3f834b3fac2b7e175a4bba45`，与上一份补丁应用后的最终文件树相同。[基线 CI](https://github.com/Tangerg/oolong/actions/runs/41250638176) 的 20 项工作全部成功；[文档部署](https://github.com/Tangerg/oolong/actions/runs/41250638177) 也成功。远端结果属于基线；本轮补丁的验证单列于文末。

## 判定方法与范围

以仓库的 [AGENTS.md](https://github.com/Tangerg/oolong/blob/e6707e39006557a823c29f1e41bc4e246ae97488/AGENTS.md) 和 [PROJECT_RULES.md](https://github.com/Tangerg/oolong/blob/e6707e39006557a823c29f1e41bc4e246ae97488/PROJECT_RULES.md) 为准。基线共 680 个跟踪文件，其中 241 个生产 Go 文件、320 个 Go 测试文件。模块从仓库脚本派生。按实际边界检查了 core、base、kit、automation、uitest、五个可选扩展、示例，以及构建和发布脚本。逐行追踪集中在下表的状态转移、失败和重入路径；全仓门禁覆盖范围更广。按同一口径计数，Go 测试及 fuzz 入口从 1,975 增至 2,011，净增 36 个。入口数不能代替行为覆盖率；下面列的是它们能拦住的具体失败。

判断一个重复值是不是缺陷，只问它能否独立推进同一事实。不可变的到达帧、输入历史、回执和已经交给写端的快照有各自的用途，应当保留。可写的第二个焦点、旧回调对新挂载的写权限、先发出的通知所见的半次操作，则能让调用方实际观察到矛盾。本轮用这些矛盾构造反例。

## 证据与修复

| 事实的所有者 | 原版本可观察到的故障 | 新行为与回归证据 |
| --- | --- | --- |
| Root 的挂载寿命 | 首次焦点谓词 panic 或主动结束所在 goroutine，构造者拿不到根，元素却仍被它持有，借出的附件也未结束。 | 未返回的根撤销临时挂载，不发布它的生命周期通知；同一元素可以重试。见 `ui/core/scene/lifetime_test.go`。 |
| 每次挂载自己的清理 | 旧根延迟交付 blur，会清掉新根中同一个按钮刚刚接受的快捷键前缀。首个回调嵌套结清或抛出时，其他移除回调还会插队或整个丢失。 | 回调按产生次序逐个认领；异常后只恢复未送达的后缀。上一代实际欠下的回调结束以前，该元素不能重新挂载。不相关的元素不必等待。见 `remount_cleanup_test.go`、`lifetime_test.go`。 |
| Root 的焦点与弹窗返回位置 | 先关下层再关上层，焦点回到第一个控件而非原按钮；重排仍在的两个弹窗，两层都忘掉最后聚焦的子项。 | 按挂载身份保留各层的返回位置。移除一层接回原调用者，重排保留层内焦点。见 `TestClosingCoveredModalKeepsOriginalReturnAddress`、`TestReorderedModalRetainsFocusOfEachSurvivingLayer`。 |
| 事件的输入、目标和命令权限 | 已把焦点转给 B 的旧按键回调可直接夺回 A；一个已返回且未认领按下的处理器仍可事后抢占指针捕获。事件公开字段也允许改写投递的输入或目标。 | 输入和目标改为只读访问；焦点和捕获命令核对当前回调、输入轮次和附件，键盘命令还核对焦点轮次。读到旧输入不意味着可以继续发命令。见 `ui/core/scene/reentry_test.go`。 |
| 语义分页的遍历 | 两个根的修订号碰巧相同，或在同根换子树、深度，原游标仍可跳过若干节点并给出貌似正常的下一页。 | 游标绑定实际场景、修订、解析后的子树及深度；混用返回 `ErrObservationChanged`。改页长、改文本预算或换成同一子树的等价定位方式仍有效。见 `continuation_test.go`。 |
| scene 的实际焦点 | 第一行编辑器已聚焦，第二行未获焦编辑器仍把真实网格光标画到第二行；未挂载的列表报告已聚焦。 | 删除编辑器和列表的可写焦点镜像；单行和多行都读 `Element.Focused()`。失焦通知只清理输入。见 `ui/base/focus_projection_test.go`。 |
| 控件自己的行、选择和子关系 | List、Select、MultiSelect、Table、Filter、Completion 通知过早，焦点谓词能读到新旧混合值。Tabs 先改选中项再换面板，谓词异常后可永久留下“选中 B、仍挂载 A”。 | 先完整安装行、选择和路由身份，再通知。Tabs 的父组预检直属子组、重复认领、根及身份预算，再一次安装标签栏和当前页面。合法重排不重新挂载。见 `collection_publication_test.go`、`tabs_test.go`、`group_relations_test.go`。 |
| 表单的收集、校验和领域确认 | 命令行问完 A，回调换成 B，表单仍可提交没问过的 B；较早答案被后来回调改变仍可提交；受控确认已拒绝或仍在等待，回答 yes 却以 false 成功。过期校验失败或取消错误也可污染后继提交。 | 收集和提交共用一次有身份的操作；成功和失败的校验都检查对应答案。区分输入错误、请求不可用、请求已启动及答案已经生效。见 `ui/kit/ask_receipt_test.go`、`ui/base/form_receipt_test.go`。 |
| 子关系的画法与控件配置 | 表单把自己的画法永久写进字段，移出后普通表单仍调用旧画法。Slider 修改标签、步长或未夹限当前值的范围，等待 `Changes` 的读者不会被唤醒。 | 表单画法属于 `Item.Paint`，保留字段自己的画法；Slider 由真正的配置改变唤醒观察者。见 `TestFormPaintBelongsToTheFieldRelation` 和 `slider_test.go`。 |
| 终端显示权与设置 | Hand 已交出终端，子程序内关闭父会话仍二次弹出标题和键盘模式、覆盖子程序刚设好的终端属性。Linux 文件描述符耗尽时，启动可先进入备用屏，再因资源申请失败而无法恢复。 | 先取得可失败的启动资源再发进入序列；串行交接真实显示权，已交出的父会话不再改动子程序的终端。见 `ui/core/term/terminal_test.go` 中的真实 PTY 测试；文件描述符不足场景另用隔离子进程复现。 |
| 输入、探测与几何的边界 | 空读改写 Esc 截止时间；无效鼠标坐标能产生左上角点击；启动属性结束标记之后、同次 read 中的消息被探测器吞掉；像素不足时可报告有效的零宽字符单元。 | 截止时间归输入流，鼠标坐标必须是完整有效的报告，标记归协议而非读调用，像素和网格从同一次窗口测量派生。无法设置预算时不发可选探测。见 `ui/core/input`、`ui/core/term` 对应回归。 |
| 文本解码与链接边界 | 两个相邻的非法字节，一次 read 和两次 read 得到的替换字符数不同；带外层括号的 IPv6 URL 可能丢掉地址自身的闭括号。 | 逐个非法字节按同一规则清洗；识别链接时保留地址内部配对括号。见 `TestMalformedStreamTextIsIndependentOfReadBoundaries`、`ui/core/link/link_test.go`。 |
| 发布者的终态通知 | 第一名订阅者在回调中关闭发布者，可让最终失败状态没有下一轮通知；终态通知中首个回调抛错会吞掉后面的订阅者。 | 一次只认领一个订阅者；收口状态另有一轮交付，异常后从后继恢复。见 `TestCloseFromListenerStillPublishesTheFinalState`、`TestInterruptedFinalNotificationResumesWithTheNextSubscriber`。 |
| 示例的生产者、文档与完成状态 | 发布容量为 1 时，真实子进程连写三行，中间行和无换行的尾行可丢失，还重复响铃。Markdown 最终准备已经失败，阅读示例却通知“已完成”。 | 命令写端等待下游容量，只继续尚未被接纳的行；从文档的最终状态和准备错误决定阅读结果。见 `TestCommandBurstHonorsPublicationCapacity`、`TestFinalPreparationFailureDoesNotAnnounceSuccess`。 |
| 工具解析的真实身份 | 显式选中指向 A 的 Mermaid 软链接，邻近安装 B 却可胜出；合法的引号或行尾注释会让不同发布脚本对同一 go.mod 得出不同模块名。 | Mermaid 从所选入口解析安装包；发布和独立模块验证调用 Go 读取模块身份，不再各用一套文本表达式。见 `extensions/mermaid/backend_internals_test.go`、`ui/internal/arch/release_internals_test.go`。 |

## 迁移

- 事件处理器读取 `event.Input()`、`event.Target()`；跨回调的焦点请求通过当前元素或 Root 发起。不要保存一个旧事件等到下一轮再认领按下。
- 手动把同一元素从一个根移到另一个根时，若遇到 `scene.ErrCleanupPending`，先让旧回调返回，在旧根所属的序列上结清通知，再重试。只有旧附件借出的 `Done` 关闭，还不能证明回调已经结束。
- 分页用 `q.Cursor = page.Next`；`page.Next == (scene.Cursor{})` 表示没有下一页。模型或范围改变后重新查询。几何和写入进度仍是独立水位，分页不冻结物理终端。
- `scene.Item` 使用具名字段；表单皮肤放在父子关系的 `Paint`。构造端自身可长期保留的皮肤仍可用元素原画法。兄弟组整体更新只适用于同一父组，不是任意树变更的全局事务。
- 外部实现的 `base.Field` 应能为一次已完成的校验提供与该答案绑定的回执，包括失败校验。已有领域请求尚未应用时，不要把“已启动”当成“回答已生效”。普通表单继续使用 `Submit`。

## 验证与边界

变更前的反例与变更后的预期均按上表追踪。主动 fuzz 选择输入分块、截断、链接边界三个目标，各运行 30 秒，共 905,002 次执行；其他目标的已提交语料随普通 Go 测试回放。Linux 使用真实 PTY 和子进程；macOS、Windows 原生 CI 的绿灯属于基线。使用本机 Node 子进程验证 Mermaid 入口和取消，并不等于在本机运行官方 Chromium 渲染。

七个模块在 Go 1.27.3 上分别通过 vet、Linux race、固定版本 lint 和 govulncheck。SSH 的扫描另报一项依赖模块级公告，未涉及导入的受影响包或可达调用。可达性、格式、workspace 清单和 API ledger 门禁通过；当前模块路径尚无已发布的比较基线，因此没有将 ledger 的跳过项记作 API 兼容验证。历史发布和本次破坏性迁移见 CHANGELOG。

关闭 workspace 后，七个模块各自的测试和 tidy 差异检查均通过；在隔离副本运行 workspace 同步未产生漂移。Go 1.27.0 下七个模块全部通过 Linux 原生测试，以及 Darwin/amd64 和 Windows/amd64 测试二进制的交叉编译。合计 77 项仓库与模块检查，失败 0 项。

对补丁内文档提交之前的 17 个代码提交，使用独立克隆依次编译生产和测试代码。改到 UI 的提交同时编译所有消费模块，共 95 次“提交 × 模块”编译，全部通过。这是中间提交的构建证据；全套行为门禁针对最终代码。

没有为测试函数数目、相似代码或不是同一事实的缓存引入新抽象；这些结果也不构成对所有应用回调、平台或调度交错均无缺陷的证明。
