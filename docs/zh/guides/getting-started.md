---
title: 构建第一个界面
description: 运行具有唯一终端所有者和纯绘制回调的场景。
contentType: Tutorial
---

# 构建第一个界面

语言：[English](../../guides/getting-started.md) | 简体中文

## 运行完整程序

Oolong 要求 Go 1.27。公开模块是 `github.com/Tangerg/oolong/ui`；`core`、`base`、`kit`、`automation` 和 `uitest` 是同一模块内的层级。在仓库检出目录运行 `go run ./examples/hello`，即可启动带测试的 [hello 示例](https://github.com/Tangerg/oolong/tree/main/examples/hello)。

应用依赖已发布的 UI 模块后，可将下列程序放入 `main.go`，在 `go.mod` 添加模块依赖，再执行 `go run .`。文档测试会编译这个完整程序，英文版本使用完全相同的代码。

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/Tangerg/oolong/ui/core/grid"
    "github.com/Tangerg/oolong/ui/core/input"
    "github.com/Tangerg/oolong/ui/core/program"
    "github.com/Tangerg/oolong/ui/core/scene"
)

func main() {
    app, err := program.NewScreen(program.ScreenConfig{
        Build: func(runtime *program.Runtime) (scene.Element, error) {
            return scene.NewElement(scene.ElementConfig{
                Key: "hello",
                Semantics: func() scene.Semantic {
                    return scene.Semantic{Role: scene.RoleStatus, Name: "Hello, Oolong"}
                },
                Paint: func(frame scene.Frame) {
                    frame.Text(0, 0, "Hello, Oolong — press q to quit", grid.Style{})
                },
                Handle: func(event scene.Event) scene.EventResult {
                    key, ok := event.Input.(input.Key)
                    if ok && key.Down() && key.Rune == 'q' {
                        runtime.Quit()
                        return scene.EventResult{Consumed: true}
                    }
                    return scene.EventResult{}
                },
            }), nil
        },
    })
    if err == nil {
        err = app.Run(context.Background())
    }
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

## 遵循所有权边界

`NewScreen` 只验证配置，不打开终端。`Run` 开始唯一会话，在宿主就绪后调用 `Build`。`Build` 返回稳定的 `scene.Element`。`Paint` 只写入帧借出的单元格；`Handle` 在 UI 所有者上改变应用状态；`Semantics` 读取同一份状态，不产生状态迁移。

合成器接受编码后的输出前，帧只是候选。场景几何、单元格差分基线和发布责任一起提交。回调返回后，不得保留可写的 `scene.Frame` 或 `grid.View`。一个会话只能运行一次；另一次终端生命周期需要创建另一个会话。

## 增加下一层

[组合组件](components.md) 引入可复用行为，场景所有权保持一致。[渲染内容](content.md) 引入可选格式实现。[流式输出](streaming.md) 解释为什么 Markdown 预览在源 EOF 前必须保持可修订。

需要在 shell 会话中保留活动区域时，使用 `program.NewInline` 和 `program.InlineConfig`。其 `Build` 接收 `*program.InlineRuntime`，增加有界的发布所有者。全屏与内联会话共享同一套场景和行为组件。
