---
title: Compose behavior and appearance
description: Use stable scene parts and choose a skin independently of interaction policy.
contentType: How-to
---

# Compose behavior and appearance

Language: English | [简体中文](../zh/guides/components.md)

## Start with a tested composition

Run `go run ./examples/picker` and read the [picker example](https://github.com/Tangerg/oolong/tree/main/examples/picker). Its application selects data and product behavior; base owns selection, navigation and actions; kit provides appearance; scene owns real structure.

Use a controller's typed operations for programmatic changes. For example, `base.NewButton(ButtonConfig)` exposes one activation action through its element, and forms expose their value and submission operations. Do not create a second semantic action in the skin merely to make it discoverable.

## Keep receipts with the owner that issued them

A form calls `Submit(id)` with an opaque `base.SubmissionID`. Return `scene.Applied` for synchronous success or `scene.Started` after accepting asynchronous work. Deliver completion on the owner turn with `form.Settle(id, outcome)` after the callback returns. A different form, a superseded attempt, or an attempt still inside its callback cannot consume that receipt.

An editor element snapshot carries its captured text: call `element.Text()` even after editing. Use the opaque `base.ElementID` to key associated payloads, and pass it only to an editor sharing the issuing `EditSession`. Use `tabs.SetRule(glyph)` to change both the visible rule and the space it occupies; the empty string removes it.

## Assemble real parts

A `scene.Group` receives keyed `scene.Item` children and a layout function. An overlay is a `scene.Group` with `GroupConfig.Overlay`; `GroupConfig.Modal` defines its modal boundary. These same parts determine paint, focus, routing, lifecycle and semantic observation.

Construct once and keep the component instance. Repeated calls to `Element()` are safe; reconstructing a controller every paint discards its identity and gestures. Change group membership on the owner before `Prepare`, then render the stable resulting tree.

## Replace a skin

Keep the base controller and set its unmounted element's paint or measure slot. Draw the supplied `scene.Frame` using core text, grid and layout primitives. Read the controller for value and state. Use the provided candidate geometry for paint; events use the arrival-frame projection through `scene.Event.Read`.

For a skin that belongs only to a particular composition, set `scene.Item.Paint` on the child relation. Removing the child reveals its own painter again. If one controller replaces the children of multiple sibling groups together, the parent can preflight and install those direct child relations with `CheckChildItems` and `SetChildItems`.

The [base reference](../base/index.md) and [kit reference](../kit/index.md) define what belongs on each side. The [state example](https://github.com/Tangerg/oolong/tree/main/examples/state) shows four owned drafts with submission normalization and rejection that preserves the draft, a `ConfirmControl` backed by one domain owner shared with an external reset action, and live ThemeOwner / ThemeScope composition.

## Use terminal images and layout-aware shortcuts

Run `go run ./examples/image` from the repository root in Kitty or Ghostty. Press `1` for the whole PNG, `2` for its left half or `3` for its right half. The example transmits the PNG once and changes the placement's source rectangle. You can also pass a PNG path. [Image example](https://github.com/Tangerg/oolong/tree/main/examples/image)

For a local terminal, enable the two capabilities independently when constructing a screen:

```go
features := term.Features{
    Probe:    true,
    Keyboard: term.KeyboardCompatible,
}
```

Pass `features` as the `Terminal` field of `program.ScreenConfig` or `program.InlineConfig`. The startup probe sends a one-pixel PNG query and a device-attributes request in the same round trip. A matching success confirms direct PNG support. An answering terminal that omits or rejects the graphics query cannot be promoted to Kitty graphics by its name. An incomplete exchange may still use terminal and environment hints. Check `runtime.Images().Protocol().Supports(graphics.Live)` and `runtime.Images().CellSize()` before transmitting. The cell-sized widget uses `Alt` when it cannot display the picture.

`runtime.Images().Transmit(ctx, png)` returns an admission error or a buffered channel carrying one `graphics.UploadResult`. Writer admission is not terminal creation. Receive the result in `program.Work.Run`, then install its `Resource` in `Apply` on the UI owner. Keep responsibility for releasing a successful resource even if task delivery is revoked. The image and Mermaid examples demonstrate this lifetime. The context cancels an unfinished upload; a successfully returned resource lives until explicit release or terminal handover/close.

Oolong sends Kitty image numbers (`I`) and accepts the physical image ID (`i`) allocated by the terminal. It never chooses a physical ID by a local counter. Request correlations are bounded, never reused across terminal instances in one process, and retained after cancellation until a late allocation can be deleted. Refused deletion admission is retried when output capacity returns. Terminal refusal, a malformed matching reply, input/output failure, cancellation and response timeout are explicit upload outcomes. A terminal session requires exclusive foreground ownership of input and output. Correlation is process-scoped; the protocol does not authenticate arbitrary preexisting or foreign bytes.

Set `kit.Image.Source` to a half-open rectangle in original PNG pixels, such as `image.Rect(16, 8, 48, 40)`. The zero rectangle selects the full image. For concurrent views of one transmission, give each view its own `Placement` ID. Remove its placements before releasing a live image. A terminal handover revokes its resources and a new transmission is required on return. [Kitty graphics specification](https://sw.kovidgoyal.net/kitty/graphics-protocol/)

`term.KeyboardCompatible` requests unambiguous input and alternate key codes. Keypad-specific bindings such as `kp-enter` take precedence over their ordinary equivalents. Inspect `runtime.Environment().Keyboard()` for the reported flags. Bind shortcuts with `keymap.Map` and resolve an incoming press or repeat with `ActionFor` or `Matcher.Handle`. An explicit binding in the active layout wins. For example, the alternate base key lets a `Ctrl+A` binding match the same physical key under a Cyrillic layout. Editors insert `Key.Text` when the terminal supplies it. Add `input.KeyboardReportEvents` for release events or `input.KeyboardReportText` for associated text. Requesting associated text also enables escape encoding for all keys as required by the protocol. [Keyboard example](https://github.com/Tangerg/oolong/tree/main/examples/keys) · [Kitty keyboard specification](https://sw.kovidgoyal.net/kitty/keyboard-protocol/)

The default `input.XTermKeyboard` dialect accepts XTerm's exact modified F3 forms, including Shift, Alt and Ctrl. A valid keyboard-flags reply identifies `input.KittyKeyboard` even when no enhancements are enabled; `CSI ... R` is then treated as a cursor report. The dialect also owns the shared function-key modifier fields: XTerm Meta and Kitty Super use the same bit. `CSI u` always follows its explicit Kitty grammar. Acquisition sends one asynchronous keyboard query when keyboard features or startup probing are requested, including over SSH and after handover. `Terminal.Keyboard()` remains the startup probe's feature snapshot; protocol-family observations used by the decoder do not rewrite it. A custom parser may select its initial `input.ParserConfig.Keyboard` or `input.StreamConfig.Keyboard`. Cursor-query owners must use DEC private CPR (`CSI ?6n`): an unmarked reply at row 1 is byte-for-byte identical to a legacy modified F3 key. [XTerm control sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html)

Clipboard transport defaults to native OSC 52, including inside tmux. `TMUX` and `TERM` do not prove that DCS passthrough is enabled. Select `term.Features{Clipboard: clipboard.Config{TmuxPassthrough: true}}` only for a session explicitly configured for it; the SSH adapter uses the same configuration for the accepted remote terminal. Native application copying through tmux uses its `set-clipboard on` path. Passthrough requires tmux's separate `allow-passthrough` policy. [tmux clipboard](https://github.com/tmux/tmux/wiki/Clipboard) · [tmux passthrough](https://github.com/tmux/tmux/wiki/FAQ)

Use `runtime.Clipboard().Read(ctx, receive)` for a directed read. Admission errors return immediately; the callback receives the read result on the UI owner. Custom hosts implement `program.ClipboardReader.ReadClipboard(context.Context) (string, error)` and may block in that worker call. Caller cancellation or runtime shutdown revokes callback delivery. Ordinary bracketed paste remains an input event; an OSC 52 answer settles only the read that requested it.

`editor.Paste(ctx, complete)` captures the original document, revision, caret, selection and attachment. Moving focus cannot redirect the result to another editor. Changing the captured edit location or detaching the editor makes completion stale; an unmounted editor may start a read only if it has never been attached. Both admission and completion errors are explicit, and keyboard-triggered failures can be read through `editor.PasteError()`. Empty clipboard contents succeed without deleting the current selection.

OSC 52 has no request identifier. A response timeout ends waiting after ten seconds but does not make a late reply safe for a newer request. Cancellation and terminal handover obey the same rule: `clipboard.ErrPending` remains until the earlier response is consumed. Repeated reads cannot bypass that unresolved query. Direct users of `clipboard.Channel` must provide a send callback with atomic admission; a partially emitted query is already sent and must retain its response obligation.

A complete matching answer received before its deadline that exceeds the clipboard or decoder budget settles with `clipboard.ErrResponse` and releases its query's wire slot. A cancelled result stays cancelled. An unfinished frame or a foreign selection cannot release the slot. Custom decoder hosts must forward a complete oversized OSC 52's `input.OSCRejection.Prefix` to `clipboard.Channel.Reject` with its transport receive time; `input.Stream` handles this routing automatically.

Ghostty implements both wire protocols. A custom libghostty host may build or configure them independently. Oolong uses the capabilities of the connected terminal. Its embedded SSH server adapter handles keyboard input but does not expose a live image host. Image passthrough through a multiplexer requires separate transport support. [Ghostty terminal features](https://ghostty.org/docs/features)
