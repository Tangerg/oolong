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

Change retained appearance through owner commands: for example, `panel.SetBox(next)`, `dialog.SetWhere(next)`, `editor.SetPrompt(next)` and `editor.SetHints(next)`. Their getters return observations; changing a returned `Box` or hint slice cannot change the component. Setters enforce the same scene phase as behavior changes and invalidate presentation. Passive values such as `Box`, `Label` and `Image` keep ordinary value construction; install a changed value through its retaining owner's setter.

A custom skin can fill a base element's paint and measure slots before attachment. Slot changes while mounted are refused. Theme and glyph inputs affect appearance; they cannot decide which field is focused, which value is committed or which action is available.

Resolve glyph capability once for the host, then pass `glyphs.Ellipsis` to `ButtonConfig.Ellipsis`, `Paragraph.Ellipsis` and `Label.Ellipsis` where a truncation marker is wanted. An empty marker truncates silently. Measurement and drawing use the same marker; changing a paragraph's marker also changes its layout-cache identity.

Fixed-cell furniture uses `kit.CellGlyph`: every `Border` edge and corner, the corresponding `Glyphs` fields, meter marks, slider and scrollbar furniture, and sparkline levels. Construct custom marks with `kit.NewCellGlyph(text)` and handle `kit.ErrInvalidCellGlyph`: a nonempty mark must be one printable grapheme occupying exactly one terminal column. The zero value omits a mark. Width validation belongs to this value, so the components compose it without repeating admission checks during drawing. Box measurement and painting therefore agree on the cell reserved for each edge; drawing content cannot erase part of a wider border glyph.

`Glyphs.Rule` is a measured string pattern for horizontal rules in tabs and pinned transcript headers. It may contain several columns or graphemes; `Tabs.SetRule` retains the same contract. `Glyphs.Horizontal` supplies only the single-cell horizontal border mark. When updating custom glyph sets, construct border marks with `NewCellGlyph`, move rule patterns to `Rule`, and use `CellGlyph.String()` when passing a validated mark to a text-based consumer. Other measured text, including the multi-column ASCII ellipsis, remains a string.

Kit retains its controller's typed collection actions and revision-qualified item addresses when replacing paint slots. `SettingsConfig.Change` accepts a context and returns `(scene.ActionResult[bool], error)`, so the skin preserves accepted effects and failure independently. Its default feedback row is excluded from selectable list geometry and shares the controller’s error with semantic observation. See [typed controller actions](../base/index.md#typed-collection-and-controller-actions).

Box title and footer placement derive their minimum width, available text width and aligned origin from one border-label inset. Corners and the adjacent border remain visible under truncation and alignment.

Every kit configuration over a base controller carries that controller's configuration in `Behavior`: `ButtonConfig`, `DialogConfig`, `EditorConfig`, `FormConfig`, `InputConfig`, `SettingsConfig`, `SliderConfig`, `TabsConfig`, `TranscriptConfig` and `TreeConfig`. For example, `kit.SliderConfig{Behavior: base.SliderConfig{Minimum: 0, Maximum: 100}, Glyphs: glyphs}` defines the value range once, in the controller configuration; the kit adds only appearance and pure formatting. A `Behavior` field naming an appearance slot the skin fills, such as `FormConfig.Behavior.PaintField`, must stay unset, and the constructor panics rather than ignore it.

## Theme ownership

Create `kit.NewThemeOwner(initial)` for a live application palette, pass `owner.Scope()` through component Config values, and call `owner.Set(next)` on the UI owner turn. The owner invalidates attached consumers without rebuilding their elements or controllers. `scope.With(kit.ThemeOverride{...})` creates an immutable subtree branch; `Appearance` supplies an instance override. Resolution is application → ancestor scopes → local scope → instance.

Override fields are pointers so an explicit zero style can restore terminal defaults; constructors copy those pointed-to values. A component may instead receive a static `Theme`. Supplying both a nonzero Theme and Scope is invalid. Palette updates are refused during presentation or a pending frame, preserving the same purity boundary as controller mutations.

Terminal-fitted neutral roles use named contrast policies. Selection, added-content and removed-content contrast remain separate semantic policies even when their current ratios match; context and divider derive from their corresponding muted and border roles.

## Transcript example

`kit.NewTranscript(TranscriptConfig{Behavior: base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted}})` returns a skin and panics for a configuration `base.TranscriptConfig.Validate` refuses; validate settings read from outside the program first. Add source documents through `skin.Controller().AddDocument(document)`, then attach `skin.Element()`. The controller owns reading, selection, sticky headers and retention. Kit adds visual selection, match decoration and pinned-header appearance from the same candidate layout. A paint slot that decorates the transcript calls `skin.Draw(frame)` and draws over the window it returns.

A dialog with `DialogConfig.Fit` sizes its panel to its content at the placed width, up to `Where.Height` or the available space, and follows the content as it grows or shrinks, so a form need not reserve rows for validation messages that may never appear. A panel whose content holds the keyboard draws its border in the palette's `Accent`, so the region that receives keys is the one that is marked. Place passive content in a scene with `scene.NewElement(scene.Passive(content))`, adding a `Key` to the configuration when automation must find it.

A status or header row that keeps a count, duration or state readable is a `kit.Split`: its `Start` truncates with the ellipsis and its `End` stays whole against the right edge, or disappears when the row cannot hold both.

Compose entries from passive blocks with `kit.Stack`, `kit.Indent`, `kit.Gap`, `kit.Label`, `kit.Framed` and `kit.Disclosure`. Each keeps its text projection on the rows and columns where it draws, so a composed entry copies, selects and searches as it is shown; a part that projects no text keeps blank rows. A `kit.Entry` labels any of them: its `Body` may be a paragraph, a rendered document or a diff. `kit.Framed` puts a body inside a `kit.Box` as one block, and `kit.Disclosure` is a summary whose marker says whether there are details and whether they are shown; its owner decides when it is open.

A base control with an intrinsic width reports it when measured across, so a panel around a `base.Completion` measures the widest candidate plus its own frame. Size a popup from `panel.Element().Measure(layout.Across, height)`, where height is the room it may take, rather than adding the frame to `Completion.Width` by hand.

The renderer never retires content because it scrolled away, and a theme never finalizes a stream. See the tested [streaming example](https://github.com/Tangerg/oolong/tree/main/examples/streaming) and [streaming guide](../guides/streaming.md).
