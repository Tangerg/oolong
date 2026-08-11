---
title: 排查 Oolong 应用问题
description: 按所有权边界定位模块、终端、输入、流式处理、内存和测试故障。
contentType: Troubleshooting
---

# 排查 Oolong 应用问题

语言：[English](../troubleshooting.md) | 简体中文

请从可见症状开始，再检查该状态的所有者。Oolong 会分离终端事实、所有者侧组件、后台字节和已交付输出，因此每种故障都有一个主要边界。

## 应用在仓库中可以构建，但独立构建失败

`go.work` 可能满足 `go.mod` 没有声明的本地 import。请关闭工作区以复现消费方依赖图：

```sh
GOWORK=off go mod tidy -diff
GOWORK=off go test ./...
```

把缺失的 Oolong 模块添加到与已有模块相同的版本。不要提交 `replace` 指令，消费方不会使用工作区替换。

## 导入的 Oolong 包无法一起编译

Oolong 仍处于 1.0 之前，并采用协调模块版本。请检查最小版本选择最终采用的版本：

```sh
go list -m all | grep github.com/Tangerg/oolong
```

同步升级所有已导入的 Oolong 模块，再阅读[变更日志](https://github.com/Tangerg/oolong/blob/main/CHANGELOG.md)中的 breaking change。

## Unicode 边框显示成损坏字节

请根据运行时真正驱动的终端解析字符集：

```go
glyphs := kit.GlyphsFor(runtime.Environment().Locale())
formulaGlyphs := latex.GlyphsFor(runtime.Environment().Locale())
```

不要在组件中读取服务器进程的 `LANG`、`LC_ALL` 或 `TERM`。SSH 客户端与服务器进程可能描述不同的终端。

## 指针输入命中了旧区域

请用 `headless.NewRoot` 包装实时 headless 树。在 `Draw` 中暂存几何，并在 `Handle` 中读取：

```go
w.areas.Stage(frame, nextAreas)

func (w *widget) Handle(event input.Event) bool {
    areas := w.areas.Value()
    return w.route(event, areas)
}
```

不要在 `Draw` 中推进语义状态。`headless.Snapshot` 只会在完整根帧成功后发布呈现几何。

## 后台 writer 阻塞

已接受字节达到配置上限后，`ByteIngress.Write` 会施加背压。请检查：

- 所有者回调会返回，而不是等待生产者
- 运行时所有者没有等待网络、进程或模型 I/O
- 取消操作能够到达生产者 context
- 生产者只调用一次 `CloseWithError`
- 只有其他 goroutine 会通过 `ByteIngress.Done` 等待资源清理

提高字节上限可以吸收更大的突发，但无法修复永不返回的所有者回调。

## 实时内存随会话年龄增长

完成的 transcript 块必须离开实时图。请把稳定块标记为完成，只保留有界的交互前缀，并通过带显式上限的 `kit.Transcript.Commit` 交付超出的部分。

只让开放的 Markdown 尾部保持可变。不要保留已经交付输出的子串或行。[流式指南](streaming.md)定义了所有权切点，[Agent 指南](agent.md)展示了有界保留策略。

## 退出后终端没有恢复

请先让 `program.Run` 返回，再调用 `os.Exit`。组件或后台 goroutine 应当通过运行时边界返回错误，而不是直接退出进程。

如果断言涉及 raw mode、光标可见性、备用屏幕对称性或内联清理，请使用真实伪终端 (PTY) 测试。`programtest` 无法观察操作系统终端状态。

## 界面测试一直等待

请等待可观察输出或状态转换，不要用 sleep 猜测时间。流式测试必须关闭 ingress 并等待最终 `ByteBatch`；运行时测试必须发送让程序终止的事件。

```go
host.Shows(t, "ready")
host.Type("q")
if err := <-done; err != nil {
    t.Fatal(err)
}
```

如果取消测试仍然挂起，请断言被阻塞的 writer 会返回，而且 `Dispatcher.Done` 之后没有生产者继续存活。

## 收集可复现报告

请在 [bug 报告](https://github.com/Tangerg/oolong/issues/new?template=bug.yml)中包含：

- `go list -m all` 输出的 Oolong 模块版本
- Go 版本、操作系统、终端，以及本地或 SSH 传输方式
- 能够复现症状的最小输入、程序或协议字节
- 预期与实际的所有权转换
- `programtest` 或 `ptytest` 是否能够复现

安全边界问题请按照[安全策略](https://github.com/Tangerg/oolong/blob/main/SECURITY.md)私下报告。
