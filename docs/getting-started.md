---
title: Build your first Oolong interface
description: Build and run a full-screen counter directly on Oolong's lowest public interface layer.
contentType: Tutorial
---

# Build your first Oolong interface

Language: English | [简体中文](zh/getting-started.md)

This tutorial creates a full-screen counter with the lowest public layer. You will
run one component, draw two lines, handle keys, and stop without importing the
component library.

## Prerequisites

Use Go 1.27 or newer and a terminal that can enter raw mode. The program also runs
on Windows Terminal and through an SSH session that supplies a terminal.

## Create the module

Create an empty module and add `core`:

```sh
mkdir oolong-hello
cd oolong-hello
go mod init example.com/oolong-hello
go get github.com/Tangerg/oolong/core@latest
```

Create `main.go`. Start with the imports and runtime entry point:

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

`Config.Root` gives the application the whole screen and restores the previous
screen when the program stops. Add the component below `main`:

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

`Draw` uses coordinates local to the view. The view clips writes at its boundary.
`Handle` returns `true` only for events the component owns.

## Run the interface

Run the module from a terminal:

```sh
go run .
```

Press keys to change the count. Press `q` or `Ctrl+C` to stop. The runtime redraws
after state changes and parks when no input or scheduled work exists.

## Add the next layer

The core contract stays the same as the interface grows:

- Add layout primitives from `core/layout` when a view needs regions
- Add `components/headless` for reusable behavior without appearance
- Add `components/kit` for the default theme and compound components
- Replace `Config.Root` with `Config.Inline` when finished output must remain in
  terminal scrollback

Run [`examples/hello`](https://github.com/Tangerg/oolong/tree/main/examples/hello) for this exact first slice. Continue with
the [example catalog](https://github.com/Tangerg/oolong/tree/main/examples) or
[compose a themeable picker](components.md) to add the component layer.
