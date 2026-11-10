---
title: Test ownership and output
description: Test model transitions, semantic actions and terminal behavior at their owning boundaries.
contentType: How-to
---

# Test ownership and output

Language: English | [简体中文](../zh/guides/testing.md)

## Choose the boundary

Use direct controller tests for value and interaction policy. Use `scene.Root` candidates for attachment, focus, semantic actions, geometry and abort behavior. Use `ui/automation` for the same queries and actions a remote agent uses. Use `uitest.New` as the host in `program.ScreenConfig` or `program.InlineConfig` to run sessions in process, and `uitest/ptytest` for terminal-screen or real-PTY conformance.

`ptytest.Screen` models relative cursor movement with the active scroll margins. LF, VT and FF share line-feed behavior; NUL, BEL and DEL do not change cells. A tab at the right margin preserves a pending wrap. Unsupported controls return `ErrUnsupportedOutput` before later text is applied. Keep independent terminal conformance tests alongside this bounded model: silently ignoring an unknown cell-affecting command cannot prove a frame correct.

For an ordered visual assertion, call `host.Repaint(ctx)`. Its snapshot combines the actual canvas size, the full frame, and the output up through that frame. An ordinary write after a request does not prove the repaint happened. Bound the context so a test can also finish if the program exits before taking the request. A real PTY session closes on cancellation, including when its direct child has exited but another member of its process group retains the terminal. If a transcript token never arrived, `WaitFor` reports the reader failure or `io.EOF` once output ends.

For a click on acknowledged output, call `host.Pointer(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: position})` after `Repaint` returns. Leave `At` zero. `Pointer` captures the completed output's immutable geometry before queueing the event, so a later frame cannot retarget it. It returns false if the program has not bound its input session or the writer or geometry is unsettled. Keyboard events, pointer intent and repaint requests share one FIFO.

Use `host.Send` to test physical reports with their actual arrival timestamps. Equal, expired or uncertain timestamps deliberately cannot prove which frame received a click. Synthetic pointer intent has an explicit causal relationship with the output already observed; it needs neither a sleep nor an invented timestamp to cross a clock boundary. Custom test transports can implement `program.PointerSource` to bind this capture capability for one program lifetime.

A screenshot checks appearance, but it does not prove that the same action is available to a keyboard or an agent. Pair visual assertions with action results, target lifetime, source revision or writer receipts when those facts matter.

`ptytest.Session.Wait` observes the direct child's exit while keeping its kernel identity reserved for group cleanup. A failed child returns `*process.ExitError` from `ui/core/process`. Always call `Close`, even after `Wait` succeeds; it sends the final group signal before reaping and releases the reserved process slot. The session owns the sole final wait, so do not independently reap its child or enable automatic SIGCHLD reaping. Context cancellation follows the same close path.

## Exercise transactions and failures

Prepare a frame, inspect that committed geometry has not advanced, then abort. Verify model changes survive while geometry remains at the previous complete frame. Admit another candidate and report physical writing separately. A queued frame is not a written frame.

Inject capacity refusal, worker completion after detach, invalid action parameters, partial writes and native-paint refusal. Stream tests compare final cells against whole-source rendering across arbitrary chunk boundaries, including forward references and split UTF-8.

## Bound repository fixture lifetimes

Tests inside the `ui` module share `ui/internal/scenetest` for component adapters and complete scene frames. Call `scenetest.New()` within each test or fuzz callback, and register the returned scope's `Close` with `t.Cleanup`. Helpers receive that scope explicitly so repeated draws and focus changes use the same component identity and frame sequence. Examples without a test handle use `defer scope.Close()`.

Create a benchmark scope within `b.Run` when its component is reused across iterations. If an iteration creates fresh components, close that iteration's scope before starting the next one. Closing a scope releases every owned scene root and cached adapter; process-wide fixture registries would retain completed tests and fuzz samples.

## Run repository gates

From the checkout, `go -C ui test ./...` covers the public module and its internal architecture tools. `scripts/modules.sh` derives all workspace modules; run each module's tests and `go test -race` when validating a cross-module change. `scripts/sync-workspace.sh --check` checks the derived workspace without editing it.

Documentation tests compile the complete bilingual getting-started program, verify paired pages and validate repository links. `npm run docs:lint` checks Markdown, and `npm run docs:build` checks the published documentation graph. Follow [CONTRIBUTING.md](https://github.com/Tangerg/oolong/blob/main/CONTRIBUTING.md) for the complete pinned release gate.
