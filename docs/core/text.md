---
title: Logical text and cell projections
description: Keep text identity stable when width, wrapping and physical rows change.
contentType: Reference
---

# Logical text and cell projections

Language: English | [简体中文](../zh/core/text.md)

## Positions belong to a document revision

`text.Position` identifies a logical line, UTF-8 byte offset and affinity. It is meaningful within one document revision. `text.Layout` owns normalization, cell hit testing, carets, rectangles and rendered rows. Its input is printable logical text; these byte offsets do not claim to be offsets into original Markdown or terminal escape sequences.

A cell is a display atom projection. Its span is owned by the grid and can exceed two columns. Clipping, overwrite, selection decoration and encoding preserve atom boundaries. A renderer must not infer width from byte length or treat a trailing atom cell as an independent character.

## Editors and transcripts compose the mapping

`base.EditSession` owns editable text changes and revision maps. Multiple editors can view that document without becoming additional text owners. Selection and caret positions are mapped through edits by the document contract; width changes only rebuild the visual projection.

Transcript endpoints additionally carry `BlockID`, prepared-content revision and a logical-projection fingerprint. Soft-wrap metadata from `text.Row.Joined` and `Gap` reconstructs copyable logical text. A width-dependent renderer that changes the actual copy text invalidates old selection coordinates with `ErrStaleSelection`; the library does not invent a source map it does not possess.

## Reading and copying

`Transcript.Layout().Hit` creates logical endpoints from displayed cells. `Selection.Text(layout)` reads printable text from that projection, and `Selection.Rects(layout)` projects highlights back to cells. Both refuse unavailable selected revisions. Unknown non-text content keeps physical row slots but supplies no fabricated copy text.

A reading anchor is a projection of content identity, logical position, viewport offset and the scroll owner's follow-end intent. Reflow derives a new offset from it; the anchor does not independently advance scroll state. A sticky header and its body use the same candidate content map.
