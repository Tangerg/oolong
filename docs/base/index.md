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

Initial settings belong in the component's `Config`. Change a live controller through its setters, such as `SetLabel`, `SetRows`, `SetLook` and `SetKeys`; read the corresponding `Label()`, `Rows()`, `Look()` and `Keys()` methods. These commands check the scene's mutation phase and invalidate the affected element, so labels, geometry and automation observations cannot change behind an unchanged model revision. Presentation and pending-frame borrows refuse these mutations before the setting changes. Composite controllers delegate shared settings to their existing owner: Text uses its Editor's editing settings, and Table uses its List's row painter, navigation wrapping and bindings.

Use `NewCompletion(CompletionConfig)`, `NewList(ListConfig[T])`, `NewSettings(SettingsConfig[T])` and `NewSticky(StickyConfig)` for initial settings instead of nonempty controller literals. Zero controllers remain usable. `Completion.CurrentToken()` queries the configured token source under a read-only borrow and returns the token and its availability; `SetCurrentToken` replaces that source.

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

Opening or closing another tree branch preserves the selected node. Closing an ancestor of the selected node selects that ancestor. The visible list owns selection; a branch transition maps the node identity into its new rows and revokes pending key sequences before notifying subscribers.

Button and Confirm publish accepted `Applied` or `Started` outcomes even when a later step returns an error. Physical input also publishes its retained UI error, including clearing an earlier error without changing the value. `Button.Press`, `Confirm.Request` and semantic actions return their errors directly and preserve the last physical-input feedback. Confirm batches synchronous domain notifications until its request outcome and UI feedback have settled; the domain remains the sole owner of the answer.

UI feedback belongs to the latest UI request. If a callback synchronously starts another request, the earlier return cannot replace that newer feedback. Form cancellation follows the same rule within its existing submission identity; a successful retry clears the earlier cancellation error. Typed programmatic and semantic requests keep returning their own result and error directly.

`Table.SetSortColumns` accepts the column owner's lookup from a column index to a pure three-way comparator. A nil comparator marks that column unavailable to both direct sorting and semantic actions. The lookup owns column support and comparison together; the table keeps no second column count or schema. Replace the lookup when supported columns or their ordering change. Removing an active sort column clears the reported sort while preserving row order.

Form preserves the callback's `Applied` or `Started` result and submission ID alongside an error. `Started` remains pending and can still be settled; its accompanying error is available through `Submission` and `Error`. An explicitly superseded attempt reports `ErrSubmissionStale` without erasing effects already accepted by its callback.

`Form.Set` validates the complete replacement before changing the field relation. A successful replacement revokes the form's old deferred shortcuts and partial key sequences even when its focused field remains mounted. A rejected replacement preserves the current relation and keyboard sequence. Starting a submission publishes its new idle attempt and cleared feedback before entering external validation. If validation exits abnormally, observers can still see that the previous attempt has ended; a later validation error publishes its own feedback separately.

Key sequences capture a `scene.KeyboardLease` through `Matcher.Handle`'s lifetime predicate. A receiver on the keyboard route keeps its lease while focus moves among its descendants. Explicitly forwarding input to another receiver, such as a completion popup, borrows the current origin's lifetime. With no focus, the active modal or root content supplies that origin. Leaving the scope, detaching or closing the root permanently revokes the old lease, including already queued resolvers; returning focus never revives it. Unmounted controls can accept fresh local input until their next attachment. Standalone matchers may pass nil when their caller owns lifetime directly.

The `SettingsConfig.Change` callback returns `scene.ActionResult[bool]`: `Value` says whether input was accepted, and `State` reports the domain owner's actual outcome. An accepted action at a value boundary returns `Value: true, State: scene.Unchanged`. Replace the callback through `Settings.SetChange`. Settings neither duplicates the value nor infers mutation from input consumption.

## Editor insertion positions

`base.Caret{Line, Byte}` addresses a byte offset in a logical line. `base.Point{Row, Col}` addresses terminal cells in a layout. The units remain explicit across wrapping and Unicode text. Vertical movement retains its desired cell column, including zero, while atomic elements snap to a boundary in the movement direction; layout queries do not reset that intent.

Multiline editors opt into `text.LayoutConfig.ReserveCaret`. A full logical line receives an empty continuation row for its final insertion position. Height, scrolling, vertical movement, hit testing and selection all consume that same immutable projection, including gutter and resize handling. Ordinary text layouts keep their existing passive row count unless they request this editing policy.

## Rich content behavior

`Transcript.AddDocument` connects a `StreamDocument` once. Changes, EOF preparation, partial-result policy, publication admission, retirement and subscription cleanup then follow the owning objects. A caller does not repeat a Feed/Changed/Finish/Commit protocol.

`Transcript.Close` releases its own committed geometry and presentation along with retained entries and subscriptions, even if no later frame is painted. A layout already retained by a caller remains immutable and keeps its own content lifetime.

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
