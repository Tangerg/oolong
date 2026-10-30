---
title: Resources and publication lifetimes
description: Separate execution, delivery, admission and physical terminal completion.
contentType: Reference
---

# Resources and publication lifetimes

Language: English | [简体中文](../zh/core/lifetimes.md)

## Scoped work

The UI owner creates `Scope` with `Runtime.NewScope`. `ScopeConfig.Attachment` can tie delivery eligibility to one mount generation; `Context` adds an external lifetime. `Runtime.Start(scope, Work[T]{Run, Apply})` reserves completion capacity before launching `Run`. The worker returns a typed result, and `Apply` runs on the UI owner only while the scope remains eligible.

`TaskStatus` separates the worker outcome, `CancelRequested`, and result delivery. Calling `Cancel` requests cooperation; it cannot truthfully declare external work finished. A successful worker can have revoked UI delivery after its view is detached. `Task.Done` and `Task.Settled` expose those distinct boundaries. Ordinary dispatcher congestion does not discard an already accepted task's completion.

## Permanent publication

`Publisher.TryPublish` captures a complete drawable into immutable owned cells before accepting responsibility. Native painters are refused because their lifetime requires live frame ownership. Capture errors, unsupported output and capacity refusal leave responsibility with the caller. Empty captured content receives a written no-op receipt with sequence zero.

| Receipt state | Meaning |
| --- | --- |
| `Accepted` | The publisher owns immutable content; writer admission may still be pending |
| `Written` | The physical writer watermark covers its sequence, or the contribution was empty |
| `Failed` | The responsibility ended with a known failure |
| `UnknownPartial` | Writer-admitted bytes may have partially reached the destination |

A publication ID identifies accepted responsibility and is never substituted for a transport sequence. Pending and writer-admitted captures share the same item and byte budget; passing a slow queue does not release that budget prematurely.

## Atomic output and shutdown

`Publisher.Prepare` reads an immutable pending prefix. `grid.Inline.Prepare` composes that prefix with the live frame without advancing the anchor. The runtime checks the complete encoded payload and writer capacity, then commits publication, output and scene candidates without running application callbacks between them. Refusal aborts the candidate and keeps the previous complete projection authoritative.

`CloseAdmission` prevents new publication while preserving accepted content for the final frame and drain. `Written` settles actual progress. Terminal failure settles remaining responsibility with its real cause. Deferred `FlushNotifications` runs after all owners settle, allowing Transcript to resume capacity-blocked publication in a later owner turn.

## Independent resource budgets

StreamDocument bounds retained source bytes. Capture bounds cells and owned strings. Publisher bounds complete retained captures. The runtime bounds action and resource admission; the writer bounds encoded bytes and pending transport. These quantities are deliberately not added into a fictitious universal memory number, and caller allocations inside a custom renderer remain the caller's responsibility.
