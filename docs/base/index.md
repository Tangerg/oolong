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

## Rich content behavior

`Transcript.AddDocument` connects a `StreamDocument` once. Changes, EOF preparation, partial-result policy, publication admission, retirement and subscription cleanup then follow the owning objects. A caller does not repeat a Feed/Changed/Finish/Commit protocol.

`Selection` stores content-relative logical endpoints. `Scroll` owns reading intent. `Sticky` owns content identities eligible for pinning. Transcript's frame candidate composes their projections; painting does not advance their state. `Scan` is a bounded synchronous search operation that callers may schedule as scoped work and accept only for the matching content revision.

## Large datasets

`VirtualWindow.Resolve` runs explicitly on the owner before painting. A `VirtualSource` supplies dataset revision, stable keys and pure height measurements. The window retains only materialized elements and bounded measurements, reusing an element while its key remains present. Leaving the window detaches its target; reappearing later receives a new mount lifetime.

Estimated total height remains marked inexact until all relevant heights are known. The metadata index and prefix pass are bounded by `MaxItems` and `MaxKeyBytes`; measurement work is bounded by `MaxMeasurements`. See [history and virtualization](../guides/history.md) for the separate ownership of archived content.
