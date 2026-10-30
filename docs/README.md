---
title: Oolong documentation
description: Terminal interfaces composed from one set of owners.
contentType: Landing
---

# Oolong documentation

Language: English | [简体中文](/zh/)

## Learn by building

| Step | Guide | Runnable result |
| --- | --- | --- |
| 1 | [Your first interface](guides/getting-started.md) | One scene and one terminal session |
| 2 | [Compose components](guides/components.md) | Shared behavior with replaceable appearance |
| 3 | [Render rich content](guides/content.md) | Optional Markdown, code and mathematics |
| 4 | [Stream bounded content](guides/streaming.md) | Revisable previews and explicit permanent publication |
| 5 | [Build an agent interface](guides/agent.md) | Human and semantic actions over the same scene |

[Browse runnable examples](guides/examples.md). Every command has an in-process test.

## Read by layer

- [Core architecture](core/architecture.md): identity, source, layout, actions, resources and writer progress
- [Logical text](core/text.md): text positions and cell projections
- [Lifetimes](core/lifetimes.md): cancellation, delivery, publication and physical writing
- [Base behavior](base/index.md): reusable controller policy
- [Kit appearance](kit/index.md): themes and component composition
- [History and virtualization](guides/history.md): active content, bounded windows and application archives

## Validate and maintain

[Testing](guides/testing.md), [troubleshooting](guides/troubleshooting.md), [modules](guides/modules.md), and [independent releases](guides/releasing.md) cover the operational contracts. [Ownership decisions](decisions/ownership.md), [prior art](decisions/prior-art.md) and [brand](decisions/brand.md) record the reasoning and language behind them.

The public module is `github.com/Tangerg/oolong/ui`; optional dependency boundaries live in `extensions/`. Package contracts and executable Go examples are the API reference. Historical reports under `docs/report` preserve the versions they analyzed.
