---
title: Prior art
description: Review the terminal UI systems Oolong studies and the design choices it adopts or declines.
contentType: Conceptual
outline: deep
---

# Prior art: six terminal UI families, and what to take from them

Language: English | [简体中文](zh/prior-art.md)

Status: a living source audit with decisions attached. It is the sibling of
[architecture.md §5](architecture.md#5-what-is-taken-from-frontend-and-flutter-systems),
which does the same job for browser and Flutter ideas: separate the responsibility
worth learning from the mechanism that carries it, and say plainly which half is being
declined.

Nothing here is a commitment. Every candidate still has to pass the gates in
[§7.1](architecture.md#71-a-package-must-earn-its-name) and
[§15](architecture.md#15-how-the-architecture-grows), and several of them do not pass
today. A survey that turns into a backlog has stopped being a survey.

## What was read, and how much

| project | read at | language | what it is |
| --- | --- | --- | --- |
| [agentui](https://github.com/minoism/agentui) | `ec0414e`, 2026-07-28 | Go, ~245 files | An agent CLI. Shares ancestry with this repository. |
| grok-build | `780d138`, 2026-08-03 | Rust, ~2,500 files | Shipping product; `xai-ratatui-*` crates are its terminal layer. |
| [opentui](https://github.com/anomalyco/opentui) | `b55f125`, 2026-08-05 | TypeScript + Zig | A terminal UI platform with React, Solid, SSH and 3D packages. |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | `6fb1f47`, 2026-08-04 | Go | The Charm runtime and renderer. |
| [bubbles](https://github.com/charmbracelet/bubbles) | `8cea431`, 2026-08-04 | Go | Charm's behaviour-oriented component catalogue. |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | `5696b28`, 2026-07-20 | Go | Charm's string styling and layout layer. |
| pi-tui | `7df73a00c`, 2026-07-24 | TypeScript | "Minimal terminal UI framework with differential rendering." |
| [Codex](https://github.com/openai/codex) | `e4e040881`, 2026-08-03 | Rust | An agent CLI whose TUI is implemented over Ratatui. |

The first pass was a survey. The 2026-08-09 pass is a source audit of the relevant
runtime, renderer, keymap, editor, completion and terminal-lifecycle implementations.
Quotations below are from those sources. Every claim about *this* repository was
checked against the code.

These projects move. A comparison without a date is a claim that stops being true
quietly, which is the mistake this repository has already made once and recorded in
[ROADMAP.md](https://github.com/Tangerg/oolong/blob/main/ROADMAP.md).

## The 2026-08-09 executable decision

This pass selected and completed three vertical slices. They are deliberately small
enough to land independently and complete enough to be useful when each batch ends.

| slice | responsibility taken | boundary kept |
| --- | --- | --- |
| exact-prefix key sequences | `keymap.Matcher` owns one reader's sequence and invokes a caller-supplied resolver; `Runtime.After` is the standard owner-goroutine resolver | `keymap` does not import `program`, own a clock, or grow a second focus tree |
| cursor shape and blink | cursor appearance is part of `grid`'s committed frame state and is diffed like position and visibility | editors may choose an appearance, but terminal escape state does not enter `headless` behaviour |
| native task progress | `term` owns OSC 9;4 encoding, keepalive and restoration; `program.Session` exposes one optional host capability | it remains distinct from the drawn `kit.Progress`, and application task policy stays downstream |

Each slice has unit tests at its owning layer, an end-to-end consumer, idle-wire tests
where applicable, and lifecycle restoration tests. `examples/keys` consumes sequence
resolution, `Editor` publishes cursor appearance through a frame, and `examples/agent`
uses native progress for a real task lifetime. A slice is not complete when only its
public type exists.

The pass also closes a false gap in the first survey. `Container` and `Stack` already
form the scope system: the focused child is offered an event first, a declined event
falls through to its parent, and the committed frame owns pointer identity and
geometry. A central keymap-layer registry would duplicate that tree and create two
answers to which scope owns an event. It is therefore rejected, not deferred.

## 1. opentui/keymap answers a limitation this repository has written down

Before this slice, `core/keymap` stated its own ceiling:

> A sequence that is a proper prefix of another binding is unreachable: without a
> timer driving lookup, the shorter binding cannot be chosen while the longer one may
> still arrive. The longer sequence therefore takes precedence.

`@opentui/keymap` does not have that ceiling:

> Programmable exact-vs-prefix disambiguation (e.g. `g` vs `gg`) with `runExact`,
> `continueSequence`, `clear`, and deferred `AbortSignal` + `sleep` decisions. Ships a
> Neovim-style timeout resolver.

The idea worth taking is not "add a timeout". It is that **disambiguation is a policy
the caller supplies**, and a timeout resolver is one implementation of it.

That shape resolves a dependency problem without reversing an edge. The dependency
graph forbids `program` from reaching `keymap` through components and forbids
`headless` from reaching `program`, so **the matcher cannot own a runtime timer**.
Instead a `Map` accepts a resolver with the same shape as `Runtime.After`. The matcher
hands it a cancellable exact action; the runtime schedules that action back onto the
interface owner. `keymap` still knows neither the runtime nor a goroutine.

One further lesson is taken in a smaller form: sequence state belongs to a behavioural
object. The old `Pending` value only exposed storage while thirteen components repeated
the same lookup/dispatch procedure. `Matcher` owns advancement, cancellation and
dispatch, so there is one implementation of reading a map.

**Scoped layers and layer diagnostics are not taken.** OpenTUI needs a registry because
its bindings can exist independently of renderable ownership. Oolong's maps are owned
by widgets inside the component tree. `Container` and `Stack` already provide
focus-within, priority and fallthrough, and their frame transaction already prevents
input geometry from disagreeing with presentation. Adding a layer graph would make
focus and key scope capable of contradicting one another. Diagnostics for a graph we
do not have would be an abstraction built to justify another abstraction.

**Not taken.** opentui's keymap lists twelve headline capabilities. The rest are
a pluggable binding language (parsers, expanders, transformers, command resolvers,
field compilers), a registrable schema, reactive matchers with subscription-driven
notifications, and React and Solid bindings.
[§16](architecture.md#16-designs-explicitly-rejected) rejects a generic signals or
observables framework outright, and
[§12 rule 8](architecture.md#12-go-api-rules) asks configuration to stay proportional
to the problem. The caller-supplied resolver and stateful matcher are the honest share.

## 2. opentui/ssh confirms the shape of the host now implemented here

> `@opentui/ssh` turns an incoming SSH session into a fully-wired OpenTUI `CliRenderer`
> whose input/output is the SSH channel and whose dimensions track the client's PTY.
> […] the package is **renderer-agnostic**: it depends only on `@opentui/core`, never
> on `@opentui/react` or `@opentui/solid`.

That shape is now implemented here: an SSH channel behind `program.Host`, in a
module of its own, never in `core`. The application and `charm.land/ssh` keep auth,
banner, listening, host keys, connection policy, logging and exit status. Oolong
owns only the accepted PTY's input decoding, window stream, frame writer and
terminal modes. This is the narrower boundary implied by the checklist rather than
a second SSH server implementation.

What changed here is the cost. When this was first considered, `program.Host` had
sixteen methods and most implementations answered "no" to a dozen of them. It now has
three, with optional capabilities added one method at a time, `EventSource.Err` to
report a transport failure, and a consumer-defined `FrameWriter`. An SSH host is now
a small module rather than a large one.

It also bears on [§13.1 condition 3](architecture.md#131-the-v1-compatibility-boundary),
which needs two independently maintained non-example applications. An SSH server is
one of those, and it is the only candidate here that is.

## 3. pi-tui: two small editor behaviours

**A kill ring, now implemented.** `headless.Editor.Yank` originally "put back the last
text cut" from one slot. It now owns a bounded ring with the behavior observed in
pi-tui:

> Consecutive kills can accumulate into a single entry. Supports yank (paste most
> recent) and yank-pop (cycle through older entries).

The private ring takes `prepend` so that a backward deletion accumulates onto the
front of the current entry and a forward deletion onto the back — which is the detail
that makes accumulation feel right rather than merely present. `YankPop` replaces
only an immediately preceding yank, and any intervening semantic action ends that
sequence. The ring owns its strings and retains at most sixteen entries.

**A large paste becomes one atomic thing.** pi-tui collapses a paste past a threshold
into a marker its editor treats as a single segment:

```text
[paste #1 +123 lines]
```

> …within paste markers into single atomic segments. This makes cursor […]

**This repository already had the mechanism; the worked example now exists.**
`headless.Editor` elements and `text.Mark` provide the atomic behavior, while
[`examples/composer`](https://github.com/Tangerg/oolong/tree/main/examples/composer) points it at `input.Paste`. The example
owns the threshold, marker wording, and original pasted bytes. None became library
policy, which is the boundary this section argued for.

### pi-tui's differential rendering, which is the interesting disagreement

pi-tui renders into an array of ANSI-styled **strings** and diffs them by **line**:

> 1. **First Render**: Output all lines without clearing scrollback
> 2. **Width Changed or Change Above Viewport**: Clear screen and full re-render
> 3. **Normal Update**: Move cursor to first changed line, clear to end, render changed
>    lines

Three of its properties are worth stating plainly, because two of them favour this
repository and the third does not.

**The unit decides the utilities.** Because a line is a styled string, pi-tui needs
`visibleWidth`, `truncateToWidth` and `wrapTextWithAnsi` — a whole family of functions
whose job is to reason about text with escape sequences buried in it. Drawing into
cells means never having that family: a wide grapheme, a hyperlink and a painted image
region are structured values rather than bytes that have to be parsed back out. This is
the same argument [ROADMAP](https://github.com/Tangerg/oolong/blob/main/ROADMAP.md) makes about line-string diffing, and pi-tui
is a current example of the other side of it.

**Strategy 3 is coarser than it sounds.** "Move to the first changed line, clear to
end, render" rewrites every row below the first change. Two changes far apart cost the
whole distance between them. A cell diff writes two regions.

**Strategy 2 detects something this repository makes unrepresentable.** pi-tui tracks
the previous viewport top and forces a full redraw when the first change is above it:

> Differential rendering can only touch what was actually visible. If the first changed
> line is above the previous viewport, we need a full redraw.

That is a real hazard for an inline interface — rows that have scrolled into scrollback
cannot be addressed again, so a diff aimed at them writes somewhere else. `grid.Inline`
cannot reach that state, because the block's height is not a size:

> The height is a ceiling rather than a size: it is what the terminal can spare, and
> the block takes as much of it as the interface draws into.

A block that never exceeds what the terminal can show has no top to lose. Bounding the
thing is a better answer than detecting the overflow, and it is the same move §3.2
makes about memory.

**Where pi-tui's strategy buys something this repository has not.** Clearing from the
first change to the end is **self-healing**. A cell diff is correct only while the
program's model of the screen is correct, and anything else that writes to the terminal
— a stray log line, a subprocess, a notification — desynchronises it silently, after
which every frame is wrong until something forces a repaint. This repository forces one
at the moments it knows about: resize, handover, a colour-depth change. It has no
answer for a write it never saw. pi-tui repairs itself from the first changed line
downward on every frame, and pays for that in bytes.

That is an asymmetry, not a defect, and it is recorded here rather than argued away.
Whether it is worth an answer depends on evidence that unknown writes happen to real
programs, which nobody here has.

**What is actually worth taking is not the rendering.** pi-tui's test terminal is an
emulator:

> `VirtualTerminal` - For testing (uses `@xterm/headless`)

This repository could already drive a program without a terminal (`core/programtest`)
and through a real pty (`ptytest`), but neither answered *what the terminal ended up
showing*. One asserted on frames, the other on bytes. `ptytest.Screen` now supplies the
third shape: it incrementally applies the cell text, movement, erasure, bounded scroll,
SGR, OSC and mode syntax emitted by the two Oolong renderers, and exposes fixed-size
cell text for assertions. It reuses `core/ansi` and `core/text` rather than growing a
second escape scanner or grapheme-width implementation.

The boundary needs saying at the same time, because
[§16](architecture.md#16-designs-explicitly-rejected) declines a terminal emulator.
The model keeps an internal write position and margins only because renderer output
uses them to address cells; neither is public terminal state. It does not answer
queries, decode input, own alternate buffers, preserve scrollback, expose cursor
visibility, or interpret device-control painters. Unsupported complete sequences fail
explicitly. That is enough to say what cell text a renderer left and deliberately not
enough to run an arbitrary terminal program.

## 4. grok-build: a counter-example, which is more useful than a borrowing

`xai-ratatui-inline` exports two resize strategies:

```rust
resize_purge_rerender,
resize_viewport_height,
```

[§3.2](architecture.md#32-the-active-interface-must-stay-bounded) rejects the first by
name:

> A resize algorithm that clears scrollback and re-emits retained history is also
> incompatible: it trades the user's terminal history and unbounded application memory
> for geometric certainty.

A shipping product took the other road. That does not change the decision — the reason
still holds, and it is the reason this repository can promise a bounded live interface
at all — but the decision is currently asserted with nobody arguing against it. A
rejection with a named counter-example is a stronger thing than a rejection alone, and
this document is where the counter-example is recorded so that §3.2 can point at it.

Everything else in that crate has a counterpart here: `emit_to_scrollback`,
`split_into_line_segments`, `LinkSpan`, `with_synchronized_output`.

## 5. agentui is the application, not a library to borrow from

Its own package documentation says so: `btw` "models Grok's correlated /btw
side-question lifecycle", `catalog` "models Grok's settings and extension browsers".
`goal`, `todo`, `tasks`, `workflow` and `session` are the same kind of thing.
[§5.3](architecture.md#53-the-component-ladder) puts all of it in the application:
shared components may provide the dialog, list and editor that express a product
grammar, and must not name the grammar.

Its `scrollback` package is a Go port of xai-grok-pager's engine and covers the same
ground as `headless.Transcript`.

One package has a general core. `media.Inspect(source, policy)` finds media references
in text **without loading them**, returning attachments with byte spans and kinds, and
**rejections carrying the reason each candidate was refused**. It is fuzzed. Two
observations follow, and only one of them is about a feature:

- `core/link` "finds the things in a piece of text that point somewhere" and reports
  only what it found. It cannot say **why** something that looks like a link was not
  treated as one. A detector that can explain a refusal is one a person can debug and
  a program can report on.
- The path policy — refusing a reference outside a workspace — is a security decision
  about a particular application. agentui draws that line at a `policy` parameter, and
  it is the right line: it does not belong in a package whose job is detection.

## 6. The component inventory

The sections above compare mechanisms. This one compares catalogues, because "what can
I build without writing it myself" is a fair question to ask a component library and it
has a countable answer.

| | components | notes |
| --- | --- | --- |
| this repository | 25 drawn (`kit`) over ~70 behaviour types (`headless`) | plus `Theme`, `Glyphs`, `Border`, `Cell`, `Column`, `LineNumbers`, `TableLayout`, `Scrim`, `Printer` as supporting values |
| opentui | 20 renderables | `ASCIIFont`, `Box`, `Code`, `Diff`, `EditBuffer`, `FrameBuffer`, `Image`, `Input`, `LineNumber`, `Markdown`, `ScrollBar`, `ScrollBox`, `Select`, `Slider`, `TabSelect`, `Text`, `TextNode`, `Textarea`, `TextTable`, `TimeToFirstDraw` |
| bubbles | 14 | `cursor`, `filepicker`, `help`, `key`, `list`, `paginator`, `progress`, `spinner`, `stopwatch`, `table`, `textarea`, `textinput`, `timer`, `viewport` |
| pi-tui | 12 | `box`, `cancellable-loader`, `editor`, `image`, `input`, `loader`, `markdown`, `select-list`, `settings-list`, `spacer`, `text`, `truncated-text` |
| agentui | 7 interface packages | `overlay`, `palette`, `completion`, `history`, `search`, `todo`, `tasks` — the last two are product grammar |

Counting is the least interesting thing about that table. What follows is the part that
is not already covered by something here under a different name.

### A. General behaviour, now implemented

Each of these was missing outright when the survey was written, is behaviour with an
appearance rather than policy, and has at least two implementations among the projects
read — which is what [§7.1](architecture.md#71-a-package-must-earn-its-name) asks for.
They now form one completed vertical slice rather than a permanent catalogue wish.

| capability | who has it | implementation here |
| --- | --- | --- |
| **A bounded numeric value** | opentui `Slider` | `headless.Slider` owns bounds, value ownership, keys and committed drag geometry; `kit.Slider` is its appearance. `Progress` remains the distinct read-only proportion. |
| **Line numbers** | opentui `LineNumberRenderable` | `text.Row` carries logical-line provenance, `headless.RowGutter` is the shared seam, and `kit.LineNumbers` dresses both editors and code blocks. |
| **A code block** | opentui `Code` | `kit.Code` assembles copied `text.Line` values, wrapping, copy rows and an optional gutter without depending on the optional `highlight` module. |
| **Columns sized from their content** | opentui `TextTable` | `kit.Cell` keeps preferred width and painting together; `Column.Size: layout.Measured(0, 0)` measures the widest title or cell, and `TableLayout` reuses the result for a whole frame. |
| **A settings list** | pi-tui `settings-list`, agentui `catalog` | `headless.Settings` adds selected-value actions to `List`; `kit.Settings` supplies the fitted label/value rows while application data and mutation stay downstream. |

### B. Real product assemblies whose boundary remains downstream

The second audit found that the earlier "one shared scope question" was the wrong
abstraction. Scope already exists in the component tree; the three items below are
application policy attached to ordinary reusable mechanisms.

| assembly | who has it | boundary here |
| --- | --- | --- |
| **A scoped command palette** | agentui `palette` (`Scope`, `Predicate`, `Registry`) | A pane owns its `Commands` and its palette in the same subtree. `Container`/`Stack` decide which subtree receives input; a second scope registry is neither needed nor allowed. |
| **Asynchronous completion sources** | agentui and pi-tui file, shell and reference providers | `Completion.Offer(token, candidates)` remains the presentation seam. The application owns I/O, cancellation and validating the token/editor snapshot before offering a result. |
| **A file picker** | bubbles `filepicker` | `Tree`, `Editor`, `Completion`, `Scroll` and `Selection` already provide the mechanics. Filesystem roots, hidden-file rules and traversal permissions are application security policy, not a shared widget. |

`Commands[T]` therefore indexes only searchable command descriptions and an opaque
caller-owned value. It does not prescribe an argument shape, store an execution
callback of its own type, or parse a slash-prefixed product language; the application
chooses `T` and its input grammar. This keeps registry metadata and application meaning
in one registration without moving either execution policy or syntax into `headless`.

`examples/composer` and `examples/agent` are two consumers of `Completion.Offer` and
can produce their application-owned candidates directly. A generic `Source` would
make the component start I/O, import cancellation policy and become a second owner of
editor state. Both agentui's generation checks and pi-tui's
text/line/column/request-id checks are evidence for keeping that validation beside the
source, not for moving the source into the component.

### C. Count, not capability

Adding these would raise a number and change nothing anyone can do.

| missing | who has it | why not |
| --- | --- | --- |
| Paginator | bubbles | A "3 / 7" indicator. |
| Timer, stopwatch | bubbles | A `time.Ticker` and a format string. |
| ASCII banner font | opentui `ASCIIFont` | Decoration. |
| Spacer | pi-tui | `layout.Flex(1)`. |
| Truncated text | pi-tui | `kit.Label` with `Ellipsis`. |
| ScrollBox, TabSelect, Input, Loader | opentui, pi-tui | `Viewport` with `Scroll`, `Tabs`, `Editor`, `Spinner`. |

### The tension this section is under

"As many components as possible" is a goal, and it is not this repository's. §15 says
no phase creates an unused framework, §7.1 wants two real consumers before a boundary,
and [§16](architecture.md#16-designs-explicitly-rejected) already declines timers and
stopwatches added for the count and a file picker with a filesystem policy inside it.

Those rules and a rich catalogue are not actually opposed, provided the split is kept:
**behaviour general enough to have two consumers goes in `headless` and `kit`; the rest
is a worked example.** A file picker's tree, filter and selection are the first kind.
Its policy is the second. A settings list is the first kind. What the settings *are* is
the second. Group A was entirely the first kind, which is why it could be implemented
without importing product grammar.

The sequence was A first, because five components that could not be built at all were
a larger gain than any number of aliases for things that could. With A complete, B is
now a recorded application boundary rather than a framework backlog. C remains
demand-driven and, where it is merely product assembly, belongs in an example.

## 7. Charm: borrow terminal contracts, not the execution model

Bubble Tea v2 makes two terminal properties explicit in every `View`:

- `Cursor` carries position, shape and blinking;
- `ProgressBar` carries normal, error, indeterminate and warning states and a bounded
  percentage.

OpenTUI independently treats cursor style as renderer state, and pi-tui independently
uses OSC 9;4 to tell a terminal window that work is active. These are not catalogue
features. They are presentation state outside the cell grid, and the terminal keeps
them after a frame unless the session deliberately changes or restores them. That is
why the executable decision adopts both at the renderer/session boundary.

The split between the two kinds of progress matters. `kit.Progress` draws a proportion
inside the interface and belongs to layout and theme. Native progress belongs to the
window or taskbar, remains useful while the window is obscured, and must be cleared on
handover and close. One cannot implement the other, so this is not a duplicate API.

Pi-tui's later keepalive fix exposes a lifecycle detail an encoder alone would miss:
some terminals expire OSC 9;4 while a task is still active. Oolong therefore starts a
ticker only for active native progress, pauses it before taking the handover watermark,
restates the latest value after reacquisition, and owns no timer at all while progress
is clear. A keepalive is session infrastructure, not an application timer.

Three larger Charm choices are deliberately not imported:

- Bubble Tea's `Model -> (Model, Cmd)` loop is an immutable-effect vocabulary. Oolong's
  owner goroutine, `Dispatcher`, bounded `ByteIngress` and concrete capability values
  already provide explicit ownership without allocating a universal message/effect
  language. Adding `Cmd` would create a second concurrency model.
- Lip Gloss styles ANSI strings and consequently needs ANSI-aware width, truncation,
  joining and layer placement. Oolong keeps graphemes, styles, links and painted
  regions structured until the final encoder. Moving back to styled strings would
  discard information and then pay to recover it.
- Bubbles' file picker owns filesystem traversal policy. Oolong keeps the reusable
  interaction mechanics and leaves filesystem authority to the application, as
  section 6 records.

Bubble Tea's one-shot `Tick` does point to one missing low-level convenience. The
keymap resolver needs exactly one cancellable callback on the interface owner, so
`Runtime.After` is adopted for that concrete consumer. Timer and stopwatch widgets
remain rejected: scheduling work and choosing a product display for time are different
responsibilities.

## 8. Codex: absorb terminal behaviour, not application grammar

Codex is useful here because it is a large agent product whose terminal code has had
to survive narrow windows, several colour depths, remote sessions and terminals with
different keyboard protocols. Its command palette, model picker, approval wording and
agent status vocabulary remain product grammar. Six lower-level responsibilities do
not.

**Markdown tables retain structure until width is known.** Codex's
`markdown_render.rs` keeps styled cells and classifies columns before choosing an
aligned grid or key/value records. Oolong now makes the same decision at the right
boundary: `markdown.Block` retains table cells, alignments and styles, then allocates
readable columns in `Measure`/`Draw`; a table that cannot remain scannable becomes
labeled records. Parsing no longer freezes one wide textual rendering that a later
layout can only truncate.

**Diff content wraps; it is not discarded.** Codex reserves a gutter, wraps the
remaining styled spans and gives continuation rows an empty gutter. `kit.Diff` now
uses one pure width-aware layout for both measurement and drawing. Line numbers yield
when they would starve content, while continuation rows preserve the sign, background
and indentation. There is no ellipsis path that silently removes a proposed change.

**Keyboard enhancement is negotiated as features.** Codex handles Kitty keyboard
flags, `modifyOtherKeys`, terminal-specific exclusions and symmetric restoration.
Oolong's transport-neutral form is `input.KeyboardFeatures`: `term.Config.Modes`
derives the exact requested flags from the environment of the terminal being driven,
and SSH uses the client's PTY environment rather than the server process. The input
package names decoded capabilities; `term` alone owns escape sequences and
compatibility decisions.

**A remote clipboard belongs to the client.** Codex routes copy through the terminal
under SSH, bounds OSC 52 input at 100,000 bytes and treats tmux as a distinct forwarding
case. `clipboard.Channel` now owns that protocol state in Oolong: encoding, tmux
passthrough, one outstanding read, answer correlation and expiry. The local terminal
and SSH host consume the same channel, so a remote OSC answer becomes `input.Paste`
only after that session requested it. Native desktop clipboard packages and WSL
PowerShell fallbacks are not imported: they are application/platform integration, not
the terminal protocol shared by both hosts.

**The surrounding terminal participates in appearance.** Codex resolves diff
backgrounds against theme and colour level instead of assuming truecolor on one fixed
background. Oolong keeps `Dark` and `Light` as explicit palettes, while `Suited` leaves
body text on the terminal's own foreground and derives neutral surfaces, lines,
selection and scrim from the reported ground. Semantic colours stay stable, and
`grid.Depth` reduces them only in the final encoder. Components therefore have one
theme vocabulary, not one palette per terminal depth.

**Visual regression is selective and dimensional.** Codex's snapshots concentrate on
narrow/wide tables, wrapped diffs and representative full screens. Oolong now composes
`programtest` with `ptytest.Screen` in `examples/internal/visualtest`: the complex agent
review is checked at 44 and 90 columns, and truecolor, 256, 16 and no-colour runs must
produce identical text geometry while using the right encoding family. This is not a
blanket snapshot policy. State transitions still use behavioural assertions; goldens
guard the few layouts whose relationships are the behaviour.

## Summary

| source | adopt | do not import |
| --- | --- | --- |
| opentui/keymap | exact-prefix disambiguation as a caller-supplied resolver; one stateful matcher per reader | a second scope tree, layer registry, binding language, reactive matchers, framework bindings |
| opentui/ssh | an SSH channel behind `program.Host` as its own module, renderer-agnostic | anything that makes `core` know a transport exists |
| pi-tui | the implemented kill ring in `headless.Editor`; the fixed-size `ptytest.Screen` assertion model | a paste marker as a library feature — the mechanism is already here and the policy is the application's; line-string diffing and the ANSI-aware string utilities it forces |
| grok-build | a named counter-example for §3.2 | purge-and-re-emit resize |
| agentui | the idea that a detector should report refusals with reasons | product grammar; a second transcript engine; path policy inside a detector |
| Charm | cursor shape/blink, native task progress and one-shot owner scheduling | `Cmd`, styled-string rendering, filesystem policy inside a widget |
| Codex | responsive semantic tables, non-truncating diffs, keyboard feature negotiation, client clipboard transport, ground-fitted themes and selective dimensional visual tests | agent commands and status grammar, native desktop clipboard policy, a second renderer or Ratatui-shaped widget model |

## Ordered candidates

Ordered by what each is blocked on, not by appetite.

1. **A named counter-example in §3.2.** One paragraph. Turns an unopposed assertion
   into a judgement with the alternative stated.
2. **An SSH host module: completed.** It accepts an already-authorized
   `charm.land/ssh` session, runs the ordinary program contract, tracks the newest
   valid PTY geometry without blocking the SSH request loop, and owns terminal
   modes and frame settlement for the call. Its dependency is isolated in the
   `ssh` module; `core` gains no SSH branch or dependency.
3. **A screen-state assertion for tests: completed.** `ptytest.Screen` stays in the
   harness module and reuses only terminal-neutral core primitives. Its stopping point
   is enforced by behavior: fixed cell text in; terminal queries, input, buffer
   ownership and arbitrary device-control output out.
4. **[Group A](#a-general-behaviour-now-implemented): completed.** The five
   capabilities landed as vertical slices: bounded value; shared row provenance,
   line numbers and code; then content-fitted cells and a settings list. The settings
   component routes actions but deliberately does not introduce a scope system.
5. **A kill ring: completed.** It remains private behavior inside `Editor`: bounded
   storage, directional accumulation, `Yank`, and immediately consecutive `YankPop`,
   with no new package or public storage abstraction.
6. **Exact-prefix key sequences, cursor appearance and native progress: completed.**
   The three executable slices landed in that order. The first removed `Pending` and
   the repeated lookup/dispatch procedure from every component; the second made
   cursor appearance committed and diffed frame state; the third made native progress
   a keepalive-aware session capability that is restored across handover and cleared
   on close.
7. **The Codex-derived terminal slices: completed.** Markdown tables and diffs now
   resolve against width without discarding content; keyboard and clipboard protocols
   are explicit host capabilities; `Suited` follows the terminal ground; representative
   screens are guarded across width and colour depth. Each responsibility landed at
   its existing owning layer rather than behind a Codex-shaped facade.
8. **Refusal reporting in `core/link`.** [§7.1](architecture.md#71-a-package-must-earn-its-name)
   wants two consumers for a boundary; there is currently one hypothetical.
9. **A paste-into-chip example: completed.** `examples/composer` owns the threshold,
   label and retained source while `headless.Editor` owns only atomic editing.

## What would change this document

A new reading, with a new date. Also any of these:

- evidence that component-tree routing cannot express a real key scope, which would
  reopen the rejected layer registry rather than quietly adding one;
- a second consumer for refusal reporting, which would move candidate 7 from a wish to
  a boundary;
- evidence that purge-and-re-emit resize buys something §3.2 did not weigh, which
  would reopen a rejection rather than merely annotate it.
