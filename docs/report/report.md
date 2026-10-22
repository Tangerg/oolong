# 项目调整补丁说明

<!-- cspell:words Dougong -->

已完成修复，并整理成可直接应用的 Git patch：

补丁文件位于本说明同目录：`oolong-upstream-fixes-73c72889.patch`。

补丁包含 **40 个文件**的调整，覆盖实现、回归测试、仓库内示例迁移、英文和中文文档，以及 breaking API 迁移说明。基线是本次通过 GitHub 连接器核验的 Oolong `main`：[73c72889dfb34c2bc4a17cb6d03014b88195d83c](https://github.com/Tangerg/oolong/commit/73c72889dfb34c2bc4a17cb6d03014b88195d83c)。

**我已在该基线的干净 checkout 上实际应用最终补丁，应用后的 Git 文件树与修复工作区完全一致。**

## 如何应用

在你的本地 `oolong` 仓库根目录执行，将路径替换成下载后的实际位置：

```bash
git apply --check /path/to/oolong-upstream-fixes-73c72889.patch
git apply /path/to/oolong-upstream-fixes-73c72889.patch
```

## 这次具体修了什么

按照 `AGENTS.md` 和 `PROJECT_RULES.md`，我将修复放在拥有对应状态和契约的位置，并删除被替代的实现路径。

### 1. 鼠标按钮身份：修复 Pointer，并覆盖关联的路由和拖选路径

报告中的问题已经通过真实 SGR 输入解码复现：**左键按下、右键按下、左键释放，会错误地完成右键点击。**

修复后的契约是：

- 明确属于其他按钮的 release 不完成点击，也不结束当前捕获。
- 匹配按钮的 release 正常完成，点击只能消费一次。
- `ButtonNone` release 表示未指明释放按钮，结束当前捕获。
- 原有“拖出区域、拖回区域、最后释放仍可点击”的行为保留。

继续检查后，我发现仅修改 Pointer 还不完整：**Root、Container、Stack、PointerRegion 也会被任意 release 提前清掉路由捕获**。这些关联路径已同步修复，避免正确按钮后续的拖动和释放丢失，或误交给替代控件。

Editor 和 Transcript 的左键拖选也存在同类问题，已一并修复。所有这些路径共用新增的 **`input.Mouse.Releases(button)`**，按钮关联判断只有一份；控件点击、路由目标和文本选择仍分别由各自的对象负责。

### 2. 泛型 History：保存完整结构化值，删除文本旁路映射

History 已调整为：

```go
History[T]
HistoryConfig[T]
NewHistory(...)
Recalled[T]
```

容量、连续去重、历史游标、草稿恢复和搜索仍由一套实现负责。业务值的复制和相等规则通过 `Config` 明确传入：

- `Clone` 负责复制可变引用数据。
- `Equal` 决定哪些相邻条目属于同一个值。
- 消息是否为空、是否记入历史、何时持久化，由应用决定。
- `Recall` 使用文本投影搜索，结果保留完整结构化条目。

这次也修复了仓库内一个实际问题：**Composer 示例用显示文本作为附件映射的 key，两次提交同名标签、不同内容的粘贴块后，回溯旧条目会取到新内容。**

该问题已经先失败、后通过。示例现在使用 `History[draft]` 保存文本和附件信息，原有的文本 key 映射以及独立草稿恢复状态已删除。

### 3. List / Filter 命中查询：统一使用最后提交帧

新增：

```go
List.Hit(point) (index, ok)
Filter.Hit(point) (index, ok)
```

查询使用组件的局部坐标，并依据最后完整提交帧的可见区域、滚动投影和集合身份回答：

- 拒绝四边越界、被裁切区域、空白行和空列表。
- 滚动或选择已经变化，但尚未重绘时，仍回答屏幕上那一帧的条目。
- 替换集合后，在新帧提交前拒绝旧命中。
- 查询不会修改选择或滚动状态。
- Filter 返回匹配结果列表中的索引，并直接复用内部 List。

这里还确认并修复了 Oolong 自身的缺陷：原来的 List 鼠标选择路径也缺少完整二维边界判断。现在 MouseDown / MouseDrag 与公开 Hit 查询共用实现，旧的 `reach(y)` 路径已删除。

### 4. Dispatcher 完成确认：实现 Invoke，并迁移仓库内真实消费者

新增：

```go
func (d Dispatcher) Invoke(
    ctx context.Context,
    fn func() error,
) error
```

它复用现有 Dispatcher 队列，保证：

- 尚未认领的回调可以被取消，取消返回后不会再开始执行。
- 已经认领的回调必须完成，等待方才返回。
- 回调正常返回时，传递其实际 error。
- 零值或停止的 Dispatcher 对未执行工作返回 `ErrStopped`。
- 回调 panic 时，等待方收到 `ErrInvocationAborted`，原始 panic 继续传播。
- 完成确认表示回调结束，不表示新帧已经写入终端。

**Invoke 必须从非界面 owner goroutine 调用。**

仓库内 `examples/agent` 的手写完成等待也已经迁移。新增回归实际证明，原实现可能在回调仍能继续提交结果时提前返回取消；修复后，该等待机制由 Invoke 统一负责，示例只保留自己的 run 身份判断。

## 需要注意的 breaking changes

主要破坏性变化集中在 History：

| 原调用方式或行为 | 新契约 |
| --- | --- |
| `History` | `History[T]` |
| `Recalled` | `Recalled[T]` |
| `Recall(query)` | `Recall(query, textProjection)` |
| 默认自动去重 | 显式配置 `Equal`；nil 保留连续重复 |
| 自动过滤空白文本 | 应用在提交边界自行判断 |
| 字符串固定存储 | 通过 `Clone` 明确值的复制规则 |

字符串使用方可以这样迁移：

```go
history := headless.NewHistory(headless.HistoryConfig[string]{
    Clone: strings.Clone,
    Equal: headless.Equal[string],
})

matches := history.Recall(query, func(value string) string {
    return value
})
```

结构化值应提供覆盖嵌套可变数据的复制函数。`Clone: nil` 使用普通赋值；复制函数还必须不修改入参、不产生外部副作用。

这些契约、迁移方法和示例均已写入补丁中的 `CHANGELOG.md`、Go 注释及中英文文档，没有保留旧字符串 History 的兼容包装。

## 实际验证结果

| 检查 | 结果 |
| --- | --- |
| 全部 10 个模块：workspace 模式 `go test -race -count=1 ./...` | 通过，48 个测试包 |
| 全部 10 个模块：`GOWORK=off`，使用仓库规定的本地替换方式运行竞态测试 | 通过，48 个测试包 |
| 独立模块 `go mod tidy -diff` | 全部通过；按 CI 规则处理本地替换对应的校验和 |
| 全模块 build、`go vet`、golangci-lint | 全部通过，最终 lint 为 0 issues |
| 全模块 govulncheck | 全部通过，未发现漏洞 |
| 架构、绘制副作用、可达性、API breaking 迁移检查 | 通过 |
| gofumpt、shfmt、`go work sync` | 通过 |
| `npm run docs:check` | 通过，包含依赖审计、拼写、Markdown 和文档构建 |
| Darwin / Windows 的 core、components、examples 生产代码与测试编译 | 六组全部通过 |
| 最终补丁应用检查、实际应用、反向检查、Git 文件树比对 | 全部通过 |

Go 验证使用 **Go 1.27.0**。Darwin / Windows 的结果是交叉编译验证；没有执行这两个平台的原生运行测试，也没有运行官方 Mermaid CLI / 浏览器集成测试。测试套件自身的条件跳过项保留，其中一项 PTY 后代进程测试在独立模块模式下因后代进程未继续存活而跳过，在 workspace 模式下通过。

## 本次交付范围

这份补丁应用于 **Oolong**。报告中的结构化 History、Hit 和 Invoke 提案已落地，并完成 Oolong 仓库内调用方的迁移；**Flame 仓库自身的重复实现仍需要在升级 Oolong 时删除**，本次没有将跨仓迁移宣称为已完成。

Popup Placement 沿用已有能力。Dougong 报告中的独立 TypeScript 文档示例修复属于 Dougong 仓库，本补丁按其责任边界处理，没有将这些职责引入 Oolong。
