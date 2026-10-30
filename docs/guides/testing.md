---
title: Test ownership and output
description: Test model transitions, semantic actions and terminal behavior at their owning boundaries.
contentType: How-to
---

# Test ownership and output

Language: English | [简体中文](../zh/guides/testing.md)

## Choose the boundary

Use direct controller tests for value and interaction policy. Use `scene.Root` candidates for attachment, focus, semantic actions, geometry and abort behavior. Use `ui/automation` for the same queries and actions a remote agent uses. Use `uitest.New` as a `program.Config.Host` to run sessions in process, and `uitest/ptytest` for terminal-screen or real-PTY conformance.

A screenshot checks appearance, but it does not prove that the same action is available to a keyboard or an agent. Pair visual assertions with action results, target lifetime, source revision or writer receipts when those facts matter.

## Exercise transactions and failures

Prepare a frame, inspect that committed geometry has not advanced, then abort. Verify model changes survive while geometry remains at the previous complete frame. Admit another candidate and report physical writing separately. A queued frame is not a written frame.

Inject capacity refusal, worker completion after detach, invalid action parameters, partial writes and native-paint refusal. Stream tests compare final cells against whole-source rendering across arbitrary chunk boundaries, including forward references and split UTF-8.

## Run repository gates

From the checkout, `go -C ui test ./...` covers the public module and its internal architecture tools. `scripts/modules.sh` derives all workspace modules; run each module's tests and `go test -race` when validating a cross-module change. `scripts/sync-workspace.sh --check` checks the derived workspace without editing it.

Documentation tests compile the complete bilingual getting-started program, verify paired pages and validate repository links. `npm run docs:lint` checks Markdown, and `npm run docs:build` checks the published documentation graph. Follow [CONTRIBUTING.md](https://github.com/Tangerg/oolong/blob/main/CONTRIBUTING.md) for the complete pinned release gate.
