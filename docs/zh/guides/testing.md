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

截图验证外观，但不能证明键盘和代理能够调用同一动作。相关事实重要时，应同时断言动作结果、target 生命周期、源修订或写入回执。

`ptytest.Session.Wait` 观察直接子进程退出，同时保留其内核身份供进程组清理使用。子进程失败时返回 `ui/core/process` 的 `*process.ExitError`。即使 `Wait` 成功，也必须调用 `Close`；它先完成最后的进程组信号，再回收子进程并释放保留的进程槽位。会话独占最终回收操作，因此不要自行回收它的子进程，也不要启用 SIGCHLD 自动回收。上下文取消走同一条关闭路径。

## 覆盖事务与失败

准备一帧，检查已提交几何尚未推进，然后中止。验证模型变更保留，几何仍停在之前完整帧。接受另一个候选，再单独报告物理写入。排队帧不等于已写入帧。

注入容量拒绝、卸载后的工作完成、无效动作参数、部分写入和原生 painter 拒绝。流式测试跨任意 chunk 边界比较最终单元格与整源渲染，包括前向引用和拆分 UTF-8。

## 运行仓库门禁

在检出目录使用 `go -C ui test ./...` 覆盖公开模块及内部架构工具。`scripts/modules.sh` 推导工作区模块；跨模块变更应逐模块运行测试和 `go test -race`。`scripts/sync-workspace.sh --check` 只检查派生工作区，不修改文件。

文档测试编译完整双语入门程序、验证配对页面与仓库链接。`npm run docs:lint` 检查 Markdown，`npm run docs:build` 检查发布文档图。完整固定版本门禁见 [CONTRIBUTING.md](https://github.com/Tangerg/oolong/blob/main/CONTRIBUTING.md)。
