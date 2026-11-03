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

The `use` directives in `go.work` define workspace membership. `scripts/sync-workspace.sh` derives versioned replacements from member requirements. Pass `--check` to verify those replacements. CI tests both the current workspace integration and each module with `GOWORK=off`, no local replacements and its declared published dependencies. The release gate repeats the independent check for the selected module.

## Independent versions

A release names one module and one semantic version, for example `ui/v0.1.0` or `extensions/markdown/v0.2.0`. Independent releases still require an honest `go.mod`: declare and review the lower dependency versions needed by the public API before releasing the higher module.

When development needs an API newer than the latest tag, push the lower module's implementation first and use `GOWORK=off go get <module>@<commit>` in each consumer that needs it. Go chooses the canonical pseudo-version and records its checksums. Then run `go mod tidy` with the workspace off and regenerate workspace replacements. A local replacement cannot substitute for a resolvable dependency requirement.

Do not keep the former `core`, `components`, or top-level extension import paths as adapters. This redesign intentionally requires source migration. The [changelog](https://github.com/Tangerg/oolong/blob/main/CHANGELOG.md) records the replacement contracts.
