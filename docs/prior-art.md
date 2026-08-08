# Prior art: four terminal interfaces, and what to take from them

Status: a survey with decisions attached. It is the sibling of
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
| pi-tui | `@earendil-works/pi-tui` | TypeScript | "Minimal terminal UI framework with differential rendering." |

This was a survey, not an audit: package documentation, exported surfaces, and the
files each project points at when it explains itself. Quotations below are from those
sources. Every claim about *this* repository was checked against the code.

These projects move. A comparison without a date is a claim that stops being true
quietly, which is the mistake this repository has already made once and recorded in
[ROADMAP.md](../ROADMAP.md).

## 1. opentui/keymap answers a limitation this repository has written down

`core/keymap` states its own ceiling:

> A sequence that is a proper prefix of another binding is unreachable: without a
> timer driving lookup, the shorter binding cannot be chosen while the longer one may
> still arrive. The longer sequence therefore takes precedence.

`@opentui/keymap` does not have that ceiling:

> Programmable exact-vs-prefix disambiguation (e.g. `g` vs `gg`) with `runExact`,
> `continueSequence`, `clear`, and deferred `AbortSignal` + `sleep` decisions. Ships a
> Neovim-style timeout resolver.

The idea worth taking is not "add a timeout". It is that **disambiguation is a value
the caller supplies**, and a timeout resolver is one implementation of it.

That shape resolves a deadlock this repository has, and could not otherwise escape.
The dependency graph forbids `runtime` from reaching `interaction` and forbids
`headless` from reaching `runtime`, so **no layer inside the library is allowed to own
the timer**. A resolver does not need to own one: `Pending` reports that an ambiguity
exists and what the choices are, and whoever can see a clock answers. The library
stays out of the business of knowing what time it is.

Two more things are worth taking and are independent of the first:

- **Scoped, priority-ordered layers with fallthrough.** This repository has one map
  per widget kind and no notion of scope, so an application that wants a binding
  active only while a pane has focus writes that condition by hand in `Handle`.
- **Diagnostics.** opentui reports shadowing, unreachable bindings, inactive reasons,
  and runs lint-style analysers over the layer graph. A keymap that can be asked
  *why* a binding never fires is a keymap people can debug; this repository's cannot
  be asked.

**Not taken.** opentui's keymap lists twelve headline capabilities. The other nine are
a pluggable binding language (parsers, expanders, transformers, command resolvers,
field compilers), a registrable schema, reactive matchers with subscription-driven
notifications, and React and Solid bindings.
[§16](architecture.md#16-designs-explicitly-rejected) rejects a generic signals or
observables framework outright, and
[§12 rule 8](architecture.md#12-go-api-rules) asks configuration to stay proportional
to the problem. Three of twelve is the honest share.

## 2. opentui/ssh confirms the shape of a host this repository does not have

> `@opentui/ssh` turns an incoming SSH session into a fully-wired OpenTUI `CliRenderer`
> whose input/output is the SSH channel and whose dimensions track the client's PTY.
> […] the package is **renderer-agnostic**: it depends only on `@opentui/core`, never
> on `@opentui/react` or `@opentui/solid`.

That is the design already sketched for this repository — an SSH channel behind
`program.Host`, in a module of its own, never in `core` — implemented by somebody
else and shown to work. Its file list is a usable checklist: `auth`, `banner`,
`connection`, `keys`, `run-session`, `server`, `safe`, `errors`, `logging`.

What changed here is the cost. When this was first considered, `program.Host` had
sixteen methods and most implementations answered "no" to a dozen of them. It now has
three, with optional capabilities added one method at a time, `EventSource.Err` to
report a transport failure, and a consumer-defined `FrameWriter`. An SSH host is now
a small module rather than a large one.

It also bears on [§13.1 condition 3](architecture.md#131-the-v1-compatibility-boundary),
which needs two independently maintained non-example applications. An SSH server is
one of those, and it is the only candidate here that is.

## 3. pi-tui: two small editor behaviours

**A kill ring.** `headless.Editor.Yank` "puts back the last text cut" — one slot.
pi-tui's is a ring:

> Consecutive kills can accumulate into a single entry. Supports yank (paste most
> recent) and yank-pop (cycle through older entries).

Its `push` takes `prepend` so that a backward deletion accumulates onto the front of
the current entry and a forward deletion onto the back — which is the detail that
makes accumulation feel right rather than merely present. This is behaviour with no
appearance, it belongs in `headless`, and it is small.

**A large paste becomes one atomic thing.** pi-tui collapses a paste past a threshold
into a marker its editor treats as a single segment:

```text
[paste #1 +123 lines]
```

> …within paste markers into single atomic segments. This makes cursor […]

**This repository already has the mechanism.** `headless.Editor` elements and
`text.Mark` exist for exactly this: "A chip in a prompt naming a file is atomic: text
typed into the middle of it…". What is missing is not a capability; it is that nobody
has pointed the capability at `input.Paste`. That makes it a worked example rather
than a library change — which is where product decisions such as the threshold and
the wording of the marker belong anyway.

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
the same argument [ROADMAP](../ROADMAP.md) makes about line-string diffing, and pi-tui
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

This repository can drive a program without a terminal (`core/programtest`) and can
drive one through a real pty (`ptytest`), and neither answers *what the terminal ended
up showing*. One asserts on frames, the other on bytes; a screen-state assertion is a
third thing, and it is the one that reads like the question a test wants to ask.
`core/text.Decoder` already turns SGR and OSC 8 into styled lines, which is a
meaningful part of the distance.

The boundary needs saying at the same time, because
[§16](architecture.md#16-designs-explicitly-rejected) declines a terminal emulator:
decode far enough to assert what a cell shows, never far enough to emulate a cursor,
scrolling region or alternate screen. A test helper that grows a scroll region has
become the product §16 refused.

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

## Summary

| source | adopt | do not import |
| --- | --- | --- |
| opentui/keymap | disambiguation as a caller-supplied resolver; scoped priority layers; diagnostics for shadowed and unreachable bindings | a pluggable binding language, registrable schema, reactive matchers, framework bindings |
| opentui/ssh | an SSH channel behind `program.Host` as its own module, renderer-agnostic | anything that makes `core` know a transport exists |
| pi-tui | a kill ring in `headless.Editor`; a screen-state assertion for tests, built on an emulator small enough to stay a test helper | a paste marker as a library feature — the mechanism is already here and the policy is the application's; line-string diffing and the ANSI-aware string utilities it forces |
| grok-build | a named counter-example for §3.2 | purge-and-re-emit resize |
| agentui | the idea that a detector should report refusals with reasons | product grammar; a second transcript engine; path policy inside a detector |

## Ordered candidates

Ordered by what each is blocked on, not by appetite.

1. **A named counter-example in §3.2.** One paragraph. Turns an unopposed assertion
   into a judgement with the alternative stated.
2. **An SSH host module.** The only candidate that is cheaper now than when it was
   first considered, carries its own caller, and moves a v1 exit condition. Must be
   its own module and must not add a dependency to `core`.
3. **A screen-state assertion for tests.** The gap it fills is real and it has two
   consumers already — the example suites and the pty suite both currently assert on
   bytes because there is nothing better to assert on. It belongs in a test module,
   never in the library, and it needs its stopping point written down before its first
   line: enough to say what a cell shows, never a cursor or a scrolling region.

4. **A kill ring.** Small, behaviour-only, no new boundary. Wants one real call site
   before it is generalised past what `Yank` already does.
5. **Keymap disambiguation, scopes and diagnostics.** The design is clear and the
   deadlock is understood. It needs a real interface that suffers from the prefix
   limitation, because the resolver's shape should be decided by a caller and not by
   this document.
6. **Refusal reporting in `core/link`.** [§7.1](architecture.md#71-a-package-must-earn-its-name)
   wants two consumers for a boundary; there is currently one hypothetical.
7. **A paste-into-chip example.** Belongs in `examples`, not in the library.

## What would change this document

A new reading, with a new date. Also any of these:

- a real interface here that the prefix limitation actually blocks, which would move
  candidate 4 up;
- a second consumer for refusal reporting, which would move candidate 6 from a wish to
  a boundary;
- evidence that purge-and-re-emit resize buys something §3.2 did not weigh, which
  would reopen a rejection rather than merely annotate it.
