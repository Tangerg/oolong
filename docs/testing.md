# Test an Oolong interface

Language: English | [简体中文](testing.zh-CN.md)

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
session, err := ptytest.Start(t.Context(), binary)
if err != nil {
    t.Fatal(err)
}
defer func() { _ = session.Close() }()

if err := session.Transcript().WaitWithin(5*time.Second, "ready"); err != nil {
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

`ptytest` captures a byte stream; it is not a general terminal emulator. Assert
protocol sequences directly when the sequence itself is the contract.

## Keep time and background work deterministic

Prefer explicit channels, `testing/synctest`, and runtime callbacks over sleeps. A
test should wait for an observable frame or state transition, not guess how long a
goroutine needs.

When a source writes through `program.ByteIngress`, close the ingress and wait for
its final owner-side batch. Cancellation tests should prove blocked writers return
and no producer survives the program.

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
