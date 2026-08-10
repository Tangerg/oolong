# 测试 Oolong 界面

语言：[English](testing.md) | 简体中文

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
session, err := ptytest.Start(t.Context(), binary)
if err != nil {
    t.Fatal(err)
}
defer func() { _ = session.Close() }()

if err := session.Transcript().WaitWithin(5*time.Second, "ready"); err != nil {
    t.Fatal(err)
}
if err := session.Type("q"); err != nil {
    t.Fatal(err)
}
```

PTY 测试能够证明进程内 host 无法观察的事实：

- 终端模式只启用一次，并按相反顺序关闭一次
- 内联块缩小时不会留下旧单元格
- resize 通过平台适配器到达进程
- 空闲界面停止写出字节
- 活动块退出后，已发布输出仍然保留

`ptytest` 捕获字节流，但不是通用终端模拟器。如果协议序列本身就是契约，应当直接断言这个序列。

## 让时间和后台工作保持确定性

优先使用显式 channel、`testing/synctest` 和运行时回调，不要用 sleep 猜测 goroutine 需要多久。测试应等待可观察的帧或状态转换。

数据源通过 `program.ByteIngress` 写入时，关闭摄入对象并等待最终的所有者侧批次。取消测试还应证明被阻塞的 writer 会返回，而且程序停止后没有生产者存活。

## 有意更新视觉 golden

复杂示例把整帧几何保存成文本 golden，便于整体评审。只有新输出确实符合预期时才重新生成：

```sh
cd examples
go test -update ./agent
```

把变化后的 golden 与代码一起评审。普通的 `go test ./...` 永远不会改写它。

请在[构建有界 Agent 界面](agent.zh-CN.md)中把两种测试工具应用到完整流式应用，或阅读
[架构](architecture.zh-CN.md)以了解这些测试所强制执行的不变式。
