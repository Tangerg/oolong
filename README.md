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

Underneath that: a transcript the output can be selected, searched and scrolled
back over; an editor with undo, selection, the system clipboard and atomic
elements, in one line or many; forms with the four fields anything ever asks for;
a diff and something to scroll it in; prompt history and a slash-command registry;
a theme that follows the colour the terminal says it draws on, and box glyphs that
fall back to ASCII when the locale says they must.

Every key is a name in a table rather than a keystroke in a widget, so all of it
can be rebound without replacing anything — and every widget answers to the name,
so all of it can be driven from a menu, from a command typed out, or from a test
that presses nothing.

```sh
go run ./examples/streaming        # from the repository root, which is a Go workspace
```

[`ROADMAP.md`](ROADMAP.md) was what was missing and in what order, read against the
libraries this one was lifted from and the ones it was compared with. It is all
done; what each item turned out to mean — including the three places the
implementation contradicted the analysis — is recorded under it, and what the work
turned up is at the end.

## What it is

Four modules in one repository.

| module | what it is | dependencies |
| --- | --- | --- |
| **`core`** | the engine: cells, text, input, layout, the terminal, frame pacing, and the loop that drives them | `uniseg`, `go-runewidth`, `x/term`, `x/sys` |
| **`components`** | widgets built on it, split into behaviour and appearance | **none of its own** — everything comes through `core` |
| **`ptytest`** | a harness that runs a terminal program on a real pty and says what reached the terminal | `x/sys` |
| **`examples`** | demonstrations, which nothing may import | — |

A module boundary costs version skew and buys an independent dependency set, so
there is one wherever the dependencies genuinely differ and nowhere else. That is
why `core` is not split further into a cell buffer, a styling layer and a runtime:
everyone who wants one wants all three, and three modules would buy nothing but a
version dance. It is also why markdown, when it arrives, has to be its own — it
wants goldmark and a syntax highlighter, and neither of the two modules above
should ever hear about them.

### Inside them, a ladder

| ring | what lives there | the web analogy |
| --- | --- | --- |
| `core/grid`, `text`, `input`, `layout`, `term`, … | cells, graphemes, columns, escape sequences. Knows what a terminal is made of and nothing about what anyone builds from it. | HTML and CSS |
| `components/headless` | behaviour with no appearance. A list knows what the arrow keys do; it does not know what a selected row looks like, and draws one by calling back to whoever does. | Radix |
| `components/kit` | one set of answers to what all that should look like, with a palette. A default, not a destination. | shadcn |
| `core/program`, `core/present` | the loop and its frame schedule. The only goroutine that touches the interface's state, and the one ring that must never know the widgets exist. | the browser |

The layering is not a convention. `internal/arch` parses every import in every
module and fails the build if one points the wrong way, if a module appears whose
dependencies nothing governs, or if the rules themselves would no longer refuse
anything.

`core/program` is deliberately not the top of the ladder. It is beside it: it drives
a `Component`, which is a method set, and a loop that imported the widgets would
make every interface built on it inherit this library's taste in widgets. The module
graph cannot catch that one — `core` could require `components` and Go would allow
it — which is why it is the rule the arch test exists for above all the others.

## Walking away from `kit`

`kit` is where the arguable decisions live — what a border is made of, what a spinner
looks like, which grey is muted text. Every one of those is something a real product
eventually disagrees with, so the way out is built in: stop importing `kit`, keep
`headless`, and nothing else changes. Nothing below `kit` knows it exists.

That is the whole reason `headless.List` has no style fields and takes a `Row`
function instead.

[`ROADMAP.md`](ROADMAP.md) is what is missing and in what order, read against the
libraries this one was lifted from and the ones it was compared with.

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

`core` has four dependencies: `rivo/uniseg`, `mattn/go-runewidth`,
`golang.org/x/term` and `golang.org/x/sys`. `components` imports none of them —
whatever reaches it, reaches it through `core`, which is what makes it obvious that
everything a widget needs is already down there.

Both lists are a promise, and a test fails when either grows — a terminal library
that drags a tree behind it is one people work around instead of using. Splitting
into modules made the promise sharper rather than looser: the list belongs to a
module now, so a future markdown module can carry goldmark without either of these
two noticing.

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
every widget is an ordinary mutable object with no lock in it.

To be exact, because the short version is easy to overstate: the process has more
goroutines than one. `core/term` runs three — a reader that cannot be interrupted
portably, a frame writer so that a slow terminal cannot stop the loop from reading
input, and a resize signal fan. None of them touches a widget. The claim is about
who owns the interface's state, and exactly one goroutine does.

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
