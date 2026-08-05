# What is missing, and in what order

Read against five prior arts: `agentui` and `grok-build` (the sources this library was
lifted from), and `opentui`, `bubbles`, `lipgloss`, `huh`, `glow` (read for comparison,
not used). It is ordered by what blocks what, not by what is most wanted.

Three findings gate everything else, so they come first.

**Done:** all of it. What each turned out to mean in practice is recorded under the
item, including the three places the implementation contradicted the analysis. What is
left is at the end, under *What this turned up*.

---

## 1. What blocks the rest

*All three are done. They interlocked exactly as described: the container came first,
focus came with it, and only then did "how is a widget dressed" have one answer.*

### 1.1 Components do not compose

Every widget here is an independent object that knows nothing about any other. A caller
places them by hand with `layout.Rows` and forwards events by hand. `kit.Composer` has
to subtract its own marker width from a mouse position before handing it to the field —
work a parent container would do, if there were one.

`bubbles` nests models inside models; `opentui` has a real tree with parents, children
and layout. Adding components before there is a container multiplies the hand-wiring by
the number of components.

### 1.2 There is no focus

With several widgets on screen, which one gets a key is decided by `Handle` returning
false and the event falling through. That works for a linear arrangement and nothing
else. It is also why `kit.Transcript.Handle` has to be passed a `*headless.Clicks` by
the caller: there is nowhere to keep state that belongs to one interaction.

`bubbles` and `huh` both have explicit focus. So does `opentui`.

### 1.3 There are four ways to dress a widget

`Label` and `Box` are values with fields. `Paragraph` and `Composer` are pointers with
methods. `Palette` has a `Dress` method, `Transcript` a `Dressed` function, and `Editor`
takes a style assigned directly. `lipgloss` has one chained `Style`; `huh` has one
`WithTheme`.

This is inconsistency accumulated by adding one widget at a time. It cannot be resolved
before 1.1, because the answer — a parent passes the look down — needs a parent.

**These three interlock.** Composition needs a container; a container needs focus
routing; only once both exist does "how is a widget dressed" have one answer.

**What the answer turned out to be.** Not a parent passing the look down. A parent was
needed to see the answer, not to carry it: once `Composer` and `Transcript` were things
a container held rather than things a caller wired up, the question "which of these
five style fields does this widget actually let a caller choose?" had one answer —
none of them. Every part of a widget has a fixed role in a look, so a field per part is
a field with one sensible value and a hundred ways to be inconsistent. Every `kit`
widget takes a `Theme`, and a `Glyphs` if it draws furniture, and nothing else.
`Dress`, `Dressed` and the package-level `Rounded` and `Square` are gone.

The exception is text: `Label` and `Paragraph` take a style, because the same label is
a heading in one place and a warning in another, and which it is here is the caller's
to say and nothing a theme can work out.

---

## 2. Abstractions worth taking

Each is placed on the existing ladder. Nothing here needs a new ring.

### 2.1 Marks and edits, in `core/text`

`opentui` has Neovim's extmarks: ranges over text that move with edits, carrying an
identity, a style and arbitrary data, rolled back by undo.

The general form is two types with no idea what they are for:

- **Edit** — a byte range replaced by new text.
- **Mark** — a byte range that survives an edit around it, or is consumed by one.

`headless.Element` becomes a mark with an identity and a kind. A highlight, a diff
region and a spelling error are three more. The editor's `spliceElements` and
`cutElements` collapse into applying one edit to whatever marks exist.

**Prerequisite that this exposes.** A flat byte offset is the general form, and the
editor stores `lines []string` with `(line, col)` positions. Making `Edit` line-aware
would encode the editor's storage choice into the abstraction, which the layering
forbids. The same storage is behind the forty-odd cursor assignment sites, the two
separate mark-shifting routines, and the special case caret affinity needed.

Moving the editor to a flat buffer is its own change and is not proposed here. It should
be re-examined after the marks land, when the shifting logic is in one place and the
change is smaller.

**What it turned out to cost.** Nothing: the prerequisite was not one. `text.Edit` and
`text.Mark` are flat, the editor keeps its lines, and the two meet in `offsetOf` and
`caretAt` — a caret as an offset, an offset as a caret. Neither idea knows about the
other. The shifting logic is now one call in one place, which is what makes the flat
buffer worth re-examining; it is no longer what would make it possible.

### 2.2 Keys: commands and keystrokes are two things

`opentui` and `bubbles` independently put key handling in a package of its own. That is
two pieces of evidence that a struct of one field per command has reached its limit.

The abstraction is not "move `Binding` somewhere". It is to separate:

- **The widget names what it can do** — `delete-word-back`, `select-all`.
- **A keymap says which keystrokes produce which name** — a prefix tree, a pending
  state, a timeout.

Today a widget owns both, which is why a sequence like `g g` cannot be expressed and why
rebinding at run time means replacing a whole struct.

The mechanism belongs in `core/input`; the default maps belong in `headless`, the same
split `Glyphs` has. Sequence timeouts need the arrival time of a key, which is the
change already made for `Mouse.At` extended to `Key`.

**What it turned out to need.** A third piece: `Do`. A widget that names what it can do
has to answer to the name, or the name is only ever a label on a `switch` inside its own
`Handle` — and then a completion driving the list inside it has to hand the list the raw
event, which resolves the same keystroke against the same map twice. `Editor`, `List`,
`Scroll`, `Completion`, `Stack` and `Container` each answer to a name now, which also
makes every one of them reachable from a menu, from a command typed by name, and from a
test that presses nothing.

**What it turned up.** `Backtab`. The legacy sequence and the Kitty protocol report
shift+tab as two different keys, the container was bound to one of them, and walking the
keyboard backwards did not work on half the terminals there are. One keystroke, one
spelling: `Backtab` is gone and `CSI Z` decodes as tab with shift held.

The description a hint row shows turned out not to need a field. An action's name is
what there is to say about it, and words kept beside a name drift from it — so
`Binding.Does` and `Binding.Hidden` are both gone, and hiding a hint is not listing it.

### 2.3 Forms, in `headless`

`huh` sits above `bubbles`. Stripped of appearance its core is behaviour:

- **Field** — holds a value, says whether it is valid, answers input.
- **Form** — navigation between fields, and validation across them.
- **Accessor** — binds a field's value to the caller's own variable.

That is `headless`, and `kit` renders it. **No new ring is needed.**

This corrects an earlier misjudgement: `agentui`'s blocking question interaction was
filed as application code. It is an instance of a form. The mechanism is the library's;
which questions to ask is the product's.

**Where the rendering turned out to have to go.** Into the fields. A field is generic
over what it holds, and `*Select[T]` for an unknown `T` is not something a renderer can
name — so a renderer that drew every kind of field cannot be written down. The look
therefore travels the other way: `kit.Form` turns a theme and a glyph set into a
`headless.Look`, the form hands it to its fields, and each field draws itself. That is
one way to dress a field and it is the form it is in, which is why a single input is a
form of one field rather than a widget with styles on it.

A form turned out to be a `Container` with two things added — an answer checked when the
keyboard leaves it, and the set checked on submission. Which field has the keyboard,
what tab does, and which field a click landed in are the same questions there as
anywhere else, and were already answered.

### 2.4 Blending, in `core/grid`

`lipgloss` composites with alpha. Layers here overwrite: what is drawn on top replaces
the cell, and a dialog's backdrop is a style merged over every cell.

Real blending needs to know the colour underneath, and a cell whose background is
`Default` means the terminal's own — which is the answer the startup probe already
gets. Adding `RGB.Blend` makes that answer load-bearing for compositing rather than only
for choosing a theme.

Smallest of the four, and the one that proves the probe's answer end to end.

**What it turned out to need.** Both of the terminal's colours, not one. A cell left at
the terminal's own is the commonest cell there is, and dimming one needs to know what
its foreground resolves to as well — so the probe asks OSC 10 alongside 11, and
`Background()` on the terminal, the host and the loop became `Ground()`. There are two
compositing operations and not one: `View.Blend` paints a sheet of colour over a
region, and `View.Fade` dissolves what is in one into whatever it is already drawn on.
The second cannot be the first, because the colour to fade toward is different in every
cell. It is what `headless.Pinned.Fade` had been computing with nothing to draw it.

### 2.5 A printed block that does not start at a column

`opentui`'s split scrollback tracks a tail column as well as a published row count,
because printed output does not always stop at a line boundary. `Inline.Print` assumes
every printed block begins at column zero. Streaming output does not arrive on line
boundaries.

**What it turned out to be.** Two methods and two fields. `Append` puts cells on the end
of the row the last one left open — reaching back up over the block to the column it
stopped at — and `Break` ends one. What is appended is handed what is left of the row
and nothing more, because a printed row that wrapped would move the block and the block
has no way to find itself again.

The awkward part was above, not below: the room left is not knowable from another
goroutine, so `InlineLoop.Append` asks the caller with what is left and keeps asking
with whole rows until it says it has finished. Wrapping text into it cannot live in
`core/grid` at all — `core/text` depends on `grid` and not the other way round.

---

## 3. Components that are missing

Ordered by value. What this library has and the others do not — a transcript, search,
sticky headers, a command registry, history, completion — follows from being built for
streaming output, and is not an accident.

*All four are done.*

1. ~~**A scrollable container.**~~ `bubbles` has `viewport`, `opentui` has `ScrollBox`.
   There is a scroll position (`headless.Scroll`) and something that draws a bar
   (`kit.Scrollbar`), and nothing that puts arbitrary content in a box and scrolls it.
   `Transcript` is a special case of it.

   **`headless.Viewport` came to forty lines**, because a view is already a clipped
   window onto a surface: content drawn into one that begins above the box lays itself
   out at its full height and loses what is outside. Nothing has to be taught about
   being scrolled, and a cursor placed off-screen is discarded rather than drawn
   somewhere wrong. `Transcript` is *not* built on it, and that is right: it measures
   incrementally because a session's output is too tall to re-measure every frame, and
   two things with an opinion about one position would fight.
2. ~~**Select, multi-select, confirm.**~~ `List` highlights a row; nothing collects a
   choice and hands it back. These are a form's fields — see 2.3.
3. ~~**A single-line input.**~~ `Editor` is multi-line and `Composer` wraps it. A
   one-line field with validation, a placeholder and masking is a different thing.

   **It turned out not to be a different thing.** The difference is two rules — no line
   breaks go in, and text wider than the box slides sideways instead of wrapping — and
   everything else is the same field: what a cursor is, what selecting means, what undo
   undoes, where a click lands. So it is `Editor.SingleLine` and `Editor.Mask`, and the
   validation is the form's, where the analysis already said it belonged. A separate
   type would have been three hundred lines of the same logic written a second time.
4. ~~**A diff view.**~~ `Theme` carries `Added`, `Removed` and `Context`, and nothing
   draws a diff. Three styles with no consumer.

   **A view needs something to view**, so `core/diff` says what changed as well —
   beside `core/fuzzy`, for the same reason that is there. `kit.Diff` is sized, so a
   change taller than its pane goes in a `Viewport` and needs nothing further.

Deliberately not taken, with reasons: syntax-highlighted code and line numbers belong
wherever markdown ends up; images wait on the frame pipeline; a file picker is product
shaped; timers and stopwatches are application logic rather than components.

---

## 4. What is deliberately not taken

- **A full flexbox engine.** `opentui` vendors Yoga and thousands of its upstream tests.
  That breaks the dependency promise, and terminal layout is not web layout. It does
  say that anything wanting deeply nested layout will outgrow `layout` as it stands.
- **An immutable model loop.** `bubbletea` returns a new model from every update, which
  copies the component tree on every keystroke. One goroutine owning mutable state suits
  an interface that updates dozens of times a second better.

---

## 5. Order

By dependency, not by value. All of it is done, in this order:

1. ~~**Blending** (2.4)~~ — depended on nothing new, and proved the probe's answer end
   to end.
2. ~~**Container and focus** (1.1, 1.2)~~ — and 1.3 with them.
3. ~~**Marks and edits** (2.1)~~
4. ~~**Keys** (2.2)~~ — one change and not four, for the reason below.
5. ~~**Components** (3)~~ — after the container, or each one adds another hand-wiring.
6. ~~**Forms** (2.3)~~ — needs keys for navigation and the components above.
7. ~~**A printed block that does not start at a column** (2.5)~~ — independent of all
   of it, which is why it went whenever there was room.

### Why 4 was one change and not four

Every widget owned both halves of its own key handling, so separating them separated
them everywhere at once: `EditorKeys`, `List`, `Completion`, `Scroll`, `Stack.Escape`
and `Container.Next` all named keystrokes where they meant commands, and `kit.Help`,
`Composer.Hints` and `Dialog.Hints` all read the same `Binding` back out to draw a hint
row. Doing half of it would have left two ways to bind a key, which is the defect 1.3
was — and this time a defect a caller could see from outside.

---

## 6. What this turned up

Not a plan. These are the things the work above put in view, kept here so they are not
rediscovered from scratch.

- **The editor's flat buffer.** Still not proposed, and now cheaper to judge: the
  shifting logic is one call in one place, and the single-line field already ignores the
  wrap. What is left is the forty-odd cursor assignments and the caret affinity, which
  are the reason to want it.
- **A sequence that is a prefix of another cannot fire.** `input.Keymap` says so plainly:
  nothing can wake an interface after a pause, so a chord that might still be the start
  of something longer can only be decided by what comes next. Giving the loop a way to
  be woken at a deadline would fix it, and would also be what an idle timeout, a toast
  that dismisses itself, and a spinner that stops need.
- **Wrapping text into `Inline.Append`.** The primitive is there and `core/grid` cannot
  wrap, because `core/text` depends on it and not the other way round. Whatever ends up
  turning a stream of text into rows belongs above both.
- **`headless.Transcript` is not a `Measurer`,** so it cannot go in a `Viewport`. That is
  deliberate today — it keeps its own position for a good reason — but the two now
  answer overlapping questions, and one of them should probably be named as a special
  case of the other rather than left to look like a coincidence.
