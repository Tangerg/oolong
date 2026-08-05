# oolong

[![ci](https://github.com/Tangerg/oolong/actions/workflows/ci.yml/badge.svg)](https://github.com/Tangerg/oolong/actions/workflows/ci.yml)
[![go reference](https://pkg.go.dev/badge/github.com/Tangerg/oolong.svg)](https://pkg.go.dev/github.com/Tangerg/oolong)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

A terminal interface library for Go, built in layers, for interfaces that stream.

```go
program.Run(ctx, program.Config{
    Inline: func(loop program.InlineLoop) program.Component { return newUI(loop) },
})
```

A chat interface that prints into your terminal's scrollback and keeps a live block
at the bottom is about a hundred lines: [`examples/streaming`](examples/streaming).

```sh
go run ./examples/streaming
```

## What it is

A ladder of abstraction, and a host beside it.

| ring | what lives there | the web analogy |
| --- | --- | --- |
| `primitives/` | cells, text, input, layout, the terminal, frame pacing, ranking. Knows what a terminal is made of and nothing about what anyone builds from it. | HTML and CSS |
| `headless/` | behaviour with no appearance. A list knows what the arrow keys do; it does not know what a selected row looks like, and draws one by calling back to whoever does. | Radix |
| `kit/` | one set of answers to what all that should look like, with a palette. A default, not a destination. | shadcn |
| `program/` | the loop. The only ring that owns a goroutine, and the one that must never know the widgets exist. | the browser |
| `ptytest/` | a harness that runs a terminal program on a real pty and says what reached the terminal. Nothing in the library imports it. | the test runner |

The layering is not a convention. `internal/arch` parses every import in the module
and fails the build if one points the wrong way, if a ring appears that no rule
governs, or if the rules themselves would no longer refuse anything.

`program/` is deliberately not the top of the ladder. It is beside it: it drives a
`Component`, which is a method set, and a loop that imported the widgets would make
every interface built on it inherit this library's taste in widgets.

## Walking away from `kit`

`kit` is where the arguable decisions live — what a border is made of, what a spinner
looks like, which grey is muted text. Every one of those is something a real product
eventually disagrees with, so the way out is built in: stop importing `kit`, keep
`headless`, and nothing else changes. Nothing below `kit` knows it exists.

That is the whole reason `headless.List` has no style fields and takes a `Row`
function instead.

## What it is for

An interface that streams — a model answering, a build running, a log arriving —
wants two things a full-screen TUI cannot give it: what it has already said should
belong to the terminal, scrollable and selectable and still there after the program
exits; and what it is still doing should be a live block at the bottom.

That is `Config.Inline` and `InlineLoop.Print`. Nothing in the inline renderer names a
row of the terminal, because the block's position is decided by whatever is above it —
which the library does not own and cannot ask about. Every frame is written relative
to where the last one left the cursor.

`Config.Root` is the other mode: a screen of its own, given back on the way out, drawn
by a cell diff that emits the smallest escape stream turning one frame into the next.

## What it costs

Three dependencies: `rivo/uniseg`, `mattn/go-runewidth`, `golang.org/x/term` (and
`golang.org/x/sys` behind the last of them). The list is a promise, and a test fails
when it grows — a terminal library that drags a tree behind it is one people work
around instead of using.

## Testing an interface

Two ways, and the second is why the first is not enough:

```go
program.Run(ctx, program.Config{Host: myHost, Inline: ...})   // no terminal in sight
ptytest.Start("./my-cli")                                     // a real pty
```

A host proves an interface drew the frame it meant to. A pty proves the bytes of
that frame do to a terminal what they were supposed to — that the block shrank
without debris, that an idle interface writes nothing at all, that every mode the
session turned on was turned off again in the reverse of the order it was set up.
That last one is a terminal the user has to close if you get it wrong, and nothing
short of a real pty can see it happen.

## Concurrency, in full

One goroutine draws and handles input. Anything that happens elsewhere reaches the
interface through `Loop.Post` and runs there. That is the whole of it, and it is why
every widget below `program` is an ordinary mutable object with no lock in it.

The program parks when there is nothing to do. It wakes for input, for posted work, and
for the terminal reporting progress — never on a clock that runs regardless. A component
that wants a clock starts one with `Loop.Every`, and an interface with nothing animating
costs nothing.

## More

[DESIGN.md](DESIGN.md) is the orientation document: where the library came from, what
was taken from whom and what was left, what is built, what is deliberately not here,
what is missing, and what its known limits are.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
