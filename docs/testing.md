---
title: Test an Oolong interface
description: Test components in process and reserve pseudo-terminals for transport behavior.
contentType: How-to
---

# Test an Oolong interface

Language: English | [简体中文](zh/testing.md)

This guide chooses the smallest harness that can observe a behavior. Use
`programtest` for component and application state. Use `ptytest` only when the
claim depends on a real terminal transport.

## Test application behavior in process

`programtest.Host` implements the three required `program.Host` methods. It sends
events and records frames without opening a terminal:

```go
func TestCounterQuits(t *testing.T) {
    host := programtest.New(t, 60, 12)
    done := make(chan error, 1)
    go func() {
        done <- program.Run(t.Context(), program.Config{
            Host: host,
            Root: buildCounter,
        })
    }()

    host.Shows(t, "0 keys")
    host.Type("ab")
    host.Shows(t, "2 keys")
    host.Type("q")
    if err := <-done; err != nil {
        t.Fatal(err)
    }
}
```

`Shows` and `Hides` request a complete repaint before checking text. Use `Frame`
when a test deliberately inspects the latest diff, and `Frames` when ordering across
several writes matters.

## Add one optional host capability

The base test host intentionally implements no optional terminal capability. Embed
it in a local test type and add only the capability under test:

```go
type darkHost struct {
    *programtest.Host
}

func (h darkHost) Ground() grid.Ground {
    return grid.Ground{
        FG: grid.RGBColor(220, 220, 220),
        BG: grid.RGBColor(20, 20, 20),
    }
}
```

This keeps absence observable. A universal fake would make every application think
the clipboard, locale, image transport, progress reporting, and window title always
exist.

## Test terminal ownership on a PTY

Use `ptytest` when the assertion names bytes or terminal state. Build the command
under test into `t.TempDir`, then start it on a PTY:

```go
if !ptytest.Supported() {
    t.Skip("this platform has no PTY harness")
}
session, err := ptytest.Start(t.Context(), ptytest.Config{}, binary)
if err != nil {
    t.Fatal(err)
}
defer func() { _ = session.Close() }()

ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
defer cancel()
if err := session.Transcript().WaitFor(ctx, "ready"); err != nil {
    t.Fatal(err)
}
if err := session.Type("q"); err != nil {
    t.Fatal(err)
}
```

A PTY test can prove facts an in-process host cannot:

- Terminal modes are enabled once and disabled once in reverse order
- An inline block shrinks without leaving old cells behind
- Resize reaches the process through the platform adapter
- An idle interface stops writing bytes
- Published output remains after the live block exits

`ptytest` captures a byte stream; it is not a general terminal emulator. When the
claim is the visible cell text produced by `grid.Screen` or `grid.Inline`, apply that
stream to `ptytest.Screen`. Its renderer-sized model includes cursor movement,
erasure, bounded scrolling, wide cells, and the terminal's delayed wrap at the right
margin; unsupported device traffic returns an error instead of being guessed. Assert
protocol sequences directly when the sequence itself is the contract.

## Keep time and background work deterministic

Prefer explicit channels, `testing/synctest`, and runtime callbacks over sleeps. A
test should wait for an observable frame or state transition, not guess how long a
goroutine needs.

When a source writes through `program.ByteIngress`, close the ingress and wait for
its final owner-side batch. Cancellation tests should prove blocked writers return
and no producer survives the program.

## Preserve fuzz regressions

`go test` replays files under `testdata/fuzz/<Target>` before fuzz generation. Keep
that directory name identical to its `func <Target>(f *testing.F)`: when a target is
renamed, move its corpus in the same change. The architecture gate rejects a corpus
with no live fuzz target, because Go otherwise ignores that directory without an
error.

Corpus files are byte fixtures. The repository forces LF endings for every
`testdata/fuzz/**` path so checkout settings cannot rewrite a seed before Go reads it.

## Give every callable path executable evidence

`scripts/check-reachability.sh` runs the pinned `deadcode` analyzer with tests across
Linux, macOS, and Windows. A private unreachable function is dead implementation. An
unreachable exported operation has no executable contract coverage, but it is not
necessarily dead API: a framework exists for downstream callers, and its extension
points may intentionally have no repository production call. Give that operation an
external-package behavioral test. Retain or remove it only after a separate review of
its responsibility, abstraction level, overlap, and contract.

Reachability never authorizes removal. `scripts/check-api-changelog.sh` separately
uses pinned `apidiff` against each preceding public module tag and requires every
removed export by exact old name in the Unreleased migration ledger. That evidence
cannot prove the design decision, but it prevents deletion from being the silent way
to satisfy the reachability gate.

Keep caller-visible behavior in external-package tests (`foo_test`). A white-box
test may use the implementation package only when the property has no public form,
and its filename must end in `_internals_test.go`. The architecture gate derives
the package boundary from source and rejects an ordinary test that crosses it, so
private coupling stays exceptional and visible during review.

## Update visual goldens deliberately

The complex examples store text goldens for geometry that is easier to review as a
whole frame. Regenerate them only when the new output is intended:

```sh
cd examples
go test -update ./agent
```

Review the changed golden beside the code change. Ordinary `go test ./...` never
rewrites it.

Apply both harnesses to a complete streaming application in
[Build a bounded agent interface](agent.md), or read [Architecture](architecture.md)
for the invariants these tests enforce.
