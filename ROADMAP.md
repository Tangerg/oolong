# Oolong roadmap

This document separates implemented contracts from further product choices. [DESIGN.md](DESIGN.md) describes the current architecture. A capability is complete only when its callers, resource limits, failure behavior and regression evidence agree.

## Implemented foundation

| Capability | Current boundary and evidence |
| --- | --- |
| Layered physical and logical structure | `ui/core`, `ui/base`, `ui/kit`, `ui/automation`, `ui/uitest` and `extensions`; import and module checks under `ui/internal/arch` |
| One interaction owner | Scene attachment, focus, capture, modal eligibility, typed actions and bounded semantic observations; scene and automation contract tests |
| Atomic presentation | Immutable scene candidates, publication candidates and terminal output transactions; aborted frames preserve the prior projection |
| Shared text geometry | Logical positions, Unicode graphemes and cell layout reused by editing and transcript selection |
| Owned editing | EditSession revision, history and change maps; base and kit editor contract tests |
| Scoped external work | Explicit scope lifetime, cancellation, owner application and terminal settlement |
| Chunk-independent Markdown | Whole-source preview preparation, explicit EOF and final partition-equivalence regression and fuzz coverage |
| Bounded publication | Frozen capture, explicit admission, deferred progress, receipts, physical writer completion and partial-write outcomes |
| Bounded recent reading | Transcript retention, KeepCompleted, logical reading anchors, selection and sticky projection from one layout |
| Virtual materialization | Bounded keys and measurements, visible element materialization, explicit estimated versus exact extent |
| Shared appearance | One ThemeOwner with immutable scope and instance resolution on existing elements |
| Independent module release | Derived inventory, explicit module selection and independently consumable declared dependencies |

## Next capabilities require concrete product contracts

### Persistent conversation archives

An archive needs one authoritative record schema, stable content identities, retention and deletion policy, storage failure semantics, and loading budgets. It must not become another mutable source behind StreamDocument or Transcript. Acceptance requires restart and migration tests, a measured long-session workload, and explicit behavior when older content cannot be loaded.

### Rich-format source mapping

Logical copy text already has stable positions within a content revision. Mapping formatted Markdown, code blocks and diagrams back to source syntax requires renderer-owned ranges and revision translation. Acceptance requires tests for nested formatting, hidden syntax, forward references and partial preparation, rather than estimating source offsets from terminal rows.

### Indexed large-history navigation

VirtualWindow currently bounds work and memory but scans its retained key metadata and prefix estimates. A tree or indexed external source should be introduced only with a workload that demonstrates the need. Acceptance requires measured random seek and resize costs, stable anchors during estimate refinement, bounded loading and clear incomplete-result semantics for search and automation.

### Optional platform capabilities

Clipboard, notifications, graphics and host services remain explicit optional capabilities. A new adapter must define unsupported behavior, cancellation and teardown. Acceptance includes real-host or protocol tests and no additional dependency in lower layers that do not use it.

## Continuing quality work

Improve regression and benchmark corpora when a real failure or performance limit identifies a gap. Add data to the existing owner or corpus instead of a competing cache, parser or control route. Keep public contract examples executable and translations paired. Preserve historical reports as evidence of their recorded revisions.

## Deliberate boundaries

No compatibility shim is planned for the removed routing, controlled-accessor, streaming-prefix or coordinated-release APIs. No default background archive is created, no observation silently mounts hidden content, and no source-size overflow changes the language mode. Higher-level policies can be added by composing the existing owners when their product requirements are clear.
