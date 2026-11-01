---
title: Oolong 所有权收口与合并补丁
description: 从实际 main 出发，记录可观测的冲突、唯一写入边界、调用方迁移和验证边界。
contentType: Reference
---

# Oolong 所有权收口与合并补丁

<!-- cspell:words mktemp mailsplit -->

## 交付基线

审查的是 Tangerg/oolong。[GitHub main](https://github.com/Tangerg/oolong/commit/0f68f7348fac796edbb1fff3d4540046840cb08a) 为 `0f68f7348fac796edbb1fff3d4540046840cb08a`，树为 `0fcdb6925a9dba373c8a84843d5d72771a587a71`。[主 CI](https://github.com/Tangerg/oolong/actions/runs/41254604556) 的 20 个任务成功。本次开始和复核时，上一份 14 提交的补丁均未进入远端 main。新补丁以该远端提交为起点，包含已经交付的 14 个提交，接着在它们的精确文件树 `dd879ed70c415b53be14e3b910d26e5a3d803d83` 上收口。没有向远端推送。

完整交付覆盖七个工作区模块；模块名单仍由仓库脚本产生。前段的失败现场与迁移见[上一份所有权审计](./oolong-ownership-review-0f68f73.md)，包括输入计费、写入水位、代理与会话的绑定、文档终态、选区、搜索、命令背压及关闭结算。本报告补充此后发现的同类入口。两个阶段通过一份 Git 邮箱补丁交付，不需要手工拼接。

## 如何界定“修完”

以 [AGENTS.md](https://github.com/Tangerg/oolong/blob/0f68f7348fac796edbb1fff3d4540046840cb08a/AGENTS.md) 和 [PROJECT_RULES.md](https://github.com/Tangerg/oolong/blob/0f68f7348fac796edbb1fff3d4540046840cb08a/PROJECT_RULES.md) 为准，对一个事实依次检查签发者、所有写入入口、只读投影、交付及撤销时机。对于能复现的冲突，同时查直接命令与返回命令、当前与保留句柄、普通返回与异常退出，以及另一个同时有效的实例。区分值的所有权与命令权限：复制文本和画面证据是合法需求，复制出来的值不能因此修改原文、恢复过期的捕获或删除别的终端的资源。

保留有独立含义的状态。例如，写端接纳与物理写入并非同一事实，原始图片协议编号与一次实际传输也并非同一身份。没有为了消除外观上相似的数据而合并这些契约。以下对应的是已验证的失败场景，而不是把每一个重复字段都算作 bug。

## 一、绘制、布局与事件

| 原可观察故障 | 唯一所有者与修复 | 证据入口 |
| --- | --- | --- |
| 第一个组件保存普通 grid 视图和暂存句柄；第二个组件绘制时，前一个组件的像素及暂存值仍能被改写。嵌套只读回调、Prepare 返回后甚至提交以后，也都能通过保存的视图写入。 | 同一个 paint 回调授权像素、cursor、原生绘制区域和暂存几何。所有子视图继承父级写限制；回调结束、panic 或主动退出后都不能续用。合成器自己的底层画布仍归合成器。 | `ui/core/scene/paint_lease_test.go`、`producer_scope_test.go`、`ui/core/grid/view_lifetime_test.go` |
| 父布局与子布局合法复用矩形切片。子布局执行后，父布局稍后才访问的兄弟矩形变了，本应在第二列的字符落到了第一列。 | Group 在进入子回调前取得本次布局结果，使一轮已决的位置不再受下一个回调写入影响。 | `TestNestedLayoutCannotOverwriteItsParentsGeometry` |
| 父预览换掉被点击的子内容，然后通过直接命令或回调返回值抢到焦点和捕获。反过来，子内容未变但父内容已换，父预览仍收到了旧坐标。 | 一次路由保留原命中者、接收者、输入轮次及布局证据。预览、冒泡、捕获和返回的命令遵守同一判定，不能用父节点仍有效来替代原命中内容仍有效。 | `ui/core/scene/route_ownership_test.go` |
| 同一个按下可以先被父预览认领，再传给子项启动第二个捕获；重新绘制后的替换正文收到旧手势的松开；到达时间不在已知历史内的松开仍被激活。 | 认领按下即消费；后续手势不仅核对按钮，还核对认领时的内容身份以及到达帧。缺少证据时撤销手势。 | 同文件内的单次捕获、替换正文、历史丢失用例 |
| 无可聚焦子项的模态框收不到 Esc；焦点在一次通知内往返后，旧的 gain 又被投递；键盘事件可读取别的根的投影，明确暂存 nil 却被报告为缺失。 | 无焦点时的键盘接收者由真正的模态作用域决定。焦点 gain 绑定到改变焦点的那一轮。输入读取本场景的已提交投影，并区分“有值且为 nil”和“没有投影”。 | `TestKeyboardFallbackUsesTheActiveModalWithoutInventingFocus` 等 |
| 类型化动作接受可空指针，AX 传同一个 nil 却报参数错误。语义关系还可指向别的根或已经卸载的节点。 | 由动作签名解释真正传入的参数；发现动作时不构造虚假的参数去测试领域可用性。观察只发布同一根的活关系，合法的子树外关联仍可见。 | `ui/core/scene/action_argument_test.go` |
| 带修饰符的 locale 或恰好以 UTF8 开头的其他编码绕过了编码判断。 | 只解释 locale 中的字符编码名，修饰符不能决定 Unicode 能力。组件与 LaTeX 仍共用这一条策略。 | `ui/core/text/locale_test.go` |

## 二、表单、编辑器与组合控件

| 原可观察故障 | 唯一所有者与修复 | 证据入口 |
| --- | --- | --- |
| 两个表单的第一次提交使用同一数字，A 的回执能结算 B。业务还在决定是否接受时，回调就能先调用 Settle；异常退出能留下没人负责的 Pending。 | Form 自己签发不透明 ID，业务只返回接纳结果。仅已返回 Started 的当前尝试能异步结算；本次回调中断记录失败，后继提交不受污染。 | `ui/base/attempt_lifetime_test.go` 及实际表单示例 |
| 草稿改了仍显示旧错误；若把每个字段错误都绑到整表，又会因修改无关字段而隐藏尚未消除的错误。 | 单字段错误由该字段当前校验提供；跨字段错误由被检查的整份草稿认证。已结束的异步提交仍描述提交时的版本。 | `ui/base/form_receipt_test.go`、`attempt_lifetime_test.go` |
| 旧原子元素的数字范围从另一份文档读出替换文本；另一个编辑器会话的同号 ID 还能删掉自己的元素。 | 元素快照保存被捕获的正文，删除命令使用编辑会话签发的 ID。共享 EditSession 的视图可以协作，独立会话不能互删。 | `ui/base/element_snapshot_test.go`、composer 示例 |
| 比较选项、排序表格、给列表评分或测量虚拟内容时，外部回调能更改眼前正在投影的游标或数据。Button、Confirm 的直接可用性查询也存在相同问题。 | 已挂载和未挂载的控件都复用 scene 的只读边界。虚拟窗口把只读查询与需要创建元素的构造步骤分开。没有给每个 setter 再造独立的忙碌标记。 | `ui/base/preparation_owner_test.go`、`ui/base/virtual.go` |
| 皮肤改变分隔线，标签栏仍按旧的行数排版；取消旧键序列可重入打开另一个弹窗，旧关闭动作随后又把它关掉。 | 分隔线文本是测量与绘制的单一来源。弹窗先结算本次开闭关系，再执行旧解析器的取消。 | `ui/kit/tab_rule_test.go`、`ui/base/preparation_owner_test.go` |

## 三、运行时、终端与图片

| 原可观察故障 | 唯一所有者与修复 | 证据入口 |
| --- | --- | --- |
| 子程序已接管终端，父会话保留的写端仍能发帧、标题、响铃。Writer 的接口还可以被还原成能发控制命令、关闭会话的对象。 | Writer 在获取排空水位前暂停普通接纳。对外只交付普通帧端口；恢复、控制输出和关闭归终端所有者。恢复后普通输出可继续，关闭后不能复活。 | `ui/core/term/terminal_test.go` 的真实 PTY 及 writer 生命周期用例 |
| 环境查询发生在进入 raw mode 或启动读取资源以后；应用的 lookup 异常可留下未恢复的终端，多个探测器还会对同一键重新询问。 | 先取得本次启动所需的环境事实，再申请终端资源。各探测器使用同一快照；协商得到的新事实仍可覆盖环境推断。 | `ui/core/term` 的启动与交接回归 |
| 框架启动的 worker、目的 writer 或 SSH reader 抛错时，没有调用者能在原 goroutine 接住；同步输入/重绘回调中断则可能遗留等待者。 | 跨 goroutine 的失败结算给所属 task、writer 或 SSH 会话，保留可追踪的错误原因。同步 UI 回调继续按 Go 语义展开，但已认领的回执和最后一个入口批次仍有终结。 | `TestWorkerPanicSettlesOnItsOwnTask`、`TestClaimedRepaintAnswersWhenPaintDoesNotReturn` 及 SSH 源生命周期用例 |
| 底层已写正字节数又返回“完整拒绝”，原控制通道仍可能当成可安全续写。 | 已写出的前缀优先决定流是否仍可复用。只有零字节且明确拒绝，才符合完整拒绝的契约。 | `ui/core/term/writer_lifetime_internals_test.go` |
| 两台终端碰巧拥有相同的图片 ID，原 API 能删除或放置另一端的数据。同一终端复用 ID、位置和坐标时，差分也可能误认它仍是原传输。 | 实际资源绑定签发终端与传输代次；canvas 在略过相同区域以前核对原生绘制目标。重复释放与跨端操作被拒绝，释放后旧区域的清理不会擦除同号新资源。低层 wire 编码仍可显式使用。 | `graphics.ImageOwner`、`graphics.Resource`、终端与两种 canvas 的原生绘制用例 |

## 四、扩展和测试证据

- 一次截图先等“有任意新输出”，再分别读取宽高、历史和最后帧，能把不同时间点的结果拼在一起。现在由合成器在指定重绘结算时报告真实宽高，测试宿主一并取得帧与历史前缀。示例与视觉测试均消费这一张回执。
- PTY 的直接子进程退出不代表仍持有终端的后代已退出。取消由 PTY 会话负责整个进程组；读取结束是 Transcript 的事实，Drain 和等待文本都能观察它。没出现目标文本且已到 EOF 时，不再等到超时。
- 测试屏幕与本地、SSH、测试宿主使用同一个物理终端尺寸上限。仍可独立创建使用不同策略的离屏 Surface。无法建模的原生图片与窗口变更不能产生貌似正确的单元格断言；单个未完成字素受限，长的普通文本流不因总块长被误拒。
- Mermaid 渲染先执行字节预算检查，再做 UTF-8 检查。即便超大输入同时是非法文本，也不会先扫描预算之外的数据。示例清理原生图片通过真正的资源所有者完成，没有裸编号的第二条删除路径。

## 迁移索引

| 调用方 | 当前用法 |
| --- | --- |
| 表单业务 | `Submit(id) (scene.ActionState, error)`；只有接纳后台工作时才返回 `scene.Started`，以后在所有者轮次以原 ID 调 `form.Settle`。 |
| 原子编辑元素 | `element.Text()` 读捕获的文本；以 `base.ElementID` 关联业务载荷。无需把另一个 editor 作为读取参数。 |
| 控件绘制与标签栏 | 在产生 frame 的回调内绘制、暂存、细化；之后通过已提交投影读取。用 `tabs.SetRule(glyph)` 同时更新画法和行数。 |
| 本地或自定义宿主 | 用 `Writer.Frames()` 或 `Terminal.Writer()` 提交普通帧；对尺寸使用 `grid.ValidateTerminalSize`。自定义原生图片宿主使用同一 `ImageOwner` 签发资源并提供其 `NativeTarget`。 |
| 测试与自定义重绘源 | `host.Repaint(ctx)` 返回已结算的快照。自定义 `program.RepaintSource` 提供 `func(width, height int, err error)` 回执。见[测试指南](../zh/guides/testing.md)。 |
| 自动化、事件与搜索 | 从真实 runtime 构造 driver；读取不可改写的 Target；搜索结果与计算它的布局一起提交。完整例子和旧 API 名见[前段迁移记录](./oolong-ownership-review-0f68f73.md)。 |

## 验证及边界

测试与 fuzz 入口的增减不等于覆盖率。交付分别检查修复前的反例、最终代码的行为和竞态、中间提交的可构建性，以及合并补丁实际应用后的文件树。本机运行 Linux；交叉构建不计作在 macOS、Windows 或其他系统上执行了原生测试。原生终端回归使用真实 PTY，Mermaid 的本地子进程与诊断管道测试不等于安装并运行官方 Chromium。自定义宿主仍然负责如实描述它持有的物理终端；裸协议编码器不是安全沙箱。

- **回归保护**：按相同规则统计顶层 Go Test/Fuzz 声明，远端基线 2,011 个，上一份补丁 2,053 个，此次交付 2,095 个。仓库声明的 23 个 fuzz 目标各运行 30 秒，共 4,186,608 次执行，未见失败。针对旧版本的逐项核对保留在 [0.25.0 的 21 条 Fixed 对照](./oolong-v2-regression-audit.md)。
- **可二分**：相对上一份补丁的 16 个代码、测试及架构契约提交逐个独立检出。在各提交受影响的模块中编译生产代码与测试，共 107 个组合通过。这里检查的是每一步可构建；行为测试、竞态和其他门禁在交付代码上单独检查。
- **Go 门禁**：合并首跑与修正后的受影响项复验，共 77 项通过。覆盖七个模块的 vet、Linux 竞态测试、lint、可达漏洞扫描、模块独立测试与 tidy、最低 Go 1.27.0 的 Linux 测试及 Darwin/Windows amd64 测试代码编译，以及格式、可达性和工作区检查。首跑抓到一个测试夹具的非法字面量；修正后重跑 UI 及相关检查，没有放宽断言。
- **平台矩阵**：按 CI 清单对 linux/arm64、darwin/amd64、windows/arm64、freebsd/amd64、js/wasm、wasip1/wasm 各做七个模块的 vet 和 build，84 项通过。远端 20 个成功任务只属于起点提交；没有把它们用作本补丁的 CI 成绩。
- **依赖与 API**：`govulncheck` 未在七个模块找到受影响的可达调用。SSH 的依赖图有一项模块级公告，未涉及导入的受影响包。API 台账对当前路径找不到已发布版本，实际跳过了历史 API 比较；本补丁也有意变更了公开契约。

## 如何从自己当前的版本应用

同一个邮箱文件包含远端基线之后的完整提交序列。先用 `git status --short` 确认工作区干净，再用 `git rev-parse 'HEAD^{tree}'` 查看文件树。`git am` 会重新创建提交，判断是否已经应用前段时应比较文件树，而不依赖提交号。

**从远端基线开始**：在仓库内，从以下精确提交新建分支，应用整份补丁。请把示例路径改为下载文件的绝对路径。

```sh
git switch -c oolong-ownership-complete 0f68f7348fac796edbb1fff3d4540046840cb08a
git am /absolute/path/to/oolong-complete-0f68f73.patch
```

**本地已应用上一份补丁**：仅当当前文件树正好为 `dd879ed70c415b53be14e3b910d26e5a3d803d83` 时使用。它保留前 14 个提交，直接应用同一文件的剩余部分。

```sh
(
  set -eu
  test -z "$(git status --short)"
  test "$(git rev-parse 'HEAD^{tree}')" = dd879ed70c415b53be14e3b910d26e5a3d803d83
  parts=$(mktemp -d)
  git mailsplit -o"$parts" -- /absolute/path/to/oolong-complete-0f68f73.patch
  set -- "$parts"/[0-9][0-9][0-9][0-9]
  shift 14
  git switch -c oolong-ownership-continued
  git am -- "$@"
)
```

如果当前分支还包含自己的改动，使用第一种方法新建验证分支，再按项目原有流程整合。两种方式均不需要重置现有分支。
