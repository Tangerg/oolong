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

Two kinds of boundary, enforced two different ways.

A **module** boundary is where the dependencies differ. `core` carries the whole
third-party list; `components` carries none; a future markdown module will carry
goldmark and a highlighter, and neither of the first two will hear about it. That
is the only thing a module boundary is worth paying for — it costs version skew,
and the Charm ecosystem's own v2 migration is the standing demonstration of what
that costs when bubbletea, bubbles and lipgloss all have to move together.

Which is why `core` is one module and not three. Charm splits the cell buffer
(`ultraviolet`), the styling layer (`lipgloss`) and the runtime (`bubbletea`) into
separate repositories, and that split is ten years of organic growth rather than a
design. Everyone who wants one of ours wants all three.

A **ring** boundary is inside a module, where the compiler cannot see it. The web
has the same shape and the names are borrowed from it deliberately, because the
shape is what people already know:

| ring | knows about | must never touch | the analogy |
| --- | --- | --- | --- |
| `core/grid`, `text`, `input`, `layout`, `term`, … | cells, graphemes, columns, escape sequences, layout, the terminal, pacing, ranking | anything built from them, including the loop | HTML, CSS |
| `components/headless` | what a list does, what a press means, where a cursor goes | what any of it looks like; goroutines; programs | Radix |
| `components/kit` | what all that should look like, and a palette | goroutines, programs | shadcn |
| `core/program`, `core/present` | the loop, the frame schedule, the goroutine that owns the interface | the widgets | the browser |

The last row is the one that is easy to get wrong, and the first version of this
library did. `core/program` is not the top of the ladder — it is orthogonal to it.
It drives a `Component`, which is a method set, and the day it imports `headless` is
the day every interface built on this library inherits its taste in widgets.

Note that the module graph does *not* catch this: `core` could require `components`
and Go would allow it. That is exactly why the edge is checked by hand, and why it
is the rule `internal/arch` exists for above all the others.

The split between `headless/` and `kit/` is the other thing worth stating plainly.
Everything arguable lives in `kit`: what a border is made of, what a spinner looks
like, which grey is muted text. All of it is a matter of taste that a real product
eventually disagrees with, so the exit is designed in — stop importing `kit`, keep
`headless`, and nothing below it changes. That is why `headless.List` has no style
fields and takes a `Row` function instead, and it is the difference between a library
people adopt and a library people fork.

The layering is not a convention anyone is trusted to keep. `internal/arch` parses
every import in every module and fails when one points the wrong way, when a directory
appears that no rule governs, when a dependency nothing declared is added, when anything
above the substrate reaches for what draws the terminal, or when the rules themselves
would no longer refuse anything. The last one is the counter-example: a guard never
shown to fail is a guard nobody knows is wired up.

### The dependency promise

`core`: `rivo/uniseg`, `mattn/go-runewidth`, `golang.org/x/term`, `golang.org/x/sys`.
`components`: none of its own — it imports nothing outside `core` and the standard
library, and the four appear in its go.mod only as what `core` brought with it. Both
are promises rather than coincidences, and a test fails when either list grows.

A terminal library that drags a tree behind it is one people work around instead of
adopting — which is why anything needing a heavy dependency (markdown, syntax
highlighting) becomes a module of its own, with a list of its own, and neither of
these two is touched.

---

## 2. Prior art

### agentui — the direct source

Most of `core/` is a reimplementation of [agentui][agentui] (Apache-2.0), a
self-built Go terminal engine. The attribution obligation is in [NOTICE](NOTICE); this
section is about the engineering.

| agentui | here | what changed |
| --- | --- | --- |
| `cellbuf` | `core/grid` | one painter instead of four near-identical emit loops; control characters dropped at the cell as a trust boundary; scroll shortcut gated on a measured byte-cost floor |
| `textwrap` | `core/text` | one width authority, with an explicit width table instead of the locale's |
| `term` + `input` | `core/term`, `core/input` | one escape parser instead of two divergent ones; the frame writer's unbounded queue and condition variable replaced by a sequence watermark |
| `present` | `core/present` | a throttled request always owes a frame; the interval decides when, not whether |
| `fuzzy` | `core/fuzzy` | anchored best-of instead of pure greedy; beginning the candidate scores above beginning a word inside it |
| `overlay`, `theme` | `core/layout`, `kit` | placement separated from drawing, so a hit test a frame later asks the same question |
| widgets living inside its runtime | `headless/` and `kit/` | lifted out, then split down the middle: what a widget does from what it looks like |

agentui's largest package by far is its `runtime` — a whole agent CLI. That is the part
that did not come across, and the reason the port is a fraction of the size: what was
taken is the engine, and what was left is a product.

The clearest example is completion. agentui's version knows about a hidden-file opt-in
spelled `@!`, a shell mode opening on `!`, a completion source enum with an `ai` member,
a truncation flag so that Tab will not infer a common prefix from a capped result set,
and a "drill anchor" protecting one previously accepted directory whose name contains
spaces. All of that is a product's grammar wearing a library's clothes. What is general
is a scorer and a list, and that is what `core/fuzzy` and `headless.Completion`
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

### core

- **`grid`** — styled grapheme cells; a clipped drawing view whose coordinates are
  local, so a widget cannot draw outside its box. Two ways a frame reaches a terminal:
  **`Screen`**, which takes the whole screen, double-buffers, and emits the smallest
  escape stream that turns one frame into the next (with a terminal-side scroll
  shortcut taken only when it beats a measured floor on what the plain diff must cost);
  and **`Inline`**, which draws a block in the terminal's own screen and prints finished
  output above it. A frame is cells and regions that something else writes into — a
  picture, a plot in pixels — which are named by what is in them, so an unchanged one
  is silent, a moved one is erased before it is painted, and one that stopped being
  asked for is taken away. A flush that would change nothing writes nothing, so an idle
  interface is silent on the wire and the cursor keeps blinking.
- **`text`** — grapheme clusters, column measurement, wrapping, truncation, tab
  expansion. One width authority shared by everything that measures or draws, because
  measuring text one way and drawing it another is the cause of every misaligned
  terminal UI. And the other direction: a decoder that reads the escape sequences a
  command wrote into its output back into styled spans, a chunk at a time, because
  output arrives in whatever pieces a read produced and a sequence split down the
  middle must not be printed.
- **`ansi`** — what a control sequence is made of: which bytes are parameters, which
  byte ends one, what an empty field means, where a sequence stops. It exists because
  two packages read sequences for opposite reasons — one reads what a terminal sends,
  the other what a program was handed — and two parsers over one syntax drift.
- **`input`** — an incremental parser for what a terminal sends: CSI and SS3 keys, the
  Kitty keyboard protocol including release and repeat and associated text, SGR mouse
  reporting with movement, bracketed paste, focus, and the two shapes an *answer*
  comes in — operating system commands and device attributes. Events are a sealed
  interface. The introducer of an answer is also the bytes a terminal sends for a
  chord, so a command is recognised only when what follows looks like one. And the
  table that turns a keystroke into the name of what it does, which is what lets a
  binding be several chords long and be written to a file and read back.
- **`term`** — the only package that touches the operating system. Raw mode, the modes
  a session turns on and the reverse order they are put back in, the goroutines reading
  input, and a frame writer with its own goroutine so that a slow terminal cannot stop
  the loop from reading input. It also asks: what colour the terminal draws on and what
  it claims to support, what it calls itself, and which keyboard enhancements
  actually took — in one round trip during startup, ended by a device attributes
  query because that is the only answer every terminal gives. What it says outranks
  the environment, which describes the terminal a shell started in rather than the
  one on the other end of an ssh connection. And it starts the program
  again in place of itself, keeping the terminal, which is the only way to move an
  interface between the alternate screen and the terminal's own.

  It also gives the terminal away and takes it back — an editor, a pager, Ctrl+Z —
  which is the same unwinding as closing a session, done twice, plus the half nothing
  else can do for a caller: the reader comes off the terminal first and goes back on
  last, so a byte typed at a child is a byte this process never took. The window
  title, the bell and a desktop notification are one sequence each, and the title is
  put back on the way out for the same reason a mode is.
- **`clipboard`** — the sequences that carry text to and from the terminal's
  clipboard. The terminal does the copying because over ssh, in a container, or
  through a multiplexer running elsewhere it is the only end of the connection the
  user is at.
- **`present`** — when to draw. Coalescing, throttling, and refusing to draw while the
  terminal is still swallowing the last frame.
- **`fuzzy`** — subsequence ranking, answering in byte offsets because whatever asks is
  about to draw the candidate with the matched characters picked out.
- **`diff`** — what changed between two texts, line by line, and the hunks worth
  showing. Beside `fuzzy` for the same reason: what changed is a fact about two
  strings and has nothing to do with a terminal.
- **`anim`** — easing, a shimmer sweep, a running wave, a transition, a spring and a
  timeline, all counted in ticks rather than measured in wall-clock time, because a
  widget that asks what time it is cannot be stepped by a test or paused by a loop
  that parked. A spring keeps the speed it had when its target moves, which is the
  whole difference from a transition, and steps by the exact solution rather than a
  small step of it — stepping a stiff spring approximately at a frame rate is how one
  turns into an oscillation that grows.
- **`link`** — the URLs in a piece of text, as byte ranges, plus a record of where
  they were drawn so a click can be answered from the same pass that wrote the cells.
  Turning a byte range into the columns it covers is `text`'s, because that is the
  package that already owns the relationship between the two counts.
- **`graphics`** — inline images, and which protocol can be used where. A transmitted
  image is also a `grid.Painter`, which is what puts one in a frame; the two packages
  do not import each other, one says what a region needs and the other happens to be
  able to do it. Kitty's gives
  a program a handle and can therefore be used in a region that redraws; iTerm2's and
  sixel put pixels at the cursor and can only be printed once. Kitty and iTerm2 are
  written; sixel is detected and not produced, because producing it means decoding the
  image and a decoder would be a dependency. PNG only, for the same reason.

- **`layout`** — dividing a region: fixed, flexible, measured and fraction-of-the-whole
  slots, a gap between them, where a child narrower than its slot sits, insets,
  alignment, and the placement a floating layer is clamped into. It hands back views
  and never draws, which is what lets the same rules place a widget, a string, or a
  hole left deliberately empty. A measured slot is asked about the axis being divided
  given the room across the other one, so `Measured` means the same thing in a row and
  in a column — the earlier version could only answer for height, and a measured
  column silently came out zero wide.

### headless

Behaviour with no appearance: scroll position that follows a live log without dragging
a reader who scrolled up, mouse tracking that commits a click on release over the target
that took the press, a generic list, an editor with a kill ring and coalesced undo — in
one line or many, masked or not — a completion offered against a token, and a window
that shows a slice of anything taller than the room it has.

Three of the things that show rows are that list with something added, and saying so
is the design rather than a shortcut: a **tree** is a list of the rows it is showing,
so opening and closing is all it has to do itself; a **table** is a list with an order,
and sorting carries the cursor with the row it was on by sorting a permutation rather
than the rows; a **filter** is a list of what answered a pattern, with where the pattern
is typed left to the caller. **Tabs** are the fourth, and they draw only the pane
showing — the strip of names is appearance.

What decides which of several widgets an event is for is a **container**: a key goes to
the one that has the keyboard, a mouse event to the one it is over, in that widget's own
coordinates, with a press captured until the release. A **form** is that container with
an answer checked when the keyboard leaves a field and the set checked on submission,
and the four fields anything ever asks for — a line of text, one choice, several, and a
yes or no — each binding what it collects to a variable of the caller's own. Each of
them can also be asked and answered in words, for somebody who is not looking at a
grid: the question is the field's, because only the field knows what an answer means,
and reading a line is left to whoever has the reader.

None of it owns a keystroke. A widget names what it can do and answers to the name; an
`input.Keymap` says which keystrokes produce which name, sequences included. That is why
every key can be rebound without replacing anything, and why every action is reachable
from a menu, from a command typed out, or from a test that presses nothing.

And, since a session's output has to be addressable before anything can be asked about
it, a **transcript**: blocks in order, each of a height that follows the width, and the
row each of them starts at. One coordinate space that everything else answers in —
**selection** that survives scrolling and rejoins what the width broke, **search** that
runs off the interface's goroutine and keeps only the newest query, and **sticky
headers** that keep the question on screen while the answer scrolls past. Plus the two
halves of a prompt: **history** that gives back the draft it interrupted, and a
**command registry** ranked by what was typed and by what was used last.

Nothing here decides what any of it looks like. A list draws a row by calling back to
whoever does, which is the one design decision that makes the ring above it optional.
Three things cannot hand their drawing out — a field is generic over what it holds, an
editor lays a selection over text only it knows the shape of, and a completion picks
out the characters a query matched — and those three take a `Look`, which is one value
with a handful of roles in it rather than a style field per part. A theme builds one.

### kit

One set of answers: box and border, label, wrapped paragraph, spinner, progress bar,
scrollbar, help row, table with a sortable header, tab strip, tree, a floating layer
with shading, a transcript view that lays selection and search results over what was
drawn, a command palette that picks out the characters a query matched, a diff, a
dressed form, the same form as a conversation in words, and the three pieces a
streaming interface is actually made of — a composer, a status line, and a printed
message. Plus a semantic palette, `Theme`, whose
names are roles rather than colours and which follows what the terminal said it draws
on, and a glyph set with an ASCII fallback for a terminal whose locale says it cannot
draw the other one.

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

### markdown

A module of its own, and the one the boundary was drawn for: rendering markdown needs
a parser, and a parser is a tree of somebody else's code.

The hard part is not the parser. Every renderer takes a document and gives back a
rendering; a program showing a model's answer has a prefix of one, growing a few words
at a time, and re-rendering the whole of it on every chunk is quadratic in exactly the
case where answers are long. So a stream splits what has arrived into the part that is
certainly finished — published once, never looked at again — and the part that is not,
which is short by construction. Where it cuts is written down, and so is what the rule
costs.

What comes out is `core/text` lines, so wrapping happens where the width is known, and
the drawable form is a `Drawer` and a `Measurer` and nothing else — which is what lets
a document go into a slot, a container or a viewport belonging to a package this module
has never heard of. It does not highlight code: a highlighter is several megabytes of
lexers and a matter of taste, which is the same argument that keeps one appearance out
of the behaviour a widget has, so there is a seam for one and no dependency on one.

### highlight

Source code into styled lines, and a module of its own for the reason markdown is: a
lexer per language and a palette per theme is several megabytes of somebody else's
tree. It is what plugs into markdown's seam, in one line, and neither module knows
the other exists.

Nothing of the highlighter reaches its API. A style is its name, a language is its
name, and what comes back is text — the same boundary markdown keeps around its
parser, and what lets either be replaced without anything above noticing.

### How it is kept honest

- The arch tests above, each with a counter-example.
- Tests state behaviour rather than implementation: what an idle frame must not write,
  what a release away from a button must not do, what a resize must repaint.
- Two ways to drive an interface without being one. A `program.Host` runs it with no
  terminal in sight, which is how most of `examples/streaming` is tested; `ptytest`
  runs the real binary on a real pty, which is how the rest is. The second exists
  because a host can only prove that an interface drew the frame it meant to, and
  what is worth proving about a renderer is what its bytes then do to a terminal.
- The assertions that came with it, `RequireSymmetricModes` above all: every mode a
  session turned on was turned off, and unwound in the reverse of the order it was
  set up. A terminal left in a mode nobody turned off is a terminal the user has to
  close, and nothing short of a real pty can see it happen.

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

1. **Sixel, and mermaid.** Sixel is reported and not written, because producing it
   means decoding an image into pixels and a decoder is the dependency
   `core/graphics` exists without — a caller holding an encoder of its own is told
   the terminal will take what it makes. Mermaid is a renderer for a diagram
   language, which is somebody else's parser again and belongs wherever it lands.
2. **A worked example of the whole surface.** The example is a chat that streams,
   and it now proves the probe, the theme that follows it, the clipboard and the
   glyph fallback. The transcript, selection, search, sticky headers and the command
   palette are proved by their own tests and by `kit`, not by a program anyone can
   run. A second example that puts them together is worth having.
3. **A trackpad scrolling differently from a wheel.** A mouse report now carries
   when it arrived, so the two can be told apart by rate — what is missing is not
   the mechanism but the number. How far a trackpad report should scroll relative
   to a wheel report is a feel decision, the prior art's table points the opposite
   way from the reasoning here, and inventing one without evidence would be worse
   than the current behaviour, which is at least proportional and consistent.

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

  Worth recording what the alternative costs, because grok-build tried the clever
  version and abandoned it. Its note says it computed the reflow from character
  counts and that "edge cases and terminal-specific behaviors made this unreliable",
  so it settled on clearing the screen *and the scrollback* and re-emitting the whole
  transcript from a string the application retains. That is exact on every terminal,
  and it costs two things this library is not willing to pay: the application has to
  keep the entire transcript in memory, which contradicts the whole point of giving
  finished output to the terminal, and it destroys whatever the user had in their
  scrollback before the program started. Approximate is the cheaper mistake.
- **No terminfo.** Colour degrades — `grid.Depth` maps a truecolor value to the 256
  palette, to the sixteen ANSI colours, or to nothing at all, and `term.DetectDepth`
  reads NO_COLOR, COLORTERM and TERM to choose. Everything else is still asked for
  rather than detected. What remains is that an unrecognised TERM is treated as
  truecolor, which is the right bet for the terminals people actually use and the
  wrong one for a genuinely old terminal that does not say so.
- **Handing the terminal to a child needs something to wait on.** Doing it correctly
  means taking the reader off the terminal first, which means a read that can be
  interrupted: on Unix that is a poll over the terminal and a pipe, on Windows a wait
  over the console and an event. Where there is neither — a session whose input is a
  pipe pretending to be a terminal, a platform with no answer at all — handing over
  reports `ErrUnsupported` rather than shipping a child that drops every other
  keystroke. On Windows a console is signalled by records that are not keystrokes, so
  a wait can still fall into a blocking read; the handover waits a moment for the
  reader to park and goes ahead without it, which is where that platform always was.
- **Output is read, not emulated.** The decoder turns the escape sequences a command
  wrote into styled text, and drops the rest — a carriage return is dropped rather
  than obeyed. Output that redrew a line in place therefore reads as the several
  versions of it. Obeying cursor movement and erasure is a terminal emulator, which is
  another product; the decoder is the ten-per-cent of it that is worth having on its
  own.
- **Streaming markdown cuts at a blank line.** A block is published once a line has
  arrived after it that does not begin with a space, and never inside a fenced block
  of code. A list with blank lines between its items is therefore published in pieces,
  which reads the same, and a link written as a reference is published before its
  address arrives, which comes out as the words without the link. Both are the price
  of showing an answer as it is written instead of after it is finished.
- **Windows has no resize events.** The console reports resizing through its own input
  API rather than a signal. A session gets its opening size and nothing after, unless a
  host delivers sizes itself. Everything else is portable.
- **`fuzzy` is not a full alignment search.** It tries the first placement and every
  placement that begins a word, and keeps the best. A better placement in a candidate
  with no word boundary to hint at it can be missed. Palette-sized candidate lists make
  this the right stopping point; a scoring matrix per candidate is what it would take.
- **The scroll shortcut is `Screen`-only.** Inline mode has no equivalent, because a
  block cannot address the region it would scroll.
- **The comparative claims in section 1 are still claims.** There are benchmarks
  now, and they measure this library against itself rather than against another
  one, which is the honest half of the job. Two things they already say, on a
  120×40 screen:

  A frame's cost is dominated by drawing text into cells, not by diffing them.
  Drawing forty rows takes about 200µs and allocates nothing; the diff on top of
  that is tens of microseconds, and a frame that changed nothing costs about the
  same as one that changed a row. "An idle interface is silent" is a claim about
  bytes on the wire, and it is true; it is not a claim about work not done.

  Wrapping allocates heavily — about a thousand allocations for a paragraph —
  because it flattens a line into one unit per grapheme cluster. That is why
  `kit.Paragraph` memoises its wrap, and it is the first place worth looking if
  any of this ever needs to be faster.
- **`Loop.Post` must not be called from the loop's own goroutine while its queue is
  full.** The buffer absorbs a burst, and a component that posted from inside `Draw`
  or `Handle` faster than the loop drains would block the only consumer there is. It
  takes 256 outstanding posts to reach, and `InlineLoop.Print` is the realistic way in.
- **Tagged low, on purpose.** The modules are versioned and built on every push, and
  they are tagged at `v0.0.1` because everything about the public API is still cheap to
  change and should be changed while that is true. A pre-1.0 tag is a promise about
  what will break, not the absence of one.

---

## 7. Provenance and licence

Apache-2.0. The derived parts and their upstream are named in [NOTICE](NOTICE), which
is an obligation and not a courtesy.

[agentui]: https://github.com/minoism/agentui
