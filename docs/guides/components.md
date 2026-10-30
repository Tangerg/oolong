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

## Assemble real parts

A `scene.Group` receives keyed `scene.Item` children and a layout function. An overlay uses `scene.Overlay` for its actual attachment and modal boundary. These same parts determine paint, focus, routing, lifecycle and semantic observation.

Construct once and keep the component instance. Repeated calls to `Element()` are safe; reconstructing a controller every paint discards its identity and gestures. Change group membership on the owner before `Prepare`, then render the stable resulting tree.

## Replace a skin

Keep the base controller and set its unmounted element's paint or measure slot. Draw the supplied `scene.Frame` using core text, grid and layout primitives. Read the controller for value and state. Use the provided candidate geometry for paint; events use the arrival-frame projection through `scene.Event.Read`.

The [base reference](../base/index.md) and [kit reference](../kit/index.md) define what belongs on each side. The [state example](https://github.com/Tangerg/oolong/tree/main/examples/state) shows four owned drafts with submission normalization and rejection that preserves the draft, a `ConfirmControl` backed by one domain owner shared with an external reset action, and live ThemeOwner / ThemeScope composition.
