---
title: "Decision: one advancing owner"
description: Record the ownership decisions behind the breaking redesign.
contentType: Conceptual
---

# Decision: one advancing owner

Language: English | [简体中文](../zh/decisions/ownership.md)

## Decision and premises

Accepted for the 2026-10-30 redesign. `AGENTS.md` requires one fact, one owner and composition over inheritance. Earlier API shapes are not compatibility constraints. A structural or lifecycle defect is repaired at the owner rather than arbitrated by synchronization rules.

## Consequences

Scene is the actual attachment, focus, event and semantic mechanism. Base controllers own value and interaction facts. Kit fills appearance slots and composes real parts. Typed actions unify availability and execution. Scoped resources separate cancellation, worker outcome and delivery. StreamDocument owns source revisions; Publisher owns accepted permanent output.

The old component adapters, independently writable accessor-backed field values, incremental Markdown publication-prefix protocol and inline open-row queue are removed. Their useful guarantees survive as new owner contracts and regression tests; their old public shapes do not survive as compatibility wrappers.

## Documentation and package direction

Actual Go imports define the architectural dependency graph. Comments can mention upper-layer consumers, and documentation can navigate to compositions. A comment mentioning `kit` is not a dependency from core to kit. Gates continue to enforce real import direction, pure rendering, owner lifetimes and API migration evidence.

Physical documentation follows `core`, `base`, `kit`, `guides` and `decisions`; Chinese pages mirror the same structure. Historical reports preserve their original API descriptions and link to the revision they described. They are evidence of prior work, not normative current contracts.

## Verification and limits

Observe a fact through its public projection and attempt to violate its boundary: mutate during paint, reuse a detached target, apply stale preparation, admit oversized output, abort a frame, or observe before physical writing. Tests must preserve those failures across refactoring. Performance remains a measured property of a stated workload.
