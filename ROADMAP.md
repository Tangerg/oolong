# What is missing, and in what order

Read against five prior arts: `agentui` and `grok-build` (the sources this library was
lifted from), and `opentui`, `bubbles`, `lipgloss`, `huh`, `glow` (read for comparison,
not used). It is ordered by what blocks what, not by what is most wanted.

Three findings gate everything else, so they come first.

**Done so far:** all of section 1, and 2.1 and 2.4 of section 2. What each turned out
to mean in practice is recorded under the item. What is left is 2.2, 2.3, 2.5 and
section 3, in the order at the end.

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

### 2.3 Forms, in `headless`

`huh` sits above `bubbles`. Stripped of appearance its core is behaviour:

- **Field** — holds a value, says whether it is valid, answers input.
- **Form** — navigation between fields, and validation across them.
- **Accessor** — binds a field's value to the caller's own variable.

That is `headless`, and `kit` renders it. **No new ring is needed.**

This corrects an earlier misjudgement: `agentui`'s blocking question interaction was
filed as application code. It is an instance of a form. The mechanism is the library's;
which questions to ask is the product's.

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

---

## 3. Components that are missing

Ordered by value. What this library has and the others do not — a transcript, search,
sticky headers, a command registry, history, completion — follows from being built for
streaming output, and is not an accident.

1. **A scrollable container.** `bubbles` has `viewport`, `opentui` has `ScrollBox`.
   There is a scroll position (`headless.Scroll`) and something that draws a bar
   (`kit.Scrollbar`), and nothing that puts arbitrary content in a box and scrolls it.
   `Transcript` is a special case of it.
2. **Select, multi-select, confirm.** `List` highlights a row; nothing collects a choice
   and hands it back. These are a form's fields — see 2.3.
3. **A single-line input.** `Editor` is multi-line and `Composer` wraps it. A one-line
   field with validation, a placeholder and masking is a different thing.
4. **A diff view.** `Theme` carries `Added`, `Removed` and `Context`, and nothing draws a
   diff. Three styles with no consumer.

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

By dependency, not by value:

1. ~~**Blending** (2.4)~~ — done. Depended on nothing new, and proved the probe's
   answer end to end.
2. ~~**Container and focus** (1.1, 1.2)~~ — done, and 1.3 with them.
3. ~~**Marks and edits** (2.1)~~ — done.
4. **Keys** (2.2) — needs an arrival time on a key; otherwise independent.
5. **Components** (3) — after the container, or each one adds another hand-wiring.
6. **Forms** (2.3) — needs keys for navigation and the components above.

The editor's flat buffer is not in this list. It is a separate change, best judged after
3, when the shifting logic is in one place.

### Why 4 is one change and not four

Every widget here owns both halves of its own key handling, so separating them
separates them everywhere at once: `EditorKeys`, `List`, `Completion`, `Scroll`,
`History`, `Commands`, `Stack.Escape` and `Container.Next` all name keystrokes where
they mean commands, and `kit.Help`, `Composer.Hints` and `Dialog.Hints` all read the
same `Binding` back out to draw a hint row. Doing half of it would leave two ways to
bind a key, which is the defect 1.3 was — and this time it would be a defect a caller
could see from outside.
