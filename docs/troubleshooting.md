---
title: Troubleshoot an Oolong application
description: Diagnose module, terminal, input, streaming, memory, and test failures by ownership boundary.
contentType: Troubleshooting
---

# Troubleshoot an Oolong application

Language: English | [简体中文](zh/troubleshooting.md)

Start from the visible symptom, then inspect the owner of that state. Oolong separates terminal facts, owner-side components, background bytes, and committed output so each failure has one primary boundary.

## The application builds in the repository but not alone

`go.work` can satisfy a local import that `go.mod` does not declare. Reproduce the consumer graph with the workspace disabled:

```sh
GOWORK=off go mod tidy -diff
GOWORK=off go test ./...
```

Add the missing Oolong module at the same version as the imported set. Do not commit a `replace` directive; consumers ignore workspace replacements.

## Imported Oolong packages no longer compile together

Oolong is pre-1.0 and publishes coordinated module versions. Inspect the versions selected by minimal version selection:

```sh
go list -m all | grep github.com/Tangerg/oolong
```

Upgrade every imported Oolong module together, then read the [changelog](https://github.com/Tangerg/oolong/blob/main/CHANGELOG.md) for breaking changes.

## Unicode furniture renders as broken bytes

Resolve glyphs from the terminal driven by the runtime:

```go
glyphs := kit.GlyphsFor(runtime.Environment().Locale())
formulaGlyphs := latex.GlyphsFor(runtime.Environment().Locale())
```

Do not read `LANG`, `LC_ALL`, or `TERM` from the server process inside a component. An SSH client and the server process can describe different terminals.

## Pointer input targets an old region

Wrap a live headless tree with `headless.NewRoot`. Stage geometry during `Draw` and read it during `Handle`:

```go
w.areas.Stage(frame, nextAreas)

func (w *widget) Handle(event input.Event) bool {
    areas := w.areas.Value()
    return w.route(event, areas)
}
```

Do not advance semantic state during `Draw`. `headless.Snapshot` publishes presentation geometry only after the complete root frame succeeds.

## A background writer blocks

`ByteIngress.Write` applies backpressure when accepted bytes reach the configured limit. Check these conditions:

- The owner callback returns instead of waiting for the producer
- The runtime owner is not blocked on network, process, or model I/O
- Cancellation reaches the producer context
- The producer calls `CloseWithError` once
- Resource cleanup waits on `ByteIngress.Done` only from another goroutine

Increasing the byte limit can absorb larger bursts, but it cannot repair an owner callback that never returns.

## Live memory grows with session age

Finished transcript blocks must leave the live graph. Mark stable blocks finished, keep a bounded interactive prefix, and commit the excess through `kit.Transcript.CommitN`.

Keep only the open Markdown tail mutable. Do not retain substrings or rows from already committed output. The [streaming guide](streaming.md) defines the ownership cut, and the [agent guide](agent.md) shows bounded retention.

## The terminal is not restored after exit

Let `program.Run` return before calling `os.Exit`. Return errors through the runtime boundary instead of exiting from a component or background goroutine.

Use a real pseudoterminal (PTY) test when the claim concerns raw mode, cursor visibility, alternate-screen symmetry, or inline cleanup. `programtest` cannot observe operating-system terminal state.

## An interface test hangs

Wait for observable output or a state transition instead of sleeping. For a streaming test, close ingress and wait for the final `ByteBatch`; for a runtime test, always send the event that terminates the program.

```go
host.Shows(t, "ready")
host.Type("q")
if err := <-done; err != nil {
    t.Fatal(err)
}
```

If a cancellation test still hangs, assert that blocked writers return and that no producer survives `Dispatcher.Done`.

## Collect a reproducible report

Include these facts in a [bug report](https://github.com/Tangerg/oolong/issues/new?template=bug.yml):

- Oolong module versions from `go list -m all`
- Go version, operating system, terminal, and local or SSH transport
- Minimal input, program, or protocol bytes that reproduce the symptom
- Expected and observed ownership transition
- Whether `programtest` or `ptytest` reproduces it

Report security boundaries privately through the [security policy](https://github.com/Tangerg/oolong/blob/main/SECURITY.md).
