---
title: Choose modules and dependencies
description: Keep the UI layers together and optional dependency boundaries separate.
contentType: Reference
---

# Choose modules and dependencies

Language: English | [简体中文](../zh/guides/modules.md)

## Public distribution

`github.com/Tangerg/oolong/ui` contains `core`, `base`, `kit`, `automation` and `uitest`. Package boundaries express architecture without forcing every behavior and appearance change through multiple synchronized module versions. Go compiles imported packages; a UI module dependency does not require an application to import every widget.

Optional modules live at `github.com/Tangerg/oolong/extensions/<name>`. Parser, renderer and transport dependencies justify those module boundaries. Source imports and physical directories use the same hierarchy.

## Derive the workspace inventory

Run `scripts/modules.sh` for all workspace modules and `scripts/modules.sh --public` for publishable modules. The tools derive membership from the repository and Go package facts. This page intentionally does not maintain another dependency count or version catalog.

`go.work` supports local composition. `scripts/sync-workspace.sh` derives its module entries; `--check` verifies them. Tests with a workspace do not prove a tagged module is independently resolvable. The release gate also tests the selected module with `GOWORK=off` and its declared published dependencies.

## Independent versions

A release names one module and one semantic version, for example `ui/v0.1.0` or `extensions/markdown/v0.2.0`. Independent releases still require an honest `go.mod`: declare and review the lower dependency versions needed by the public API before releasing the higher module.

Do not keep the former `core`, `components`, or top-level extension import paths as adapters. This redesign intentionally requires source migration. The [changelog](https://github.com/Tangerg/oolong/blob/main/CHANGELOG.md) records the replacement contracts.
