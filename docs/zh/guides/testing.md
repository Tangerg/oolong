---
title: 测试所有权与输出
description: 在各自所有权边界测试模型迁移、语义动作和终端行为。
contentType: How-to
---

# 测试所有权与输出

语言：[English](../../guides/testing.md) | 简体中文

## 选择测试边界

直接控制器测试验证值与交互策略。`scene.Root` 候选验证挂载、焦点、语义动作、几何和中止行为。`ui/automation` 验证远程代理使用的相同查询与动作。将 `uitest.New` 作为 `program.ScreenConfig` 或 `program.InlineConfig` 的 Host 在进程内运行会话，使用 `uitest/ptytest` 验证终端屏幕或真实 PTY 一致性。

需要断言某次重绘时，调用 `host.Repaint(ctx)`。快照包含实际绘制的画布尺寸、完整帧和截至该帧的全部输出；请求之后出现普通写入，并不代表请求的帧已经画好。给上下文设置时限，这样即使程序在接收请求前退出，测试也不会挂起。取消真实 PTY 会话时，即使直接子进程已经退出，同一进程组中仍占用终端的后代也会被终止。若输出结束时仍未出现所等待的内容，`WaitFor` 返回读取错误或 `io.EOF`。

需要点击已确认输出时，在 `Repaint` 返回后调用 `host.Pointer(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: position})`，保持 `At` 为零。`Pointer` 在排队前捕获已完成输出的不可变几何，后续帧不能重新定位这个事件。程序尚未绑定输入会话，或写入、几何仍未稳定时，它返回 false。键盘事件、指针意图和重绘请求共享一条 FIFO。

使用 `host.Send` 验证带真实到达时间戳的物理报告。相等、过期或不确定的时间戳不能证明点击属于哪一帧，因此有意拒绝这类坐标关联。合成指针意图与已经观察到的输出有明确因果关系，不需要 sleep 或伪造时间戳来跨越时钟刻度。自定义测试传输可实现 `program.PointerSource`，在一次程序生命周期内绑定捕获能力。

截图验证外观，但不能证明键盘和代理能够调用同一动作。相关事实重要时，应同时断言动作结果、target 生命周期、源修订或写入回执。

`ptytest.Session.Wait` 观察直接子进程退出，同时保留其内核身份供进程组清理使用。子进程失败时返回 `ui/core/process` 的 `*process.ExitError`。即使 `Wait` 成功，也必须调用 `Close`；它先完成最后的进程组信号，再回收子进程并释放保留的进程槽位。会话独占最终回收操作，因此不要自行回收它的子进程，也不要启用 SIGCHLD 自动回收。上下文取消走同一条关闭路径。

## 覆盖事务与失败

准备一帧，检查已提交几何尚未推进，然后中止。验证模型变更保留，几何仍停在之前完整帧。接受另一个候选，再单独报告物理写入。排队帧不等于已写入帧。

注入容量拒绝、卸载后的工作完成、无效动作参数、部分写入和原生 painter 拒绝。流式测试跨任意 chunk 边界比较最终单元格与整源渲染，包括前向引用和拆分 UTF-8。

## 限定仓库夹具的生命周期

`ui` 模块内的测试通过 `ui/internal/scenetest` 共享组件适配和完整场景帧的实现。在每个测试或 fuzz 回调内调用 `scenetest.New()`，并用 `t.Cleanup` 注册所返回作用域的 `Close`。辅助函数显式接收该作用域，让连续绘制和焦点变化使用同一组件身份与帧序列。没有测试句柄的示例使用 `defer scope.Close()`。

若基准测试跨迭代复用组件，在 `b.Run` 内创建作用域；若每次迭代创建新组件，就在下一次迭代前关闭当前迭代的作用域。关闭作用域会释放它拥有的全部场景根和缓存适配器；进程级夹具注册表会持续持有已经结束的测试和 fuzz 样本。

## 运行仓库门禁

在检出目录使用 `go -C ui test ./...` 覆盖公开模块及内部架构工具。`scripts/modules.sh` 推导工作区模块；跨模块变更应逐模块运行测试和 `go test -race`。`scripts/sync-workspace.sh --check` 只检查派生工作区，不修改文件。

文档测试编译完整双语入门程序、验证配对页面与仓库链接。`npm run docs:lint` 检查 Markdown，`npm run docs:build` 检查发布文档图。完整固定版本门禁见 [CONTRIBUTING.md](https://github.com/Tangerg/oolong/blob/main/CONTRIBUTING.md)。
