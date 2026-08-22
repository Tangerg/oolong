---
title: 构建你的第一个 Oolong 界面
description: 直接使用 Oolong 最底层的公开界面构建并运行全屏计数器。
contentType: Tutorial
---

# 构建你的第一个 Oolong 界面

语言：[English](../getting-started.md) | 简体中文

本教程使用最底层的公开能力构建一个全屏计数器。你将运行一个组件、绘制两行文字、处理按键并退出，全程不需要导入组件库。

## 前置条件

使用 Go 1.27 或更新版本，并在能够进入原始输入模式的终端中运行。程序也支持 Windows Terminal，以及提供终端的 SSH 会话。

## 创建模块

创建一个空模块并添加 `core`：

```sh
mkdir oolong-hello
cd oolong-hello
go mod init example.com/oolong-hello
go get github.com/Tangerg/oolong/core@latest
```

创建 `main.go`。先写导入和运行时入口：

```go
package main

import (
    "context"
    "fmt"
    "os"
    "strconv"

    "github.com/Tangerg/oolong/core/grid"
    "github.com/Tangerg/oolong/core/input"
    "github.com/Tangerg/oolong/core/program"
)

func main() {
    err := program.Run(context.Background(), program.Config{
        Root: func(runtime *program.Runtime) program.Component {
            return &counter{runtime: runtime}
        },
    })
    if err != nil {
        fmt.Fprintln(os.Stderr, "hello:", err)
        os.Exit(1)
    }
}
```

`Config.Root` 让应用独占整个屏幕，并在程序退出时恢复原来的屏幕。在 `main` 下面添加组件：

```go
type counter struct {
    runtime *program.Runtime
    keys    int
}

func (c *counter) Draw(view grid.View) {
    view.Text(0, 0, "Oolong", grid.Style{Attr: grid.Bold})
    view.Text(0, 1, strconv.Itoa(c.keys)+" keys | q quits", grid.Style{})
}

func (c *counter) Handle(event input.Event) bool {
    key, ok := event.(input.Key)
    if !ok || !key.Down() {
        return false
    }
    if key.Rune == 'q' || key.Rune == 'c' && key.Mods.Has(input.Ctrl) {
        c.runtime.Quit()
        return true
    }
    c.keys++
    return true
}
```

`Draw` 使用相对于视图的局部坐标，视图会在自己的边界处裁剪写入。`Handle` 只为组件真正接管的事件返回 `true`。

## 运行界面

在终端中运行模块：

```sh
go run .
```

按任意键改变计数，按 `q` 或 `Ctrl+C` 退出。状态变化后运行时才会重绘；没有输入或定时工作时，运行时会休眠。

## 添加下一层能力

界面继续生长时，底层组件契约不需要变化：

- 需要切分视图区域时，添加 `core/layout` 的布局原语
- 需要无外观的可复用行为时，添加 `components/headless`
- 需要默认主题和复合组件时，添加 `components/kit`
- 已完成输出需要保留在终端回滚区时，把 `Config.Root` 换成 `Config.Inline`

[`examples/hello`](https://github.com/Tangerg/oolong/tree/main/examples/hello) 就是这个最小切片。接下来可以浏览
[示例目录](https://github.com/Tangerg/oolong/tree/main/examples)，或者阅读[组合一个可换主题的选择器](components.md)
以添加组件层。
