---
title: Appearance and composition
description: Apply a theme or replace a skin without rebuilding behavior or action identity.
contentType: Conceptual
---

# Appearance and composition

Language: English | [简体中文](../zh/kit/index.md)

## A polished layer over behavior

Kit provides theme, glyph choices, passive content blocks and useful compound controls. It composes base controllers and scene groups; it does not own another focus router, selection model or semantic registry. This is the intended platform → Base UI → shadcn-style separation, expressed as ordinary versioned Go packages.

## Stable behavior parts

Construct interactive components with their explicit `Config` values. `Controller()` returns the actual behavior owner. `Element()` returns its scene attachment, or the compound component's scene group when it has several real parts. It does not create a fresh element on each call.

A custom skin can fill a base element's paint and measure slots before attachment. Slot changes while mounted are refused. Theme and glyph inputs affect appearance; they cannot decide which field is focused, which value is committed or which action is available.

## Theme ownership

Create `kit.NewThemeOwner(initial)` for a live application palette, pass `owner.Scope()` through component Config values, and call `owner.Set(next)` on the UI owner turn. The owner invalidates attached consumers without rebuilding their elements or controllers. `scope.With(kit.ThemeOverride{...})` creates an immutable subtree branch; `Appearance` supplies an instance override. Resolution is application → ancestor scopes → local scope → instance.

Override fields are pointers so an explicit zero style can restore terminal defaults; constructors copy those pointed-to values. A component may instead receive a static `Theme`. Supplying both a nonzero Theme and Scope is invalid. Palette updates are refused during presentation or a pending frame, preserving the same purity boundary as controller mutations.

## Transcript example

`kit.NewTranscript(TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})` returns a skin and an error. Add source documents through `skin.Controller().AddDocument(document)`, then attach `skin.Element()`. The controller owns reading, selection, sticky headers and retention. Kit adds visual selection, match decoration and pinned-header appearance from the same candidate layout.

The renderer never retires content because it scrolled away, and a theme never finalizes a stream. See the tested [streaming example](https://github.com/Tangerg/oolong/tree/main/examples/streaming) and [streaming guide](../guides/streaming.md).
