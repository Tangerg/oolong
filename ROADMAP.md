# What is left, and what is refused

This page states the current position: what is still worth building, and what was evaluated and deliberately
not taken. The analysis rounds that produced it are in Git history.

Oolong was read against `agentui` and `grok-build` — the sources it was lifted from — and against `opentui`,
`bubbles`, `lipgloss`, `huh`, and `glow` for comparison.

## What is left

Ordered by what would be built next. None of these is a gap in the library.

1. **A picture that is not a PNG, and a terminal that only speaks sixel.** `core/graphics` reads a PNG's size
   out of its header, which is why it needs no decoder and therefore no dependency; producing sixel means
   decoding an image into pixels, which needs one. A caller holding an encoder is told the terminal will take
   what it makes, and that is the honest place for the boundary. Mermaid is a renderer for a diagram language,
   which is somebody else's parser again and belongs wherever it lands.
2. **A trackpad scrolling differently from a wheel.** A mouse report carries when it arrived, so the two can
   be told apart by rate. What is missing is not the mechanism but the number: how far a trackpad report
   should scroll relative to a wheel report is a feel decision, the prior art's table points the opposite way
   from the reasoning here, and inventing one without evidence would be worse than the current behaviour,
   which is at least proportional and consistent.
3. **A second worked example.** The one there is streams an answer and proves the probe, the theme, the
   clipboard, and the glyph fallback. Everything added since — the tree, the tabs, the table, the filter, the
   images, the handover — is proved by its own tests and by nothing anybody can run. That is the next thing
   worth doing, and it is a program rather than a library.

Everything else is either built, refused below, or an application's to decide: what a tool call looks like,
what an `@` refers to, how a session is persisted, syntax-aware editing, a shell, process management. Those
are the things a library that meant to be general must not decide.

## What is deliberately not taken

Not "later" — these are decisions. A roadmap that only lists gaps invites filling in the ones already
answered.

- **Application grammar.** What a candidate is, where candidates come from, what accepting one means, what a
  slash command is, what `@` refers to. A library that knew would be a framework for one program.
- **A retry layer, a logger, an observability abstraction.** A library that owns none of these is one that
  fits into a program that already has them.
- **Colour degradation and terminfo.** A colour is the terminal's default or a truecolor value, and optional
  behaviours are asked for rather than detected — a terminal that does not implement a request ignores it.
  This is a decision with a known cost, recorded in the limits section of [DESIGN.md](./DESIGN.md).
- **A widget for everything.** `kit` holds the ones a streaming interface actually needs, and `headless` the
  behaviour worth sharing underneath them. A library whose widget count is its selling point ends up with
  fifty widgets and no layering.
- **A full flexbox engine.** `opentui` vendors Yoga and thousands of its upstream tests. That breaks the
  dependency promise, and terminal layout is not web layout. It does say that anything wanting deeply nested
  layout will outgrow `layout` as it stands.
- **An immutable model loop.** `bubbletea` returns a new model from every update, which copies the component
  tree on every keystroke. One goroutine owning mutable state suits an interface that updates dozens of times
  a second better.

## What the comparison says not to build

These are answered here and should not be reopened as gaps.

- **Cell diffing, against line-string diffing.** `bubbletea`'s standard renderer diffs rendered strings by
  line; this diffs cells. Wide characters, combining marks, and emoji are exactly where the first produces
  damage the second cannot.
- **Compositing that asks the terminal first.** `lipgloss` blends with alpha; nothing there asks what the
  terminal draws on, so a translucent layer over a cell left at the terminal's own colours is a guess.
  `Ground`, `Blend`, and `Fade` are the answer to that, and they cost one round trip at startup.
- **Printed output that belongs to the terminal.** Finished blocks go into the scrollback and are not redrawn,
  with a tail column so a stream need not stop at a line boundary. An inline `bubbletea` program owns every row
  it has ever written.
- **A transcript with selection, search, and sticky headers.** Built here, and built by hand in every program
  that wants it elsewhere.
- **Keys as a table of names.** `bubbles/key.Binding` is a keystroke and a description in one value. Sequences
  are in neither.
- **Focus and press capture in a container.** `bubblezone` bolts mouse hit-testing onto a string renderer after
  the fact; here it falls out of the container knowing where it put its children.
- **A harness that runs the real binary on a real pty**, plus arch tests that fail the build when an import
  points the wrong way. `teatest` drives a model; neither of the others checks that every mode a session
  turned on was turned off.
