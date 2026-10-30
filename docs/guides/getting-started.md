---
title: Build your first interface
description: Run a scene with one terminal owner and a pure painter.
contentType: Tutorial
---

# Build your first interface

Language: English | [简体中文](../zh/guides/getting-started.md)

## Run the complete program

Oolong requires Go 1.27. The public module is `github.com/Tangerg/oolong/ui`; the `core`, `base`, `kit`, `automation` and `uitest` packages are layers inside that module. In a repository checkout, run the tested [hello example](https://github.com/Tangerg/oolong/tree/main/examples/hello) with `go run ./examples/hello`.

For an application using a released UI module, put the following program in `main.go`, add the module to `go.mod`, then run `go run .`. This complete program is compiled by the documentation test, and its Chinese version uses exactly the same code.

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

## Follow the ownership boundaries

`NewScreen` validates construction without opening a terminal. `Run` starts the one session and invokes `Build` after the host is ready. `Build` returns a stable `scene.Element`. `Paint` only writes the frame's borrowed cells; `Handle` changes application state on the UI owner. `Semantics` reads that same state without producing changes.

A frame is a candidate until the compositor admits its encoded output. Scene geometry, cell diff baselines and publication responsibilities commit together. Never retain a writable `scene.Frame` or `grid.View` after the callback returns. A session can run once; create another session for another terminal lifetime.

## Add the next layer

[Compose components](components.md) adds reusable behavior without changing scene ownership. [Render content](content.md) introduces optional format implementations. [Stream output](streaming.md) explains why a provisional Markdown preview remains revisable until source EOF.

Use `program.NewInline` with `program.InlineConfig` for a live block within the shell session. Its `Build` receives `*program.InlineRuntime`, which adds the bounded publication owner. Full-screen and inline sessions share the same scene and behavior components.
