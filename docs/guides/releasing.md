---
title: Release one module
description: Review a single immutable module tag against independently resolvable dependencies.
contentType: How-to
---

# Release one module

Language: English | [简体中文](../zh/guides/releasing.md)

## Prepare the release commit

Choose the public module from `scripts/modules.sh --public`. Commit reviewed source, required dependency versions and a `CHANGELOG.md` section named `[module/vX.Y.Z]`. The release tool does not update dependency versions, manufacture commits or publish unrelated modules.

Use a clean `main` matching `origin/main`. Published tags are immutable. Semantic import suffixes and physical module paths must agree; the release tool derives the tag prefix from that relationship.

## Inspect the dry run

```sh
scripts/release.sh ui 0.1.0
scripts/release.sh extensions/markdown 0.2.0
```

Each command selects only its named module. The tool runs repository gates, verifies the selected module with `GOWORK=off`, and checks the proposed API version. Shared workspace success cannot conceal an undeclared dependency or a version that has never been published.

## Publish the reviewed tag

```sh
scripts/release.sh ui 0.1.0 --execute
```

Execution creates and pushes the one reviewed tag, then verifies its remote commit. Failure does not authorize tag deletion, movement, force-push or another release path. Inspect the actual state before deciding what to do next. See [CONTRIBUTING.md](https://github.com/Tangerg/oolong/blob/main/CONTRIBUTING.md) for exact tool pins and quality gates.
