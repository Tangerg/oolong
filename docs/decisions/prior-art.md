---
title: Prior art and adopted decisions
description: Separate reusable architectural lessons from another framework's implementation shape.
contentType: Conceptual
---

# Prior art and adopted decisions

Language: English | [简体中文](../zh/decisions/prior-art.md)

## Review basis

This redesign was evaluated against the three requested terminal ecosystems and their primary sources. The source revisions below pin the comparison; no claim that one renderer is universally faster follows from the language or rendering style alone.

| Ecosystem | Reviewed primary sources |
| --- | --- |
| OpenTUI | [Pinned workspace](https://github.com/anomalyco/opentui/tree/ae15b990629510038339fcfb60b865106febad57), [interaction](https://opentui.com/docs/core-concepts/interaction/), [testing](https://opentui.com/docs/core-concepts/testing/) |
| Charm | [Bubble Tea](https://github.com/charmbracelet/bubbletea/tree/67b4c7a8a2b34a0b1ce15261b0c705bc7e1a04b7), [Bubbles](https://github.com/charmbracelet/bubbles/tree/e04077d178b7752a88ad0f687966fd10e6effd3a), [Lip Gloss](https://github.com/charmbracelet/lipgloss/tree/19f011f1cc573e5d2bf9d7a9c4f14d429647759e), [Huh](https://github.com/charmbracelet/huh/tree/ffb6a97195b4a031925a31cc70c62fc1d309e69d), [Glamour](https://github.com/charmbracelet/glamour/tree/010543cac1a07c9bfd9969e58d683f2ddef4c8f7), [Ultraviolet](https://github.com/charmbracelet/ultraviolet/tree/a45b7c86333f9dfedb95f81064e80259b02937f0), [Wish](https://github.com/charmbracelet/wish/blob/2af05484795152f53d9ad10b5be5748a4bec3c95/README.md) |
| Ratatui | [Workspace architecture](https://github.com/ratatui/ratatui/blob/cdcb0c01e4348d868a7bb0a6e392d91418e7a820/ARCHITECTURE.md), [stateful widgets](https://github.com/ratatui/ratatui/blob/cdcb0c01e4348d868a7bb0a6e392d91418e7a820/ratatui-core/src/widgets/stateful_widget.rs), [rendering](https://ratatui.rs/concepts/rendering/under-the-hood/), [snapshot tests](https://ratatui.rs/recipes/testing/snapshots/) |

## What Oolong adopts

OpenTUI is a useful reference for treating interaction, layout, lifecycle and testing as platform capabilities. Oolong applies that lesson to one scene mechanism and typed attachments without importing a JavaScript framework or assuming Yoga is required for terminal composition.

The Charm family demonstrates the value of a coherent runtime, reusable behavior, styling, forms, content rendering and host integration. Oolong keeps those responsibilities visible, puts tightly coupled UI layers in one Go module, and keeps heavy format/transport dependencies optional. It does not turn a historical comparison of renderer internals into a permanent competitive claim.

Ratatui is a useful reference for separating state from painting, treating cell geometry as an explicit contract, and verifying rendered behavior. Oolong preserves passive drawing and adversarial Unicode tests while using an owned scene for interactive identity, semantic actions and written-frame input projection.

## The additional UX, DX and AX decisions

[Base UI](https://base-ui.com/react/overview/about) and [shadcn/ui](https://ui.shadcn.com/docs) clarify the distinction between complete behavior and polished composition. Oolong expresses it as core primitives, base controllers and kit appearance. [Playwright locators](https://playwright.dev/docs/locators) inform the decision to retain query intent and resolve the current target at action time.

These influences become local contracts: a skin cannot duplicate state, observation cannot cause work, a stale target cannot act on its replacement, and full Markdown remains revisable until the whole source ends. Each decision is assessed by user behavior and executable evidence, rather than by copying an upstream API verbatim.
