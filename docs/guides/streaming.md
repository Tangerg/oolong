---
title: Stream bounded content
description: Keep provisional Markdown revisable and compose completion with publication once.
contentType: How-to
---

# Stream bounded content

Language: English | [简体中文](../zh/guides/streaming.md)

## Follow the complete vertical slice

Run `go run ./examples/streaming` and read the tested [streaming example](https://github.com/Tangerg/oolong/tree/main/examples/streaming). Construct `program.NewInline`, create a Transcript with `Publisher: runtime.Publisher()` and `Retention: base.PublishCompleted`, and attach its stable element.

Create a `content.StreamDocument` with an explicit format, then call `Transcript.AddDocument` once. Owner-side ingestion calls `Append`; source completion calls `End(cause)`. Schedule `document.Snapshot().Prepare(ctx, renderer)` explicitly, and return its typed result through `Runtime.Start` to `document.Accept`.

## EOF is a semantic boundary

Transport chunks are arbitrary byte partitions. A chunk can split UTF-8, a fence, a list, a raw HTML block or a forward reference. Full Markdown is therefore provisional until source EOF and final preparation. The final rendered cells, links and styles must match rendering the whole source, independent of chunk sizes.

`End` advances revision even when no new bytes arrive. Accepting an earlier preview cannot finalize the document. `ErrStalePreparation` means the result is obsolete and should be discarded; it is not a reason to install it through another path. Coalescing preparation to the newest snapshot is a scheduling choice, not a change in document semantics.

## Retention composes admission

Transcript subscribes to document changes and the publisher's deferred progress notifications. It publishes only a leading complete run, preserving order. `KeepCompleted` retains the newest configured number within that run; zero publishes every eligible item. The configured recent count must be smaller than `MaxItems`, leaving room for another source. On acceptance it releases body and source subscriptions and retains the receipt. Capacity refusal keeps the reference live and resumes when capacity changes. A partial result is retained unless `PublishPartial` explicitly permits publication.

Use the receipt to distinguish responsibility transfer from physical writing. Do not append a compensating duplicate after a writer failure or `UnknownPartial`; accepted bytes may already be present. Applications do not manually call Changed, Finish, Commit, or a publication-resume callback.

## Bound each stage

| Stage | Owner and policy |
| --- | --- |
| Incoming ordered bytes | `ByteIngress` has its own capacity and cooperative writer backpressure |
| Retained source | `StreamConfig.MaxSourceBytes`; default 4 MiB, atomic refusal on overflow |
| Preview preparation | Complete immutable source snapshot; application schedules/coalesces scoped work |
| Retained active references | `TranscriptConfig.MaxItems`; default 64, no silent eviction |
| Permanent capture | Publisher cell and item-byte budgets; native paint refused |
| Pending plus inflight responsibility | Publisher item and total-byte budget, released by writer completion |
| Encoded terminal frames | Exact encoded-payload admission at the runtime/writer boundary |

A source-limit error does not silently switch Markdown into plain text or declare a syntactically incomplete prefix final. An application may report the limit, end with an explicit partial cause, or choose an explicitly different product representation.

## Preserve reading intent

The default transcript follows new content. Manual scrolling keeps a logical reading anchor across reflow when the projected text remains equivalent. Selection is based on content identity and logical positions, not physical row numbers. Sticky headers, selection and pointer mapping share the same candidate layout.
