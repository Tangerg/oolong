---
title: Architecture and ownership
description: Locate each advancing fact in one owner and keep every projection read-only.
contentType: Conceptual
---

# Architecture and ownership

Language: English | [简体中文](../zh/core/architecture.md)

## Physical layers are logical layers

The public module lives in `ui/`. The intended composition is `core → base → kit`, analogous to platform primitives, Base UI behavior, and a polished component system. Dependencies point downward: `kit` may compose `base`; `base` may use `core`; core packages do not import widgets. Optional renderers and hosts live in `extensions/` and consume lower contracts.

| Directory | Responsibility |
| --- | --- |
| `ui/core/scene` | Attachment, focus, event routing, frame projections, semantic observation and typed actions |
| `ui/core/text` | Safe text, display widths, logical positions and layout projections |
| `ui/core/program` | The UI sequence, scoped resources, transport composition and session lifetime |
| `ui/core/publication` | Immutable permanent-content admission, bounded pending work and receipts |
| `ui/base` | Reusable interaction and content policies over scene primitives |
| `ui/kit` | Appearance and useful compositions over base behavior |
| `ui/automation` | Owner-scheduled observation, location, invocation and conditional waiting |
| `ui/uitest` | In-process host tests and real-terminal conformance helpers |
| `extensions/` | Optional parser, renderer and host dependency boundaries |

The package graph is checked from Go import facts. Mentioning an upper-layer consumer in a comment does not introduce a code dependency. Documentation navigation may explain how a lower primitive is composed.

## One fact has one advancing owner

| Fact | Owner | Read-only or derived representations |
| --- | --- | --- |
| Actual attachment and mount generation | `scene.Root` and its scene nodes | `Element`, `Attachment`, `Target` handles |
| Keyboard focus and pointer capture | `scene.Root` | Semantic focus flags and event targets |
| Field value | The concrete base controller or explicit domain Control | Form output, semantic value, kit paint |
| Logical editable text and revision | `base.EditSession` | Editor layout, carets and selections mapped through changes |
| Stream source and finalization | `content.StreamDocument` | Immutable source snapshots and prepared results |
| Retained conversation references | `base.Transcript` | Transcript layout and reading anchors |
| Permanent-output responsibility | `publication.Publisher` | Immutable batches and concurrency-safe receipts |
| Worker outcome | `program.Task` | Task status; cancellation and delivery are separate fields |
| Written terminal progress | The frame writer | Publication and scene written-watermark projections |
| Application palette | `kit.ThemeOwner` | Immutable theme scopes and resolved instance palettes |

An accessor that reads application state and also keeps an independently writable local value is not a controlled component contract. Owned fields expose one controller and typed domain operations. Explicit controlled protocols such as `ConfirmControl` instead delegate Snapshot, revision-checked Change and Subscribe to the domain owner and do not retain a second writable answer. Skins receive their controller; they do not copy its value, selection, focus, or routing policy.

## Model, layout and write progress differ

An input or typed action advances the model. `Root.Prepare` stages geometry in immutable frame projections. The terminal encoder prepares bytes without advancing its diff baseline. Writer admission then commits the complete scene and output candidate. Physical completion advances the written watermark later.

Aborting a candidate preserves the previous complete geometry and terminal baseline, while leaving already accepted model changes intact. Pointer events use the geometry of the frame associated with their arrival, including the local projections saved in `scene.Snapshot`. A resize invalidates incompatible geometry; a detached target never identifies its replacement.

## Composition has one structural mechanism

`scene.Group` and `scene.Overlay` own actual child attachment. Base components choose behavior and assemble these primitives. Kit components fill appearance slots or compose more primitives around the same behavior parts. There is no independently mutable widget tree for focus and another one for automation.

`scene.Action[P, R]` owns availability, parameter validation and the invocation result. Keyboard, pointer and automation paths reuse that action or the same typed owner operation. Disabled, read-only, busy and modal constraints remain meaningful for semantic callers.

## Bounds describe concrete stages

Source bytes, prepared cells, key metadata, per-turn measurement work, accepted task resources, publication storage and encoded writer payloads are different budgets. Each is enforced by its owner. A bounded viewport alone does not imply bounded parsing, retained history or queued output.

Full Markdown preparation retains the entire bounded source because later references can change earlier meaning. `VirtualWindow` bounds materialization and its measured cache, while its key index and estimated row pass still depend on the configured dataset size. No end-to-end performance claim is inferred from cell diffing alone.

The [architecture diagram](/diagrams/oolong-architecture.html) is generated from the checked-in diagram source and follows these same owners.

## Further contracts

Read [text positions](text.md), [resource and publication lifetimes](lifetimes.md), [base behavior](../base/index.md), [kit appearance](../kit/index.md), and the [ownership decision](../decisions/ownership.md). Public API comments and executable tests carry the operation-level preconditions and failures.
