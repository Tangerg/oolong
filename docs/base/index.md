---
title: Behavior components
description: Compose interaction from scene primitives while keeping each value in one controller.
contentType: Conceptual
---

# Behavior components

Language: English | [简体中文](../zh/base/index.md)

## The role of base

Base supplies behavior that remains useful with a different appearance: editing, selection, lists, forms, dialogs, tabs, scrolling, transcripts and virtual windows. It imports core contracts and never imports kit or the application runtime. A base component exposes a stable `Element()` and concrete typed operations; a skin paints that same behavior part.

## Values and parts

A field controller owns its value. Form output and semantic values are projections. Validation and submission are typed operations with explicit results, rather than a callback that reads another writable copy. `EditSession` owns text and revision maps; editors own view-level interaction while composing the document.

`Group` and `Overlay` remain scene mechanisms. Dialog focus policy, tab activation and list selection reuse those mechanisms without creating another attachment or routing tree. Replacing appearance preserves the behavior part's element identity and action binding.

## Typed collection and controller actions

List, Filter, Select, MultiSelect, Tree, Table, Settings and Completion expose `base.ActionReadItems` on their existing element. Pass `CollectionQuery{ContentRevision: observed.ContentRevision, Start: start, Limit: limit}` to read a copied `CollectionPage[T]`, including the current selection and total count. A zero limit uses `DefaultCollectionPageSize`; requests beyond `MaxCollectionPageSize` are unavailable. Item values keep the same reference ownership as the controller's ordinary snapshot API. Tree pages contain the currently shown `Shown[T]` rows, filter pages contain ranked matches, and multi-select pages contain `ChoiceState[T]` with each option's value, label and chosen state. Completion pages contain `Candidate` values with detached match offsets.

`CollectionIndex{ContentRevision: observed.ContentRevision, Index: index}` addresses an item in that exact order. Replacing items, filtering, sorting or expanding a branch invalidates old positional addresses. An invalid or stale address returns `scene.ErrUnavailable` instead of clamping to a different item. Read the current semantic revision and request a new page before retrying.

| Controller | Action | Parameter | Result |
| --- | --- | --- | --- |
| List, Filter, MultiSelect, Tree, Table, Settings, Completion | `scene.ActionSelect` | `CollectionIndex` | `CollectionSelection[R]`, where `R` is the page item type |
| Select | `scene.ActionSelect` | `T` | Selected value `T` |
| Completion | `scene.ActionActivate` | `CollectionIndex` | Accepted `Candidate` |
| Filter | `scene.ActionFill` | Pattern `string` | Current pattern |
| MultiSelect | `scene.ActionSetValue` | Chosen values `[]T` | Actual chosen values, in option order |
| Tree | `scene.ActionExpand`, `scene.ActionCollapse` | `CollectionIndex` | Current branch openness |
| Table | `base.ActionSort` | `SortOrder` | Detached current `*SortOrder` |
| Table | `base.ActionClearSort` | `struct{}` | `struct{}` |
| Settings | `scene.ActionIncrement`, `scene.ActionDecrement`, `scene.ActionActivate` | `CollectionIndex` | Domain acceptance `bool` |
| Viewport, Transcript | `base.ActionScroll` | `ScrollRequest` | Current row offset |
| Form | `base.ActionReadSubmission` | `struct{}` | Current `Submission`, including its opaque ID |
| Form | `base.ActionCancel` | The `SubmissionID` returned by submit | Current `Submission` |

Actions compose the same owner operations as pointer and keyboard input. Repeating an explicit table `SetSort` order is unchanged; `SortBy` retains its interactive toggle behavior. Multi-select set-value rejects unavailable values and excess choices before mutation, while equivalent option aliases still name one choice. A form cancellation request reports `Started` until the application settles the attempt; an old attempt ID cannot cancel a later submission.

Completion activation accepts the addressed candidate in one operation, independently of the current selection. A later offer invalidates its address, so automation does not need a selection-then-accept sequence that could cross offer versions. Physical Enter continues to accept the current candidate. Closed or expired offers are unavailable to all collection actions.

Button and Confirm publish accepted `Applied` or `Started` outcomes even when a later step returns an error. Physical input also publishes its retained UI error, including clearing an earlier error without changing the value. `Button.Press`, `Confirm.Request` and semantic actions return their errors directly and preserve the last physical-input feedback. Confirm batches synchronous domain notifications until its request outcome and UI feedback have settled; the domain remains the sole owner of the answer.

Table column interpretation belongs to its `SetLess` comparator. The row controller has no column schema: sort availability checks a nonnegative index, an installed comparator and nonempty rows, and callers must use columns supported by that comparator.

`Settings.Change` returns `scene.ActionResult[bool]`: `Value` says whether input was accepted, and `State` reports the domain owner's actual outcome. An accepted action at a value boundary returns `Value: true, State: scene.Unchanged`. Settings neither duplicates the value nor infers mutation from input consumption.

## Editor insertion positions

`base.Caret{Line, Byte}` addresses a byte offset in a logical line. `base.Point{Row, Col}` addresses terminal cells in a layout. The units remain explicit across wrapping and Unicode text. Vertical movement retains its desired cell column, including zero, while atomic elements snap to a boundary in the movement direction; layout queries do not reset that intent.

Multiline editors opt into `text.LayoutConfig.ReserveCaret`. A full logical line receives an empty continuation row for its final insertion position. Height, scrolling, vertical movement, hit testing and selection all consume that same immutable projection, including gutter and resize handling. Ordinary text layouts keep their existing passive row count unless they request this editing policy.

## Rich content behavior

`Transcript.AddDocument` connects a `StreamDocument` once. Changes, EOF preparation, partial-result policy, publication admission, retirement and subscription cleanup then follow the owning objects. A caller does not repeat a Feed/Changed/Finish/Commit protocol.

`Selection` stores content-relative logical endpoints. `Scroll` owns reading intent. `Sticky` owns content identities eligible for pinning. Transcript's frame candidate composes their projections; painting does not advance their state. `Scan` is a bounded synchronous search operation that callers may schedule as scoped work and accept only for the matching content revision.

`ScrollLayout.Reveal`, `Resize` and `Anchor` refine candidate geometry without changing end-following intent. Explicit `Scroll` commands own that transition; an aborted frame cannot alter it. A late publication refusal invalidates Transcript's scene element after storing the error, while preserving the retained content revision and selection.

Public controller commands publish invalidation through that same element after the entire mutation settles. A composite operation uses the atomic primitives internally and publishes once; callers do not add a second notification path. Commands that leave the value unchanged do not advance the model revision.

A transcript scrolls one full viewport. A sticky header overlays its top rows without reducing the scroll range or page size. Custom skins draw `TranscriptView.Body` from `TranscriptView.BodyFrom`; this body origin includes the rows hidden by the overlay and differs from `Scroll.Offset`. Pointer mapping and reading anchors use the same projection. Recomputing the header from the reduced body origin would create a circular layout decision at section boundaries.

## Large datasets

`VirtualWindow.Resolve` runs explicitly on the owner before painting. A `VirtualSource` supplies dataset revision, stable keys and pure height measurements. The window retains only materialized elements and bounded measurements, reusing an element while its key remains present. Leaving the window detaches its target; reappearing later receives a new mount lifetime.

Estimated total height remains marked inexact until all relevant heights are known. The metadata index and retained measurements are bounded by `MaxItems`, key storage by `MaxKeyBytes`, and new measurement work in each resolution by `MaxMeasurementsPerResolve`. Measurements survive scrolling for the current source revision and width so known prefix positions cannot revert to estimates. Only visible and overscan elements remain mounted. See [history and virtualization](../guides/history.md) for the separate ownership of archived content.

With no Build callback the virtual window can measure a range without mounting any rows. Its scene layout describes only actual children. An explicit scroll offset remains the request while estimated heights are refined; consult the resulting range for the final offset.

## Transcript search

Keep the layout you searched: `source := transcript.Layout()`. Read `source.Rows(...)`, compute matches, then handle the error from `transcript.SetMatches(source, matches)`. A changed width, different transcript or replaced content can make physical rows obsolete. On `base.ErrStaleMatches`, take the current layout and recompute. A custom painter reads `MatchesIn(source, from, to)` and `CurrentMatch(source)` using the layout it is drawing.
