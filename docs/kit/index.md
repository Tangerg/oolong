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

Keep live skins by pointer after construction. Their installed controller callbacks retain that appearance owner; copying the struct would separate its editable fields from the object actually measured and painted. These types state that they must not be copied, a contract checked by `go vet`. Construct another skin from its Config when an independent component is needed; passive content and immutable theme snapshots retain their ordinary value semantics.

Change retained appearance through owner commands: for example, `panel.SetBox(next)`, `dialog.SetWhere(next)`, `composer.SetPrompt(next)` and `composer.SetHints(next)`. Their getters return observations; changing a returned `Box` or hint slice cannot change the component. Setters enforce the same scene phase as behavior changes and invalidate presentation. Passive values such as `Box`, `Label` and `Image` keep ordinary value construction; install a changed value through its retaining owner's setter.

A custom skin can fill a base element's paint and measure slots before attachment. Slot changes while mounted are refused. Theme and glyph inputs affect appearance; they cannot decide which field is focused, which value is committed or which action is available.

Resolve glyph capability once for the host, then pass `glyphs.Ellipsis` to `ButtonConfig.Ellipsis`, `Paragraph.Ellipsis` and `Label.Ellipsis` where a truncation marker is wanted. An empty marker truncates silently. Measurement and drawing use the same marker; changing a paragraph's marker also changes its layout-cache identity.

Fixed-cell furniture uses `kit.CellGlyph`: every `Border` edge and corner, the corresponding `Glyphs` fields, meter marks, slider and scrollbar furniture, and sparkline levels. Construct custom marks with `kit.NewCellGlyph(text)` and handle `kit.ErrInvalidCellGlyph`: a nonempty mark must be one printable grapheme occupying exactly one terminal column. The zero value omits a mark. Width validation belongs to this value, so the components compose it without repeating admission checks during drawing. Box measurement and painting therefore agree on the cell reserved for each edge; drawing content cannot erase part of a wider border glyph.

`Glyphs.Rule` is a measured string pattern for horizontal rules in tabs and pinned transcript headers. It may contain several columns or graphemes; `Tabs.SetRule` retains the same contract. `Glyphs.Horizontal` supplies only the single-cell horizontal border mark. When updating custom glyph sets, construct border marks with `NewCellGlyph`, move rule patterns to `Rule`, and use `CellGlyph.String()` when passing a validated mark to a text-based consumer. Other measured text, including the multi-column ASCII ellipsis, remains a string.

Kit retains its controller's typed collection actions and revision-qualified item addresses when replacing paint slots. `SettingsConfig.Change` accepts a context and returns `(scene.ActionResult[bool], error)`, so the skin preserves accepted effects and failure independently. Its default feedback row is excluded from selectable list geometry and shares the controller’s error with semantic observation. See [typed controller actions](../base/index.md#typed-collection-and-controller-actions).

Box title and footer placement derive their minimum width, available text width and aligned origin from one border-label inset. Corners and the adjacent border remain visible under truncation and alignment.

`ButtonConfig`, `SliderConfig` and `TranscriptConfig` carry their base configuration in `Behavior`. For example, `kit.SliderConfig{Behavior: base.SliderConfig{Minimum: 0, Maximum: 100}, Glyphs: glyphs}` defines the value range once, in the controller configuration; the kit adds only appearance and pure formatting.

## Theme ownership

Create `kit.NewThemeOwner(initial)` for a live application palette, pass `owner.Scope()` through component Config values, and call `owner.Set(next)` on the UI owner turn. The owner invalidates attached consumers without rebuilding their elements or controllers. `scope.With(kit.ThemeOverride{...})` creates an immutable subtree branch; `Appearance` supplies an instance override. Resolution is application → ancestor scopes → local scope → instance.

Override fields are pointers so an explicit zero style can restore terminal defaults; constructors copy those pointed-to values. A component may instead receive a static `Theme`. Supplying both a nonzero Theme and Scope is invalid. Palette updates are refused during presentation or a pending frame, preserving the same purity boundary as controller mutations.

Terminal-fitted neutral roles use named contrast policies. Selection, added-content and removed-content contrast remain separate semantic policies even when their current ratios match; context and divider derive from their corresponding muted and border roles.

## Transcript example

`kit.NewTranscript(TranscriptConfig{Behavior: base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted}})` returns a skin and an error. Add source documents through `skin.Controller().AddDocument(document)`, then attach `skin.Element()`. The controller owns reading, selection, sticky headers and retention. Kit adds visual selection, match decoration and pinned-header appearance from the same candidate layout.

The renderer never retires content because it scrolled away, and a theme never finalizes a stream. See the tested [streaming example](https://github.com/Tangerg/oolong/tree/main/examples/streaming) and [streaming guide](../guides/streaming.md).
