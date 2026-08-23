---
title: 测试 Oolong 界面
description: 在进程内测试组件，只在传输行为需要时使用伪终端。
contentType: How-to
---

# 测试 Oolong 界面

语言：[English](../testing.md) | 简体中文

本指南帮助你选择能够观察目标行为的最小测试工具。组件和应用状态使用 `programtest`；只有当结论依赖真实终端传输时，才使用 `ptytest`。

## 在进程内测试应用行为

`programtest.Host` 实现了 `program.Host` 的三个必需方法。它不打开终端，只发送事件并记录帧：

```go
func TestCounterQuits(t *testing.T) {
    host := programtest.New(t, 60, 12)
    done := make(chan error, 1)
    go func() {
        done <- program.Run(t.Context(), program.Config{
            Host: host,
            Root: buildCounter,
        })
    }()

    host.Shows(t, "0 keys")
    host.Type("ab")
    host.Shows(t, "2 keys")
    host.Type("q")
    if err := <-done; err != nil {
        t.Fatal(err)
    }
}
```

`Shows` 和 `Hides` 会先请求一次完整重绘，再检查文字。测试需要观察最近一次差量帧时使用 `Frame`，需要观察多次写入顺序时使用 `Frames`。

## 只添加一个可选 host 能力

基础测试 host 刻意不实现任何可选终端能力。在本地测试类型中嵌入它，并且只添加需要验证的能力：

```go
type darkHost struct {
    *programtest.Host
}

func (h darkHost) Ground() grid.Ground {
    return grid.Ground{
        FG: grid.RGBColor(220, 220, 220),
        BG: grid.RGBColor(20, 20, 20),
    }
}
```

这样才能观察能力缺失的情况。一个全能 fake 会让所有应用误以为剪贴板、locale、图片传输、进度报告和窗口标题永远存在。

## 在 PTY 上测试终端所有权

断言直接涉及字节或终端状态时使用 `ptytest`。先把待测命令构建到 `t.TempDir`，再在 PTY 上启动：

```go
if !ptytest.Supported() {
    t.Skip("this platform has no PTY harness")
}
session, err := ptytest.Start(t.Context(), ptytest.Config{}, binary)
if err != nil {
    t.Fatal(err)
}
defer func() { _ = session.Close() }()

ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
defer cancel()
if err := session.Transcript().WaitFor(ctx, "ready"); err != nil {
    t.Fatal(err)
}
if _, err := io.WriteString(session, "q"); err != nil {
    t.Fatal(err)
}
```

PTY 测试能够证明进程内 host 无法观察的事实：

- 终端模式只启用一次，并按相反顺序关闭一次
- 内联块缩小时不会留下旧单元格
- resize 通过平台适配器到达进程
- 空闲界面停止写出字节
- 活动块退出后，已发布输出仍然保留

`ptytest` 捕获字节流，但不是通用终端模拟器。当结论是 `grid.Screen` 或
`grid.Inline` 输出的可见单元格文字时，把该字节流交给 `ptytest.Screen`。这份与 renderer
等宽的模型覆盖光标移动、擦除、有界滚动、宽字符，以及终端在右边界上的延迟换行；遇到不支持的设备流量会返回错误，而不会猜测其语义。如果协议序列本身就是契约，应当直接断言这个序列。

## 让时间和后台工作保持确定性

优先使用显式 channel、`testing/synctest` 和运行时回调，不要用 sleep 猜测 goroutine 需要多久。测试应等待可观察的帧或状态转换。

数据源通过 `program.ByteIngress` 写入时，关闭摄入对象并等待最终的所有者侧批次。取消测试还应证明被阻塞的 writer 会返回，而且程序停止后没有生产者存活。

## 保留 fuzz 回归

开始生成新输入之前，`go test` 会回放 `testdata/fuzz/<Target>` 下的文件。目录名必须与
`func <Target>(f *testing.F)` 完全一致；重命名目标时，必须在同一变更中迁移其语料。
架构关卡会拒绝没有存活 fuzz 目标的语料，因为 Go 原本会静默忽略这种目录。

语料文件是字节 fixture。仓库强制所有 `testdata/fuzz/**` 路径使用 LF，避免检出设置在
Go 读取之前改写 seed。

## 为每一条可调用路径提供可执行证据

`scripts/check-reachability.sh` 会带测试在 Linux、macOS 与 Windows 上运行钉住版本的
`deadcode` 分析器。私有不可达函数是死亡实现；公开不可达操作则缺少可执行契约覆盖，
但不一定是死亡 API：框架本来就服务于下游调用方，扩展点也可能刻意没有仓库内生产调用。
应当为它提供外部包行为测试；是否保留或删除，还必须另外评审其职责、抽象层级、重叠与契约。

唯一纯静态的例外是 `noCopy.Lock` 和 `noCopy.Unlock`。它们的方法集由 `go vet` 消费；仅仅
为了满足运行期可达性工具而调用它们，反而会制造虚假证据。脚本只过滤这两个精确名称，
并用合成的标记方法与普通方法发现项自测过滤器；架构关卡则另行证明标记方法仍然存在。

可达性绝不授权删除。`scripts/check-api-changelog.sh` 会另外使用钉住版本的 `apidiff`，
把各公开模块与前一个 tag 比对，并要求每个不兼容的导出 API 变更以精确名称出现在 Unreleased
迁移清单中。这份证据无法替代设计判断，但能防止删除或重塑成为静默满足可达性关卡的捷径。

## 让单拥有者类型在机制上保持单拥有者

若一个导出的可变类型在文档中声明首次使用后不得复制，它就直接携带一个 `noCopy`、`sync`
或 `sync/atomic` 字段。这样，`go vet` 的标准 `copylocks` 分析器会拒绝值复制。架构关卡从
生产文档中推导这些契约，并验证私有 `noCopy` 标记仍然同时实现 `Lock` 与 `Unlock`；类型
清单和标记含义都不能静默过期。

只有复制会共享可变存储或生命周期状态时才使用这项契约。刻意允许复制的值应当拥有不可变
数据，或像 `markdown.Doc` 一样在修改之前分离存储。不要仅仅因为方法恰好使用指针接收者
就增加标记。

调用方可见的行为应放在外部包测试（`foo_test`）中。只有当某项性质无法通过公开面表达时，
白盒测试才可以使用实现包，而且文件名必须以 `_internals_test.go` 结尾。架构关卡会从源码
推导包边界，并拒绝越过边界的普通测试，使私有耦合始终是例外，也始终能在评审中看见。

## 有意更新视觉 golden

复杂示例把整帧几何保存成文本 golden，便于整体评审。只有新输出确实符合预期时才重新生成：

```sh
cd examples
go test -update ./agent
```

把变化后的 golden 与代码一起评审。普通的 `go test ./...` 永远不会改写它。

请在[构建有界 Agent 界面](agent.md)中把两种测试工具应用到完整流式应用，或阅读
[架构](architecture.md)以了解这些测试所强制执行的不变式。
