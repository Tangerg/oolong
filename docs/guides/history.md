---
title: Retain, publish and virtualize history
description: Choose distinct owners for live content, terminal output and durable archives.
contentType: How-to
---

# Retain, publish and virtualize history

Language: English | [简体中文](../zh/guides/history.md)

## Choose the history lifetime

Retained interactive history belongs to Transcript and consumes its active-item budget. Permanent terminal output belongs to Publisher after accepted immutable capture. Durable searchable history belongs to application storage. Publishing to terminal scrollback does not create a random-access archive or guarantee terminal-side copy and search behavior.

Keep a `TranscriptItem` when the application needs its ID and receipt. Retirement clears its body, document and subscriptions. A detached scene target cannot be reused as an address for archived content; use an application content ID and an explicit archive query instead.

## Materialize a bounded window

Use `base.VirtualWindow` for a dataset too large to mount eagerly. Implement `VirtualSource.Revision`, `Len`, `Key` and pure `Measure`. Keys represent stable item identity, independent of indices. Call `Resolve(width, height, offset)` before scene preparation; `Build(key)` creates an actual element only for materialized items.

Defaults bound the dataset to 100,000 keys and 4 MiB of key bytes, and each resolution to 512 measurements. Only the current window retains measured heights. These are adjustable contracts with explicit refusal, not a promise that all datasets fit. Dataset-key indexing and row-prefix estimation remain linear in the bounded dataset size.

## Preserve identity and honest geometry

The window reuses the same element while a key remains materialized. Removal detaches its target and revokes attachment-bound work. A later instance of the same content key has a new attachment lifetime. Invalid keys, budget failures, negative heights and mid-resolution source changes preserve the previous window.

`VirtualRange.Exact` reports whether total height is measured rather than estimated. Estimated geometry cannot be reported as an exact scrollbar or an authoritative archive offset. Logical reading positions should be based on content identity and text positions where the renderer can provide them.

## Keep archive capabilities explicit

The current library does not supply a persistent archive database, a universal format-source map, or automatic offscreen materialization from an automation query. Applications can compose storage and virtual-window operations as explicit actions. Observation must distinguish unavailable, virtualized and retired content instead of inventing live targets for it.
