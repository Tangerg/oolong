---
title: Resources and publication lifetimes
description: Separate execution, delivery, admission and physical terminal completion.
contentType: Reference
---

# Resources and publication lifetimes

Language: English | [简体中文](../zh/core/lifetimes.md)

## Scoped work

The UI owner creates `Scope` with `Runtime.NewScope`. `ScopeConfig.Attachment` can tie delivery eligibility to one mount generation; `Context` adds an external lifetime. `Runtime.Start(scope, Work[T]{Run, Apply})` reserves completion capacity before launching `Run`. The worker returns a typed result, and `Apply` runs on the UI owner only while the scope remains eligible.

Leave `Attachment` nil for a session-bound scope. To bind work to a mounted element, obtain `lease := element.Attachment()` and pass `ScopeConfig{Attachment: &lease}`. Construction copies the lease: replacing the caller's variable cannot rebind accepted work. An explicitly supplied empty, detached, or foreign lease returns `ErrScopeClosed`; it never falls back to session lifetime. Detaching and remounting an element creates a new lease and cannot revive an old scope.

`TaskStatus` separates the worker outcome, `CancelRequested`, and result delivery. Calling `Cancel` requests cooperation; it cannot truthfully declare external work finished. A successful worker can have revoked UI delivery after its view is detached. `Task.Done` and `Task.Settled` expose those distinct boundaries. Ordinary dispatcher congestion does not discard an already accepted task's completion.

## Admitted frames and pointer input

A custom compositor calls `Root.Wrote(sequence)` when its writer advances. If several admissions are waiting, the watermark selects the newest covered layout; later admissions remain pending. A single acknowledgement does not assign fictional times to skipped frames. `RootConfig.MaxPendingFrames` bounds this queue independently of timestamp history (default eight). When `Prepare` returns `scene.ErrBusy` at capacity, wait for an acknowledgement before preparing again. Resizing or invalidating geometry clears the old coordinate authority, including semantic bounds.

Pointer input names a mount, content ID, revision and coordinate frame. A callback that changes any of these cannot send the same click to replacement content. Modal shortcuts run inside the active modal. The optional `DialogConfig.Dismissed` callback lets application work follow the actual close, including when an agent invokes it.

## Moving a live element

An attachment's `Done` closes at detach. Its old blur, capture cancellation or detach callback may still be queued or executing. If mounting the same element returns `scene.ErrCleanupPending`, let the old callback return and flush the old root on its owner before retrying. Unrelated elements can mount immediately. An interrupted flush keeps the remaining notifications in order; reentering a flush does not overtake them. Closing a root wakes current and subsequent change subscribers.

A routed event retains its input for inspection. Its imperative focus and capture methods expire with the callback or a newer input turn. Save the element or a generation-checked target for a later command.

## Permanent publication

`Publisher.TryPublish` captures a complete drawable into immutable owned cells before accepting responsibility. Native painters are refused because their lifetime requires live frame ownership. Capture errors, unsupported output and capacity refusal leave responsibility with the caller. Empty captured content receives a written no-op receipt with sequence zero.

| Receipt state | Meaning |
| --- | --- |
| `Accepted` | The publisher owns immutable content; writer admission may still be pending |
| `Written` | The physical writer watermark covers its sequence, or the contribution was empty |
| `Failed` | The responsibility ended with a known failure |
| `UnknownPartial` | Writer-admitted bytes may have partially reached the destination |

A publication ID identifies accepted responsibility and is never substituted for a transport sequence. Pending and writer-admitted captures share the same item and byte budget; passing a slow queue does not release that budget prematurely.

## Read-only composition borrows

`Element.ReadOnly` runs a synchronous projection while the composition's phase owner refuses model and attachment changes. A mounted composition uses its scene root; an unmounted composition uses its outermost group or element. Mounting or moving the composition during the borrow is refused, so the callback cannot escape to another owner. Nested borrows restore the enclosing phase, including after a panic or goroutine exit.

Transcript uses this primitive around publication capture. Its scroll, selection and sticky controllers already share the same Element, so their commands consult the same phase owner without separate capture flags. `Element.Measure` uses the same boundary. This is an owner-sequence contract, not permission to invoke UI operations concurrently from workers.

A controller's drawing and staging use the frame of the root that mounts its actual Element. Custom controls call `Element.CheckProjection(frame)` before deriving candidate geometry or invoking render callbacks. Mount that Element and fill its paint slot before mounting; an unrelated wrapper cannot become a second geometry producer. Detached controllers still support model operations and measurement, but cannot stage committed geometry. Standalone primitives with no Element follow the phase of their frame.

Read-only borrows also refuse snapshot staging and refinement. A pending candidate must commit or abort before a new read-only borrow or measurement can begin. This prevents a saved candidate from changing the observed geometry inside a callback while preserving Commit's ability to settle already-admitted output without another rejection point.

`ScrollLayout` reads and refines that one staged candidate within the producing paint callback. It holds no independently advancing offset copy. A refused refinement leaves both the candidate and its observable offset unchanged; after preparation, read the current controller position through `Scroll.Offset`.

## Atomic output and shutdown

`Publisher.Prepare` reads an immutable pending prefix. `grid.Inline.Prepare` composes that prefix with the live frame without advancing the anchor. The runtime checks the complete encoded payload and writer capacity, then commits publication, output and scene candidates without running application callbacks between them. Refusal aborts the candidate and keeps the previous complete projection authoritative.

`CloseAdmission` prevents new publication while preserving accepted content for the final frame and drain. `Written` settles actual progress. Terminal failure settles remaining responsibility with its real cause. Deferred `FlushNotifications` runs after all owners settle, allowing Transcript to resume capacity-blocked publication in a later owner turn.

Inline shutdown drains the final accepted frame before admitting cursor cleanup, then waits for cleanup itself. A one-frame writer budget therefore preserves output order and cursor restoration without relying on a background writer winning a scheduling race.

## Independent resource budgets

StreamDocument bounds retained source bytes. Capture bounds cells and owned strings. Publisher bounds complete retained captures. The runtime bounds action and resource admission; the writer bounds encoded bytes and pending transport. These quantities are deliberately not added into a fictitious universal memory number, and caller allocations inside a custom renderer remain the caller's responsibility.

An application obtains a producer with `runtime.Publisher()`. Its `TryPublish` checks the live session and writer on every admission, including through a saved handle. A successful admission schedules a frame. Only the compositor can set geometry, admit a writer sequence, or settle receipts. For a custom compositor, construct and own a separate `publication.Publisher`.

## Callback authority

A scene grants cells and staged geometry to one paint callback. A copied frame, its child views, its plain grid view and its staged handles all expire when that callback returns. Nested read-only work cannot write through them. The compositor still owns the underlying destination; rendering another sibling does not renew an old painter's access. A custom layout may reuse its rectangle slice after returning.

Opaque terminal painters receive `grid.PaintContext` in both `Paint` and `Erase`. It implements `io.Writer` and supplies immutable cell dimensions and a native layer. Copies share one callback lease; subsequent writes return `io.ErrClosedPipe`. The compositor closes hyperlinks and resets SGR before and after each callback because opaque output cannot update its cell encoder's rendition cache. Native image layers follow composition order below text cells, independently of upload IDs. A low-level Kitty placement must receive this layer explicitly rather than assign its own order.

Pointer commands check the original hit as well as the node receiving preview or bubbling. A claimed press stops propagation. Drag and release require a recorded arrival map for the same captured content; losing that evidence cancels the gesture. Cleanup notifications cannot claim a new press. A modal with no focusable child can receive keys itself without creating a fictional focus target. For non-pointer input, `Event.Read` reads a staged projection from this scene's current layout; an absent producer is not a producer that staged nil.

## Handing a terminal or image to another consumer

The terminal owner pauses ordinary frame admission before draining pending output and handing off the display. A retained frame sink cannot switch to the control queue or close the writer. Its ordinary writes resume only after a successful return of the terminal. Setup queries the environment before acquiring raw mode.

`Session.Suspend` uses this handover before invoking `term.Suspend`. On Linux, Darwin and FreeBSD, the stop request reaches a synchronous operating-system boundary before the call returns. Default job control returns after continuation; ignored signals, host handlers and orphaned process groups may leave the process running. A blocked stop signal or an unsupported platform returns `errors.ErrUnsupported` without queuing a future stop. The library neither replaces the host's signal disposition nor waits for a continuation that may never occur.

`Images.Transmit(ctx, png)` returns an admission error or a buffered, single-result `<-chan graphics.UploadResult`. Only the terminal's successful creation acknowledgement supplies a `Resource`. Receive the result in `Work.Run`, apply placement on the UI owner, and retain cleanup responsibility if scope revocation prevents delivery; the image and Mermaid examples demonstrate this ownership transfer. The request context ends unfinished work only. A returned resource remains live until release, terminal handover or terminal close. Cancelled requests retain bounded correlation until a late allocated image can be deleted, including retry after output capacity becomes available.

A host that supports native images binds each acknowledged transmission through its own `graphics.ImageOwner` and reports that owner's `NativeTarget`. Resource handles carry the issuing destination and transmission lifetime. Stop drawing a resource before releasing it through the same host. Placing a live resource on another terminal is refused even when the two local wire IDs match. The low-level `graphics.Image` encoder still describes explicit protocol operations; its wire ID is not a resource receipt.
