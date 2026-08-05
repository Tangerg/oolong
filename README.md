# oolong

A terminal interface library for Go, built in layers, for interfaces that stream.

```go
program.Run(ctx, program.Config{
    Inline: func(loop program.InlineLoop) program.Component { return newUI(loop) },
})
```

## What it is

Three rings, each more general than the one above it, and a dependency edge that only
ever points down.

| ring | what lives there |
| --- | --- |
| `primitives/` | cells, text, input, the terminal, frame pacing, ranking. Knows what a terminal is made of and nothing about what anyone builds from it. |
| `atoms/` | widgets with no meaning of their own — a list is a list whether it holds files or sessions. May draw and answer input; may not own a goroutine, a terminal, or a program. |
| `program/` | the loop. The only ring that owns a goroutine. |

The layering is not a convention. `internal/arch` parses every import in the module and
fails the build if one points the wrong way, if a ring appears that no rule governs, or
if the rules themselves would no longer refuse anything.

## What it is for

An interface that streams — a model answering, a build running, a log arriving — wants
two things a full-screen TUI cannot give it: what it has already said should belong to
the terminal, scrollable and selectable and still there after the program exits; and
what it is still doing should be a live block at the bottom.

That is `Config.Inline` and `InlineLoop.Print`. Nothing in the inline renderer names a
row of the terminal, because the block's position is decided by whatever is above it —
which the library does not own and cannot ask about. Every frame is written relative to
where the last one left the cursor.

`Config.Root` is the other mode: a screen of its own, given back on the way out, drawn
by a cell diff that emits the smallest escape stream turning one frame into the next.

## What it costs

Three dependencies: `rivo/uniseg`, `mattn/go-runewidth`, `golang.org/x/term`. The list
is a promise, and a test fails when it grows — a terminal library that drags a tree
behind it is one people work around instead of using.

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
