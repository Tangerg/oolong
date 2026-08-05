# oolong — what it is, where it came from, what is missing

This is the orientation document. [README](README.md) is the front door; this is the
part that says why the library is shaped the way it is, what was taken from whom, what
has actually been built, and what has not.

---

## 1. Positioning

**A terminal interface library for Go, in layers, for interfaces that stream.**

The name answers bubbletea. Boba is the pearl in the tea; oolong is the leaf. The
difference in the libraries is the same kind of difference:

| | bubbletea | oolong |
| --- | --- | --- |
| state | an immutable model, replaced each update | retained mutable widgets, owned by one loop |
| a frame | a string the model returns, diffed by line | cells drawn into a surface, diffed by cell |
| the point | a program the user enters and leaves | an interface that is part of a session |

The third row is the one that matters. A full-screen TUI is a mode: it takes the
screen, and on exit gives back a blank terminal with nothing to show that it ran. An
interface that *streams* — a model answering, a build running, logs arriving — wants
the opposite. What it has already said should belong to the terminal: scrollable,
selectable, greppable in the user's scrollback, still there tomorrow. What it is still
doing should be a live block at the bottom, and only that.

### The objection this positioning has to answer

"An interface that streams" is not on its own a reason to exist, and pretending it is
would not survive the first informed reader. Bubble Tea's default is already inline —
the alternate screen is opt-in — and `tea.Println` has printed above the live view
into scrollback for years. Its flagship users are streaming agent CLIs. Anyone who
knows the ecosystem will ask about this in their first sentence, so it is answered
here rather than left for them to find.

What is different is narrower and checkable:

- **What a frame costs.** An Elm update rebuilds the model and returns a fresh view
  string every token. Here the composer, the transcript and the spinner are retained
  objects owned by one loop, and a frame is a cell diff against the last one. In a
  session that runs for hours and mutates constantly, that is the cheaper description
  as well as the simpler one.
- **What a diff is.** Bubble Tea's standard renderer diffs strings by line. This diffs
  cells. Wide characters, combining marks and emoji are where line diffing produces the
  misalignment every terminal UI eventually has, and a cell grid makes the wide-pair
  invariant something the type system holds rather than something everyone remembers.
- **What it drags in.** Three dependencies against bubbletea + bubbles + lipgloss +
  glamour.

None of that is proven yet — see the note on benchmarks in §6. Until it is, this
section is a claim and should be read as one.

That is the reason this library exists rather than being a wrapper over an existing
one. Everything else here follows from it.

### What "in layers" buys

A ladder of abstraction, and a host beside it. The web has the same shape and the
names are borrowed from it deliberately, because the shape is what people already
know:

| ring | knows about | must never touch | the analogy |
| --- | --- | --- | --- |
| `primitives/` | cells, graphemes, columns, escape sequences, layout, the terminal, pacing, ranking | anything built from them | HTML, CSS |
| `headless/` | what a list does, what a press means, where a cursor goes | what any of it looks like; goroutines; programs | Radix |
| `kit/` | what all that should look like, and a palette | goroutines, programs | shadcn |
| `program/` | the loop, the frame schedule, the one goroutine | the widgets | the browser |

The last row is the one that is easy to get wrong, and the first version of this
library did. `program/` is not the top of the ladder — it is orthogonal to it. It
drives a `Component`, which is a method set, and the day it imports `headless` is the
day every interface built on this library inherits its taste in widgets. That edge is
the one `internal/arch` exists for above all the others.

The split between `headless/` and `kit/` is the other thing worth stating plainly.
Everything arguable lives in `kit`: what a border is made of, what a spinner looks
like, which grey is muted text. All of it is a matter of taste that a real product
eventually disagrees with, so the exit is designed in — stop importing `kit`, keep
`headless`, and nothing below it changes. That is why `headless.List` has no style
fields and takes a `Row` function instead, and it is the difference between a library
people adopt and a library people fork.

The layering is not a convention anyone is trusted to keep. `internal/arch` parses
every import in the module and fails when one points the wrong way, when a directory
appears that no rule governs, when a fourth dependency is added, when anything above
`primitives/` reaches for what draws the terminal, or when the rules themselves would
no longer refuse anything. The last one is the counter-example: a guard never shown to
fail is a guard nobody knows is wired up.

### The dependency promise

Three: `rivo/uniseg`, `mattn/go-runewidth`, `golang.org/x/term`. This is a promise, not
a coincidence, and a test fails when the list grows. A terminal library that drags a
tree behind it is one people work around instead of adopting — which is why anything
needing a heavy dependency (markdown, syntax highlighting, images) belongs in a sibling
module and not in here.

---

## 2. Prior art

### agentui — the direct source

Most of `primitives/` is a reimplementation of [agentui][agentui] (Apache-2.0), a
self-built Go terminal engine. The attribution obligation is in [NOTICE](NOTICE); this
section is about the engineering.

| agentui | here | what changed |
| --- | --- | --- |
| `cellbuf` | `primitives/grid` | one painter instead of four near-identical emit loops; control characters dropped at the cell as a trust boundary; scroll shortcut gated on a measured byte-cost floor |
| `textwrap` | `primitives/text` | one width authority, with an explicit width table instead of the locale's |
| `term` + `input` | `primitives/term`, `primitives/input` | one escape parser instead of two divergent ones; the frame writer's unbounded queue and condition variable replaced by a sequence watermark |
| `present` | `primitives/present` | a throttled request always owes a frame; the interval decides when, not whether |
| `fuzzy` | `primitives/fuzzy` | anchored best-of instead of pure greedy; beginning the candidate scores above beginning a word inside it |
| `overlay`, `theme` | `primitives/layout`, `kit` | placement separated from drawing, so a hit test a frame later asks the same question |
| widgets living inside its runtime | `headless/` and `kit/` | lifted out, then split down the middle: what a widget does from what it looks like |

agentui's largest package by far is its `runtime` — a whole agent CLI. That is the part
that did not come across, and the reason the port is a fraction of the size: what was
taken is the engine, and what was left is a product.

The clearest example is completion. agentui's version knows about a hidden-file opt-in
spelled `@!`, a shell mode opening on `!`, a completion source enum with an `ai` member,
a truncation flag so that Tab will not infer a common prefix from a capped result set,
and a "drill anchor" protecting one previously accepted directory whose name contains
spaces. All of that is a product's grammar wearing a library's clothes. What is general
is a scorer and a list, and that is what `primitives/fuzzy` and `headless.Completion`
are.

One thing agentui does that was read and deliberately inverted: its `theme` package
returns a `*scrollback.Theme`, so the palette is owned by the transcript engine and
everything that wants a colour depends on it. Here the palette is a value in `kit`
that nothing below `kit` can see. The engine does not get to own the paint.

agentui states that it is itself an independent reimplementation of the terminal
interface of Grok CLI, and is not affiliated with xAI. Nothing here was taken from Grok
CLI directly.

### bubbletea and the Charm ecosystem — compared, then not used

Read closely and deliberately declined, for reasons that are about fit and not quality:

- **The Elm model.** Immutable model-update-view is a real answer to "who owns the
  state", and its cost is that every frame is a fresh string and every widget change is
  a new value. For an interface where a text field, a transcript and a spinner all live
  for the whole session and mutate constantly, retained widgets owned by one loop are
  the simpler description. The single-owner rule buys the same safety without the copying.
- **Cells over strings.** Lipgloss composes styled strings; this composes cells. A cell
  grid is what makes clipping a boundary rather than a convention, makes a wide-character
  pair an invariant the type system can hold, and makes the frame diff exact.
- **The dependency graph.** bubbletea + bubbles + lipgloss + glamour is the right
  trade for most programs and the wrong one for a library that wants to be adopted rather
  than wrapped.

Its influence is real anyway: the inline renderer's relative-cursor approach is the same
technique bubbletea's standard renderer uses, arrived at for the same reason.

### Others read

Five terminal agent clients were read for what their interfaces actually do, rather than
for code: Claude Code, Codex, opencode, kimi-code, and Grok's build. What they have in
common — and what this library is built to make easy — is the streaming-transcript shape:
finished output printed permanently, a composer pinned below it, approvals interrupting
in place.

---

## 3. What is built

### primitives

- **`grid`** — styled grapheme cells; a clipped drawing view whose coordinates are
  local, so a widget cannot draw outside its box. Two ways a frame reaches a terminal:
  **`Screen`**, which takes the whole screen, double-buffers, and emits the smallest
  escape stream that turns one frame into the next (with a terminal-side scroll
  shortcut taken only when it beats a measured floor on what the plain diff must cost);
  and **`Inline`**, which draws a block in the terminal's own screen and prints finished
  output above it. A flush that would change nothing writes nothing, so an idle
  interface is silent on the wire and the cursor keeps blinking.
- **`text`** — grapheme clusters, column measurement, wrapping, truncation, tab
  expansion. One width authority shared by everything that measures or draws, because
  measuring text one way and drawing it another is the cause of every misaligned
  terminal UI.
- **`input`** — an incremental parser for what a terminal sends: CSI and SS3 keys, the
  Kitty keyboard protocol including release and repeat and associated text, SGR mouse
  reporting with movement, bracketed paste, focus. Events are a sealed interface.
- **`term`** — the only package that touches the operating system. Raw mode, the modes
  a session turns on and the reverse order they are put back in, the goroutines reading
  input, and a frame writer with its own goroutine so that a slow terminal cannot stop
  the loop from reading input.
- **`present`** — when to draw. Coalescing, throttling, and refusing to draw while the
  terminal is still swallowing the last frame.
- **`fuzzy`** — subsequence ranking, answering in byte offsets because whatever asks is
  about to draw the candidate with the matched characters picked out.

- **`layout`** — dividing a region: fixed, flexible and measured slots, insets,
  alignment, and the placement a floating layer is clamped into. It hands back views
  and never draws, which is what lets the same rules place a widget, a string, or a
  hole left deliberately empty. A measured slot is asked about the axis being divided
  given the room across the other one, so `Measured` means the same thing in a row and
  in a column — the earlier version could only answer for height, and a measured
  column silently came out zero wide.

### headless

Behaviour with no appearance: scroll position that follows a live log without dragging
a reader who scrolled up, mouse tracking that commits a click on release over the
target that took the press, a generic list, a multi-line editor with a kill ring and
coalesced undo, a completion offered against a token, and the key bindings all of them
match against.

Nothing here decides what any of it looks like. A list draws a row by calling back to
whoever does, which is the one design decision that makes the ring above it optional.

### kit

One set of answers: box and border, label, wrapped paragraph, spinner, scrollbar, help
row, table, a floating layer with shading, and the three pieces a streaming interface
is actually made of — a composer, a status line, and a printed message. Plus a
semantic palette, `Theme`, whose names are roles rather than colours.

It is a default and not a destination, and the whole ring is designed to be walked
away from.

### program

One goroutine draws and handles input. Anything that happens elsewhere reaches the
interface through `Loop.Post` and runs there. That is the whole concurrency model, and
it is why every widget below this ring is an ordinary mutable object with no lock in it.

The loop parks when there is nothing to do — it wakes for input, for posted work, and
for the terminal reporting progress, never on a clock that runs regardless. A component
that wants a clock starts one with `Loop.Every`, built on `Post`, so the loop itself
contains no timer logic and an interface with nothing animating costs nothing.

`Config.Root` and `Config.Inline` are the two modes, and which one is set is what
decides the rendering model. `InlineLoop` is a separate interface precisely so that a
program on a screen of its own cannot be handed a component that prints: there is no
scrollback there, and the alternative was a method that quietly did nothing half the
time.

### How it is kept honest

- The arch tests above, each with a counter-example.
- Tests state behaviour rather than implementation: what an idle frame must not write,
  what a release away from a button must not do, what a resize must repaint.
- The inline renderer was verified against a real pty with an emulator implementing only
  the sequences the renderer is allowed to use — transcript accumulating above the block,
  the block shrinking with no debris, the caret on the right row and column every frame,
  correct behaviour on a terminal too short for the interface, and the caller's own
  output landing below the block after `Run` returns.

---

## 4. Deliberately not here

Not "later" — these are decisions:

- **Application grammar.** What a candidate is, where candidates come from, what
  accepting one means, what a slash command is, what `@` refers to. A library that knew
  would be a framework for one program.
- **A retry layer, a logger, an observability abstraction.** A library that owns none of
  these is one that fits into a program that already has them.
- **Colour degradation and terminfo.** A colour is the terminal's default or a
  truecolor value, and optional behaviours are asked for rather than detected — a
  terminal that does not implement a request ignores it. See the limits below: this is a
  decision with a known cost, not a free one.
- **A widget for everything.** `kit` holds the ones a streaming interface actually
  needs, and `headless` the behaviour worth sharing underneath them. A library whose
  widget count is its selling point ends up with fifty widgets and no layering.

---

## 5. What is missing

Ordered by what would be built next.

1. **Markdown, as a sibling module.** Rendering a model's answer is the single most
   common thing a streaming interface does, and doing it properly wants goldmark and a
   syntax highlighter. Those are exactly the dependencies the core promise excludes, so
   this is a separate module in this repository — the same relationship glamour has to
   bubbletea. Not started.
2. **A testing harness.** There is no equivalent of bubbletea's teatest or agentui's
   ptytest — the latter is worth porting almost as it stands, particularly its
   `RequireSymmetricModes`, which asserts that every terminal mode a session turned on
   was turned off again. What exists today is that an interface can be driven through
   a `program.Host` with no terminal in sight, which is how `examples/streaming` is
   tested end to end; what is missing is a real pty and the assertions to go with it.
   This is the largest remaining gap for adopters.
3. **A focus model.** Nothing assigns the keyboard to one of several widgets; a container
   routes events by trying its children in an order it chose. That works for one composer
   and one transcript and stops working at the first dialog with two fields.
4. **Images and graphics.** Kitty and iTerm2 inline images, sixel. Real for an agent
   that reads screenshots, and last because it is the least general.
5. **Search over a transcript.** Product-shaped enough that it may never belong here.

Not in the list because they are not the library's: syntax-aware editing, a shell,
process management.

---

## 6. Known limits

Stated because a limit nobody wrote down is a bug report waiting to happen.

- **Inline mode and resize.** The terminal may reflow what is above the block, and
  there is no way to ask where the block ended up, so the next frame repaints in full
  from where the cursor was left. Exact when the terminal did not reflow, approximate
  when it did. Querying the cursor position (DSR) would make it exact and needs a
  synchronous round trip through an asynchronous loop; not done.
- **No terminfo, and no colour fallback.** Truecolor is emitted unconditionally. On a
  terminal without it, colours will be wrong rather than degraded. This is the one place
  where "ask and let it be ignored" does not hold, and it is the most likely thing to
  need fixing first.
- **Windows has no resize events.** The console reports resizing through its own input
  API rather than a signal. A session gets its opening size and nothing after, unless a
  host delivers sizes itself. Everything else is portable.
- **`fuzzy` is not a full alignment search.** It tries the first placement and every
  placement that begins a word, and keeps the best. A better placement in a candidate
  with no word boundary to hint at it can be missed. Palette-sized candidate lists make
  this the right stopping point; a scoring matrix per candidate is what it would take.
- **The scroll shortcut is `Screen`-only.** Inline mode has no equivalent, because a
  block cannot address the region it would scroll.
- **No benchmarks.** Nothing here has been measured, only reasoned about and bounded.
- **`Loop.Post` must not be called from the loop's own goroutine while its queue is
  full.** The buffer absorbs a burst, and a component that posted from inside `Draw`
  or `Handle` faster than the loop drains would block the only consumer there is. It
  takes 256 outstanding posts to reach, and `InlineLoop.Print` is the realistic way in.
- **Not published, not versioned, no CI.** It is consumed from a sibling checkout by one
  program. Everything about the public API is still cheap to change, and it should be
  changed while that is true.

---

## 7. Provenance and licence

Apache-2.0. The derived parts and their upstream are named in [NOTICE](NOTICE), which
is an obligation and not a courtesy.

[agentui]: https://github.com/minoism/agentui
