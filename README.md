# oolong

[![ci](https://github.com/Tangerg/oolong/actions/workflows/ci.yml/badge.svg)](https://github.com/Tangerg/oolong/actions/workflows/ci.yml)
[![go reference](https://pkg.go.dev/badge/github.com/Tangerg/oolong.svg)](https://pkg.go.dev/github.com/Tangerg/oolong)
[![license](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

A terminal interface library for Go, built in layers, for interfaces that stream.

```go
program.Run(ctx, program.Config{
    Inline: func(runtime *program.InlineRuntime) program.Component {
        return headless.NewRoot(newUI(runtime))
    },
})
```

A canonical chat interface that combines bounded background ingress, incremental
markdown, a selectable recent transcript, terminal-owned history, approval and
cancellation is [`examples/streaming`](examples/streaming).
There are [seven more](examples), shallowest first — a key count, a form, a picker,
a file browser, a dashboard, a command runner, and an answer arriving a few
characters at a time.

Underneath that: a transcript the output can be selected, searched and scrolled
back over; an editor with undo, selection, the system clipboard and atomic
elements, in one line or many; forms with the four fields anything ever asks for,
answerable on a grid or a question at a time in words; lists, trees, tabs, tables
with a cursor and a filter over any of them; a diff and something to scroll it in;
prompt history and a slash-command registry; a theme that follows the colour the
terminal says it draws on, and box glyphs that fall back to ASCII when the locale
says they must.

It reads the terminal's own language in both directions: the colours a command
wrote into its output come back as styled text, and the terminal can be handed to
an editor and taken back with every mode it was holding put back in order.

Every key is a name in a table rather than a keystroke in a widget, so all of it
can be rebound without replacing anything — and every widget answers to the name,
so all of it can be driven from a menu, from a command typed out, or from a test
that presses nothing.

```sh
go run ./examples/streaming        # from the repository root, which is a Go workspace
```

[`ROADMAP.md`](ROADMAP.md) was what was missing and in what order, read against the
libraries this one was lifted from and the ones it was compared with, and then read
again against `opentui` and the `bubbletea` family. Both lists are done, and what
each item turned out to mean — including the places the implementation contradicted
the analysis — is recorded under it. One thing on the second list is not done and
says why: a cell holds a grapheme and a style, and an image is neither.

## What it is

Six modules in one repository.

| module | what it is | dependencies |
| --- | --- | --- |
| **`core`** | the engine: cells, text, input, layout, the terminal, frame pacing, and the runtime that drives them | `uniseg`, `go-runewidth`, `x/term`, `x/sys` |
| **`components`** | widgets built on it, split into behaviour and appearance | **none of its own** — everything comes through `core` |
| **`markdown`** | markdown into terminal rows, including markdown that has not finished arriving | `goldmark` |
| **`highlight`** | source code into styled lines, which is what a markdown look asks for | `chroma` |
| **`ptytest`** | a harness that runs a terminal program on a real pty and says what reached the terminal | `x/sys` |
| **`examples`** | demonstrations, which nothing may import | — |

A module boundary costs version skew and buys an independent dependency set, so
there is one wherever the dependencies genuinely differ and nowhere else. That is
why `core` is not split further into a cell buffer, a styling layer and a runtime:
everyone who wants one wants all three, and three modules would buy nothing but a
version dance. It is also why markdown and highlighting are their own — one wants a
parser and the other a lexer per language, and the two modules above promise a
dependency list a terminal library can be adopted for. They do not depend on each
other either: a document with no highlighter draws code in one style, and a program
that wants one says so in a line.

### Inside them, a ladder

| ring | what lives there | the web analogy |
| --- | --- | --- |
| `core/ansi`, `grid`, `layout`, … | foundational values, byte protocols and pure geometry; only stdlib concepts or their own domain values | HTML and CSS primitives |
| `core/input`, `text`, `present` | decoded protocols, derived text models and frame coordination, each depending only on foundations | DOM and scheduling |
| `core/keymap` | interaction policy over decoded keys: named actions, bindings and independent sequence state | event bindings |
| `core/term` | the operating-system terminal adapter over those protocols | browser platform adapter |
| `components/headless` | behaviour with no appearance. A list knows what the arrow keys do; it does not know what a selected row looks like, and draws one by calling back to whoever does. | Radix |
| `components/kit` | one set of answers to what all that should look like, with a palette. A default, not a destination. | shadcn |
| `core/program` | runtime composition over the terminal adapter and frame schedule. The only goroutine that touches interface state, and a ring that never knows the widgets exist. | the browser |
| `core/programtest` | an in-process host above the runtime for application tests; its base type deliberately has no optional terminal capabilities. | browser test harness |
| `markdown`, `highlight` | beside the ladder rather than on it: they turn text into the substrate's own lines, so anything that can draw those can draw a document or a block of code without either of them knowing about the other. | — |

The layering is not a convention. `internal/arch` declares the direct dependency
DAG and parses every production import and API comment. It fails the build if an
edge has no path downward, if a module appears whose dependencies nothing governs,
or if the graph is incomplete or cyclic.

`core/program` is deliberately not the top of the ladder. It is beside it: it drives
a `Component`, which is a method set, and a runtime that imported the widgets would
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

## What it is for

An interface that streams — a model answering, a build running, a log arriving —
wants two things a full-screen TUI cannot give it: what it has already said should
belong to the terminal, scrollable and selectable and still there after the program
exits; and what it is still doing should be a live block at the bottom.

That is `Config.Inline` and `InlineRuntime.Print`. Nothing in the inline renderer names a
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
module now, which is what lets `markdown` carry goldmark without either of these two
noticing, and a program that does not render markdown never hear about it.

## Testing an interface

Two ways, and the second is why the first is not enough:

```go
host := programtest.New(t, 80, 24)
go program.Run(t.Context(), program.Config{Host: host, Inline: ...})
host.Shows(t, "ready")

session, err := ptytest.Start(t.Context(), "./my-cli") // a real pty
```

A `programtest.Host` proves an interface drew the frame it meant to, while remaining
embeddable when a test needs to observe one optional capability. A pty proves the bytes of
that frame do to a terminal what they were supposed to — that the block shrank
without debris, that an idle interface writes nothing at all, that every mode the
session turned on was turned off again in the reverse of the order it was set up.
That last one is a terminal the user has to close if you get it wrong, and nothing
short of a real pty can see it happen.

## Concurrency, in full

One goroutine draws and handles input. Anything that happens elsewhere reaches the
interface through the small `Dispatcher` returned by `Runtime.Dispatcher`, and runs
there. The concrete `Runtime` stays with the interface goroutine; background work cannot
accidentally call an operation that assumes ownership of the component tree or the
terminal. That is the whole of it, and it is why every widget is an ordinary mutable
object with no lock in it.

Runtime services are concrete domain objects rather than a wide service interface:
`Environment` owns negotiated terminal facts, `Clipboard` owns copy and paste,
`Session` owns handover and attention, and `Images` owns image transport. A component
is handed only the object it actually needs.

To be exact, because the short version is easy to overstate: the process has more
goroutines than one. `core/term` runs three — a reader that cannot be interrupted
portably, a frame writer so that a slow terminal cannot stop the runtime from reading
input, and a resize signal fan. None of them touches a widget. The claim is about
who owns the interface's state, and exactly one goroutine does.

The program parks when there is nothing to do. It wakes for input, for posted work, and
for the terminal reporting progress — never on a clock that runs regardless. A component
that wants a clock starts one with `Runtime.Every`, and an interface with nothing animating
costs nothing. A busy interface keeps at most one tick waiting, so resuming it does
not replay time that has already passed.

## More

[DESIGN.md](DESIGN.md) is the orientation document: where the library came from, what
was taken from whom and what was left, what is built, what is deliberately not here,
what is missing, and what its known limits are.

[docs/architecture.md](docs/architecture.md) ([简体中文](docs/architecture.zh-CN.md)) is
the target architecture: the streaming lifetime and ownership model, dependency and
API rules, the parts taken from frontend and Flutter systems, and the executable gates
future abstractions have to pass on the way to v1.

[docs/brand.md](docs/brand.md) ([简体中文](docs/brand.zh-CN.md)) records the name,
positioning, voice, visual direction, and the boundary that keeps brand metaphors out
of the Go API.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
