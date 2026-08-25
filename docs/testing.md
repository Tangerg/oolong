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
    host := programtest.New(t, programtest.Config{Width: 60, Height: 12})
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

`Shows` and `Hides` request a complete repaint and inspect its visible text runs.
Styles and hyperlinks do not split adjacent text, while cursor movement and erasure
remain boundaries. Use `Frame` when a test deliberately inspects the raw latest diff,
and `Frames` when ordering across raw writes matters. Use `ptytest.Screen` when the
claim needs actual terminal cell state rather than content runs.

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
if _, err := io.WriteString(session, "q"); err != nil {
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

This is independent evidence for terminal-side application and storage, not a second
width oracle. `ptytest.Screen` deliberately uses the same text-width estimator as the
renderer, while the width fixtures are the single authority for that estimator. The
screen model independently decides when a printable atom wraps, overwrites, erases, or
scrolls after those widths have been supplied.

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

The only static-only exception is `noCopy.Lock` and `noCopy.Unlock`. Their method set
is consumed by `go vet`, and calling them merely to satisfy a runtime reachability
tool would be fake evidence. The script filters exactly those two names and tests its
own filter with synthetic marker and ordinary-method findings; the architecture gate
separately proves the marker methods still exist.

Reachability never authorizes removal. `go -C internal run ./cmd/apiledger -root ..`
separately uses pinned `apidiff` against each preceding public module tag and requires
every incompatible exported API change by exact name in the Unreleased migration
ledger. Its Go implementation has isolated tests for report parsing, release-section
selection, module-ledger ownership, all three platform passes, and command failures;
there is no second shell implementation of that policy. Public membership comes from
`scripts/modules.sh --public`, the same inventory consumed by CI and release. The
script derives that subset from externally importable production packages rather than
module names. Derivation must read every supported source set, publishes no partial
answer, and rejects an empty public set. Every executable consumer captures that
successful answer before iterating it; standard error is diagnostic data and never a
module name. The architecture gate exercises those failure and symlinked-checkout
paths, independently reads Go's package facts, and proves both the complete and public
sets are exact. The checker reports a public module with no released tag instead of
silently omitting it from compatibility evidence.
That evidence cannot prove the design decision, but it prevents deletion or reshaping
from being the silent way to satisfy the reachability gate.

An entry may use the exact name emitted by `apidiff`, such as `grid.Cell.Width`, or
qualify it with the module ledger name. The latter keeps a root-package entry readable
as `latex.GlyphsFor` instead of the context-free `GlyphsFor`; a different qualifier
does not satisfy the gate. Both forms must appear as one exact token enclosed in
backticks.

The ledger is minimum migration evidence, not a closed inventory. Extra names may be
part of a rename explanation or a coordinated migration, so the checker does not
reject them as stale; `apidiff` alone decides which current breaks must be present.

## Compile terminal-neutral layers where no terminal exists

CI vets and builds every workspace module for both `wasip1/wasm` and `js/wasm`, and
the release gate repeats the same check. This proves source separation: a rendering
or component package cannot quietly acquire a dependency or source file with no
implementation on those targets. It does not prove that a package performs no I/O,
or claim that Oolong supplies a browser, xterm.js, or WASI terminal adapter. A
downstream adapter still owns transport and terminal facts through `program.Host`.

## Spell-check human prose

`npm run spell` checks the Markdown corpus and NOTICE with the exact CSpell version
locked in `package-lock.json`; `npm run docs:check` includes it in CI and releases.
The project dictionary contains deliberate product names, Go tools, protocol terms,
and repository vocabulary. It deliberately does not scan every identifier and test
fixture: accepting thousands of generated or synthetic tokens would make the
dictionary broad enough to hide the prose errors this gate exists to catch.

## Make single-owner types mechanically single-owner

An exported mutable type whose documentation says it must not be copied after first
use carries a direct `noCopy`, `sync`, or `sync/atomic` field. `go vet` therefore
rejects value copies with the standard `copylocks` analyzer. The architecture gate
derives these contracts from production documentation and also verifies that a
private `noCopy` marker still implements both `Lock` and `Unlock`; neither the type
list nor the marker's meaning can silently become stale.

Use this contract only where copying would share mutable storage or lifecycle state.
A deliberately copyable value should instead own immutable data or detach before a
mutation, as `markdown.Doc` does. Do not add a marker merely because methods happen
to use pointer receivers.

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
