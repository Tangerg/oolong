---
title: Compose a themeable picker
description: Combine layout, focus, input, filtering, and themes into a reusable terminal picker.
contentType: Tutorial
---

# Compose a themeable picker

Language: English | [简体中文](zh/components.md)

This tutorial turns the core component contract into a reusable interface without
giving product policy to the library. You will combine a text editor, fuzzy filter,
layout, focus, pointer routing, and a terminal-aware theme.

Complete code: [`examples/picker`](https://github.com/Tangerg/oolong/tree/main/examples/picker)

## Before you begin

Finish [Build your first interface](getting-started.md) first. This tutorial assumes
you know how `program.Run`, `Draw`, and `Handle` fit together.

Add the component module beside `core`:

```sh
go get github.com/Tangerg/oolong/core@latest
go get github.com/Tangerg/oolong/components@latest
```

Upgrade both modules together. Oolong modules share one coordinated release version.

## Keep each decision in one layer

The picker is a product component assembled from smaller framework components:

```mermaid
flowchart TD
    picker["application picker"] --> composer["kit.Composer appearance"]
    picker --> filter["headless.Filter behavior"]
    composer --> editor["headless.Editor behavior"]
    picker --> layout["core/layout geometry"]
```

`Filter` owns matching, selection, and scrolling. It does not own the query editor or
the look of a row. The application owns both because only the application knows where
the query belongs and what choosing an item means.

## Resolve terminal facts once

Build appearance from the terminal driven by this runtime. Do not read process
environment variables inside a component because a local session and an SSH session
may describe different terminals.

```go
func newPicker(runtime *program.Runtime, items []string) *picker {
    theme := kit.Suited(runtime.Environment().Ground())
    glyphs := kit.GlyphsFor(runtime.Environment().Locale())

    p := &picker{runtime: runtime, theme: theme}
    p.query = kit.Composer{
        Theme:  theme,
        Prompt: glyphs.Marker + " ",
    }
    p.query.Editor().Placeholder = "type to narrow"
    p.configureItems(items)
    return p
}
```

`Theme` names semantic roles such as `Accent`, `Selection`, and `Danger`. `Glyphs`
answers a different question: which characters this terminal can represent.

## Let the filter own filtering

Configure state-changing behavior through methods. `SetItems` replaces the values and
the projection that says how they read as one source; `SetPattern` narrows that source.
Both invalidate the cached ranking, so no intermediate source can disagree with its
visible results.

```go
func (p *picker) configureItems(items []string) {
    p.list = &headless.Filter[string]{
        Row: p.drawRow,
    }
    p.list.SetItems(items, func(item string) string { return item })
}
```

The row callback is the appearance seam. It receives the item, fuzzy-match offsets,
and selection state. Draw the same behavior with a different shape by replacing this
callback, not by replacing `Filter`.

## Draw passive charts

Charts are completed content, not controllers. `Sparkline` and `BarChart` draw into a
`grid.View`, implement `headless.Block`, and own no selection, clock, or history. The
application retains the measurements and decides when they change.

```go
trend := kit.Sparkline{
    Theme: theme, Glyphs: glyphs,
    Values: []float64{0.12, 0.18, 0.15, 0.24, 0.31},
    Minimum: 0, Maximum: 1,
}

usage := kit.BarChart{
    Theme: theme, Glyphs: glyphs, Maximum: 100,
    Bars: []kit.Bar{
        {Label: "CPU", Value: 42, Text: "42%"},
        {Label: "RAM", Value: 68, Text: "68%"},
    },
}
```

Sparkline uses the newest samples that fit. A valid `Minimum`/`Maximum` pair fixes
its domain, as completion and resource metrics usually require; the zero pair derives
the scale from the visible window. BarChart is deliberately horizontal and
non-negative: it serves labelled dashboard comparisons without introducing axes, a
canvas, or a constraint solver. Compose either block inside `headless.Static`, a
transcript, a panel body, or any application-owned layout.

## Divide the assigned region

A headless widget draws into `headless.Frame`. The frame carries the same clipped
grid view as a core component plus the transaction used for input geometry.

```go
func (p *picker) Draw(frame headless.Frame) {
    rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
        {Size: layout.Fixed(1)},
        {Size: layout.Flex(1)},
        {Size: layout.Fixed(1)},
    }))
    p.query.Draw(rows[0])
    p.list.Draw(rows[1])
    kit.Label{
        Text:  fmt.Sprintf("%d of %d", p.list.Matched(), p.list.Len()),
        Style: p.theme.Subtle,
    }.Draw(rows[2].View)
}
```

The application chooses the vertical composition. Neither child needs to know what
is above or below it. Replace the slots with `layout.Across` to make a two-pane
interface without changing either child.

## Route events by ownership

Offer navigation keys to the list and text input to the composer. Handling and
changing are different facts: Backspace is handled at the beginning of an empty
editor but changes nothing. Compare `Editor.Revision` around input to publish the
pattern only when semantic content changed, without guessing from keys or action
names.

```go
func (p *picker) Handle(event input.Event) bool {
    if key, ok := event.(input.Key); ok && key.Down() {
        switch key.Code {
        case input.Enter:
            p.choose()
            return true
        case input.Up, input.Down, input.PageUp, input.PageDown:
            return p.list.Handle(event)
        }
    }
    before := p.query.Editor().Revision()
    handled := p.query.Handle(event)
    if p.query.Editor().Revision() != before {
        p.list.SetPattern(p.query.Editor().Text())
    }
    return handled
}
```

For pointer input, stage child rectangles in a `headless.Snapshot` during `Draw` and
route against `Snapshot.Value` in `Handle`. The snapshot becomes visible only after
the complete root frame succeeds, so an event never sees half of a new layout. The
complete picker demonstrates this pattern.

## Install the headless root

`headless.NewRoot` is the only bridge from a live headless tree to
`program.Component`. It commits all nested presentation snapshots atomically.

```go
err := program.Run(context.Background(), program.Config{
    Root: func(runtime *program.Runtime) program.Component {
        return headless.NewRoot(newPicker(runtime, files()))
    },
    Terminal: term.Features{Probe: true, Mouse: true},
})
```

`program.Config.Terminal` accepts only optional `term.Features`. `Root` already means
alternate-screen ownership and `Inline` already means the ordinary terminal screen,
so there is no second `AltScreen` switch to keep consistent. A transport adapter uses
`program.Config.TerminalConfig()` when it acquires the underlying terminal session.

Passive content such as a finished Markdown document does not need this transaction.
Adapt it with `headless.Static` only when placing it inside a live widget tree.

## Change appearance without changing behavior

Copy a theme value and replace semantic roles:

```go
theme := kit.Suited(runtime.Environment().Ground())
theme.Accent = grid.Style{FG: grid.RGBColor(0xD7, 0x8B, 0xFF)}
theme.Selection = grid.Style{BG: grid.RGBColor(0x32, 0x27, 0x3B)}
```

Pass the modified value to kit components. Headless state remains unchanged. For a
fully custom design system, keep `components/headless` and draw every appearance seam
with your own package; `headless` never imports `kit`.

## Keep value identity and navigation explicit

A controlled field owns edits through its accessor. Configure `Select.Same` and
`MultiSelect.Same` before installing options; `headless.Equal[T]` compares ordinary
comparable values. Labels remain display text, so translating a label cannot change
which value is selected. `Chosen`, `Taken`, and `Ask` only project state. Call `Sync`
to explicitly reconcile external changes; input and validation also synchronize.

`Text` exposes its answer, cursor, mask, clipboard, gutter and cursor style directly.
Use `SetText`, `SetCursor`, `Handle`, or `Do` to edit through the accessor's acceptance
boundary. `Text` does not expose its private Editor. `kit.Composer` still exposes its
Editor because the composer is an appearance wrapper around that editor.

Moving a list selection or navigating an Editor cursor requests visibility once. `Scroll.Reveal` survives an aborted
frame and is consumed when a complete frame presents it. Subsequent manual scrolling
wins, and `Scroll.FollowingEnd` reports the follow policy. On `kit.Transcript`, set
`Current` to highlight a match and call `RevealMatch(index)` to navigate to one.
Editor requests resolve at the next frame's wrap width; resizing an already settled
view preserves manual scrolling. Filter query resets return to the first result.

For container children that move, assign stable keys and retain the same widget
instances. `FocusIndex` addresses the current collection. Pointer input resolves the
presented attachment, so a removed or replaced child cannot give its old click or
capture to a new instance. Use pointer widgets when identity must survive `Set`.

A `Pointer` belongs to one control. Call `Stage(frame, area)` in Draw, then Handle
and `Clicked(button)` in the event handler. Draw only reads `Over()` and `Pressing()`;
a fast press and release needs no frame between them. `PointerRegion` routes a child
by its continuous presentation lifetime: an absent child returning later cannot
resume an old gesture. Tabs and Viewport use that same owner.

After `List.SetItems`, pointer selection waits for the replacement to be drawn.
Custom tab strips use `Tabs.SelectPresented(index)` for hits in their committed strip;
`Tabs.Select(index)` remains programmatic navigation in the current collection.
Changing key bindings cancels pending sequences, and replaced choice collections or
modal owners do not inherit delayed actions. Settings value actions remain tied to
the row where their sequence began.

`Completion.Renderer` pairs `DrawRow` with `Width`; replacing it can change the entire
candidate-row layout. `Select.Row` and `MultiSelect.Row` customize one-row choices.
An external field can implement `ThemedField.DrawWith` to receive the Form look for
one frame, using the same channel as built-in fields.

## Run and verify the slice

```sh
go run ./examples/picker
cd examples && go test ./picker
```

You now have the component-level composition model. Continue with
[Render Markdown, code, and mathematics](content.md) to add passive content, or
[Build bounded streaming output](streaming.md) to connect background work.
