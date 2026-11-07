---
title: Stream bounded content
description: Keep provisional Markdown revisable and compose completion with publication once.
contentType: How-to
---

# Stream bounded content

Language: English | [简体中文](../zh/guides/streaming.md)

## Follow the complete vertical slice

Run `go run ./examples/streaming` and read the tested [streaming example](https://github.com/Tangerg/oolong/tree/main/examples/streaming). Construct `program.NewInline`, compose its publisher with the example's preparation queue, create a Transcript with `Retention: base.PublishCompleted`, and attach its stable element. The queue's publisher adapter refuses a cold width before permanent capture and resumes retention when preparation settles.

Create a `content.StreamDocument` with an explicit format, then call `Transcript.AddDocument` once. Owner-side ingestion calls `Append`; source completion calls `End(cause)`. Schedule `document.Snapshot().Prepare(ctx, renderer)` explicitly, and return its typed result through `Runtime.Start` to `document.Accept`.

The shared `examples/internal/docprep` composition admits one executing preparation and retains only the newest pending snapshot for each document. `Task.Cancel` requests cancellation; it does not prove an entered parser has exited. The next preparation starts only when the preceding `Run` actually returns and its owner-side completion is delivered. Closing a scope revokes delivery, so an application must not close and replace scopes on every source chunk as a substitute for execution admission.

## Prepare the target width before painting

`markdown.NewRenderer` returns a passive `markdown.Snapshot`. Parsing alone does not complete width-dependent work. On the worker, call `Snapshot.PrepareWidth(ctx, width)` to prepare wrapping and text rows. `PreparedLayout(width)` reads an already prepared immutable projection without starting work, allowing permanent capture to pin that projection even when the bounded width cache is later replaced.

Install one `Runtime.SetFramePreparation` check with signature `func(width, height int) (bool, error)`. It reads the compositor-supplied size and checks the transcript's retained bodies. A changed width schedules background preparation and returns `(false, nil)`; task completion or `Refresh` wakes the owner to try again. Return `(true, nil)` when ready, or an error to terminate `Run` after an unrecoverable preparation failure. The check must not block. It controls readiness only: scene still owns candidate geometry and atomic frame admission, and `StreamDocument` still owns source revisions and EOF. A stale-width result can retain its parsed syntax while the queue prepares the requested width; it does not need another source revision or another parse.

An ordered repaint waits for readiness and actual writing before acknowledging success. Input EOF settles a waiting repaint with `program.ErrStopped` and zero dimensions, then drains the already admitted input prefix before returning the source result. Shutdown does not wait for a worker whose delivery has been revoked. If preparation is pending and the view has pending permanent captures, those captures replace the old live block in FIFO order and transient live UI is discarded. This prevents a retired item in the old view from being printed a second time. Without pending captures, shutdown finishes the previously admitted live block.

## EOF is a semantic boundary

Transport chunks are arbitrary byte partitions. A chunk can split UTF-8, a fence, a list, a raw HTML block or a forward reference. Full Markdown is therefore provisional until source EOF and final preparation. The final rendered cells, links and styles must match rendering the whole source, independent of chunk sizes.

`End` advances revision even when no new bytes arrive. Accepting an earlier preview cannot finalize the document. `ErrStalePreparation` means the result is obsolete and should be discarded; it is not a reason to install it through another path. Coalescing preparation to the newest snapshot is a scheduling choice, not a change in document semantics.

## Retention composes admission

Transcript subscribes to document changes and the publisher's deferred progress notifications. It publishes only a leading complete run, preserving order. `KeepCompleted` retains the newest configured number within that run; zero publishes every eligible item. The configured recent count must be smaller than `MaxItems`, leaving room for another source. On acceptance it releases body and source subscriptions and retains the receipt. Capacity refusal keeps the reference live and resumes when capacity changes. Partial and failed results are retained unless `PublishPartial` explicitly permits publication. A failed preparation with no body then receives an empty, immediately written capture in the same order; it emits no bytes and keeps its original error, so later diagnostics are not stranded behind it.

Use the receipt to distinguish responsibility transfer from physical writing. Do not append a compensating duplicate after a writer failure or `UnknownPartial`; accepted bytes may already be present. Applications do not manually call Changed, Finish, Commit, or a publication-resume callback.

## Bound each stage

| Stage | Owner and policy |
| --- | --- |
| Incoming ordered bytes | `ByteIngress` has its own capacity and cooperative writer backpressure |
| Retained source | `StreamConfig.MaxSourceBytes`; default 4 MiB, atomic refusal on overflow |
| Preview preparation | One actual worker, latest pending snapshot per document, explicit finite admission budget |
| Width layout and text projection | One derived Markdown width cache; frame readiness and immutable capture consume it |
| Retained active references | `TranscriptConfig.MaxItems`; default 64, no silent eviction |
| Permanent capture | Publisher cell and item-byte budgets; native paint refused |
| Pending plus inflight responsibility | Publisher item and total-byte budget, released by writer completion |
| Encoded terminal frames | Exact encoded-payload admission at the runtime/writer boundary |

A source-limit error does not silently switch Markdown into plain text or declare a syntactically incomplete prefix final. An application may report the limit, end with an explicit partial cause, or choose an explicitly different product representation.

## Preserve reading intent

The default transcript follows new content. Manual scrolling keeps a logical reading anchor across reflow when the projected text remains equivalent. Selection is based on content identity and logical positions, not physical row numbers. Sticky headers, selection and pointer mapping share the same candidate layout.
