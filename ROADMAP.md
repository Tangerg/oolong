# What is missing, and in what order

Read against five prior arts: `agentui` and `grok-build` (the sources this library was
lifted from), and `opentui`, `bubbles`, `lipgloss`, `huh`, `glow` (read for comparison,
not used). It is ordered by what blocks what, not by what is most wanted.

Three findings gate everything else, so they come first.

**Done:** all of it. What each turned out to mean in practice is recorded under the
item, including the three places the implementation contradicted the analysis.

With that list finished, the same reading was done again. Sections 6 and 7 are the
result: what the work itself put in view, and what was still missing when this was
held against what a Go program would otherwise be built on.

**Section 7 is done as well**, in the order it gave, and what each item turned out to
mean is recorded under it. Section 8 is what that round turned up.

---

## 1. What blocks the rest

*All three are done. They interlocked exactly as described: the container came first,
focus came with it, and only then did "how is a widget dressed" have one answer.*

### 1.1 Components do not compose

Every widget here is an independent object that knows nothing about any other. A caller
places them by hand with a downward `layout.Flow` and forwards events by hand. `kit.Composer` has
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

The mechanism belongs in `core/keymap`; `core/input` stops at decoded keystrokes and
their serializable chord representation. The default maps belong in `headless`, the
same split `Glyphs` has. Sequence timeouts need the arrival time of a key, which is the
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
`Background()` became `Ground()` on the terminal and its host capability, and is exposed
through `Runtime.Environment()`. There are two
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
goroutine, so `InlineRuntime.Append` asks the caller with what is left and keeps asking
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
- **A sequence that is a prefix of another cannot fire.** `keymap.Map` says so plainly:
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

---

## 7. Read against them again

The first list was drawn against five prior arts. With it finished, here is the same
reading, held this time against the two a Go program would otherwise be built on —
`opentui`, and the `bubbletea` family taken whole: `bubbles`, `lipgloss`, `glamour`,
`huh`, `harmonica`, `bubblezone`, `wish`, and the `x/` packages beside them.

Ordered by what blocks what. Section 4 already says what is refused on purpose, and
nothing here contradicts it.

*All of it is done except the one thing it names as waiting on something else: images
in a frame, which is still waiting on the same thing.*

### 7.1 What stops an interface being built on this today

1. **Text that arrives with escape sequences in it.** An interface that runs commands
   is handed their output, and their output is coloured. A cell drops control
   characters at the boundary — deliberately, as a trust boundary — and nothing turns
   `ESC [ 31 m` into a red cell. Every caller has to strip the colour or write the
   decoder again.

   Two things, and the difference between them is a factor of ten. **A one-way SGR
   decoder** — escape sequences to styled spans — is small, and the parameter parser it
   needs already exists in `core/input`. **A terminal emulator** — cursor movement,
   erasure, an alternate screen, so that an interactive child can live in a pane — is
   another product's worth of work and is a decision rather than a gap. `charm` has
   both, in `x/cellbuf` and `x/vt`; `opentui` decodes into its own buffer.

   The first one is the largest thing missing for the price.

   **What it turned out to need.** A third package, and it is the smaller half.
   `core/ansi` is what a sequence is made of — which bytes are parameters, which byte
   ends one, what an empty field means, where a sequence stops — and `core/input`
   reads its own reports through it now. Two parsers over one syntax drift, which is
   what the parameter parser said about itself the day it was written; this is that
   sentence acted on.

   The decoder itself is `core/text.Decoder`, and it is a decoder rather than a
   function because of what the library is for. Output arrives in whatever pieces a
   read produced: the style in force has to carry from one chunk to the next, a
   sequence split down the middle has to be held rather than printed, and the line no
   newline has ended yet has to be drawable while the rest is still coming.

   What it refuses is written down. A carriage return is dropped rather than obeyed,
   because obeying it — and the cursor movement and erasure beside it — is the
   terminal emulator this deliberately is not. Output that redrew a line in place
   reads as the several versions of it, and that is the whole cost.

2. **Handing the terminal to a child and taking it back.** Opening an editor, a pager,
   or anything that wants the terminal for itself. `core/term.Relaunch` replaces this
   process; there is no "let go, wait, and resume". The machinery for it is already
   here and is the reason it would be exact: a session records the modes it turned on
   and unwinds them in the reverse order, which is the same thing giving the terminal
   up and taking it back has to do, twice. Suspend on Ctrl+Z is the same mechanism with
   a signal in front of it.

   **What the analysis missed.** The reader. The modes were exactly as ready as this
   says, and giving them back is now one routine shared by closing a session and
   handing it over — but a session that only restored the modes would still be reading
   the terminal, and every other keystroke would go to this process instead of to the
   child. So the reader no longer blocks in a read it cannot be called out of: it
   waits first, on the terminal and on a pipe this process can write a byte to, and
   parks *before* the read rather than after it. That is what makes "a byte taken here
   is a byte the child never sees" true rather than hopeful.

   It also answers a wart the package had documented about itself: closing a session
   now takes the reader off the terminal at once instead of on the next byte.

   Where a reader cannot be interrupted — anywhere that is not Unix — handing over
   reports `ErrUnsupported` and does nothing. Handing over while still reading is not
   a lesser version of this; it is a child that drops every other keystroke.

3. **Markdown.** Section 3 puts it off to wherever markdown ends up, and `DESIGN.md`
   says why that is a module of its own: it wants a parser and a highlighter, which are
   the dependencies the two modules here promise not to have. It is still the largest
   functional gap, because rendering an answer is the commonest thing a streaming
   interface does — and the hard part is not the parser but that streaming markdown is
   an incremental parse of text still arriving, which no off-the-shelf parser does.
   `glamour` is what a `bubbletea` program reaches for, and it renders a finished
   document.

   **What it turned out to be.** A module, `markdown`, carrying goldmark and nothing
   else — and the hard part was where the analysis said it was. `Stream` splits what
   has arrived into the part that is certainly finished, published once and never
   looked at again, and the part that is not, which is short by construction and
   re-rendered as often as anybody asks. Where it cuts is written down in the package,
   and so is what the rule costs.

   Two things it turned out not to want. A widget: what comes out is `core/text`
   lines, and `Doc` is a `Drawer` and a `Measurer` and nothing else, so a document
   goes into a slot, a container or a viewport belonging to a package this module has
   never heard of. And a highlighter: several megabytes of lexers is a matter of
   taste, which is the argument that keeps one appearance out of the behaviour a
   widget has, so `Look.SetRenderer` is where one plugs in and chroma is nobody's
   dependency until somebody wants it.

4. **Images in a frame.** `core/graphics` knows the protocols and which of them survive
   a redraw; `core/grid` has no notion of an image, so nothing can put one in a drawn
   frame. Section 3 calls this waiting on the frame pipeline, which is exactly what it
   is waiting on.

   **What the decision turned out to be.** A frame is cells, and regions that
   something else writes into. `grid.Painter` is that something: a frame keeps room
   for one, writes the cells around it, and hands it the writer with the cursor
   already at the region's corner.

   One rule, and it is the whole design — a painter must leave the cursor where it
   found it. Every position in a frame is a movement from the last known one and an
   inline block's whole position is relative, so a painter that moved it would move
   everything after it; and that same rule is exactly what makes an image protocol
   usable in a region that redraws, because the one that can be told not to move the
   cursor is the one that can be told to take an image away again.

   So the lifecycle was the work rather than the drawing. A region is named by what
   is in it: an unchanged one writes nothing, a moved one is erased before it is
   painted — a terminal that remembers what it was shown would otherwise hold both —
   and a full repaint says all of it again, because what a terminal remembers being
   shown is not a cell. `core/graphics` needed two methods and one parameter, and the
   two packages still do not import each other.

### 7.2 Widgets that are simply absent

Nothing here is hard. They are listed because "the library has no progress bar" is a
sentence somebody says out loud.

*All of them are built. What each turned out to be is in the last column.*

| was missing | there | what it turned out to be |
| --- | --- | --- |
| **A progress bar** | `bubbles/progress` | `kit.Progress`, and the question that tells it from a spinner: is there a total? The cell the bar ends in is drawn as a fraction of itself, and the percentage beside it gets a fixed field — a bar that shrinks by a column between 9% and 10% goes backwards while the work goes forwards. |
| **A tree** | `lipgloss/tree`, `opentui` | `headless.Tree`, which *is* a list of the rows it is showing: the selection, the scrolling, the wheel and the click are the list's, and only opening and closing are the tree's. Which branches are open is remembered by position, so a file tree refreshed under the reader keeps the shape they gave it. |
| **Tabs** | both | `headless.Tabs` owns controlled or local selection and draws only the pane showing; the strip of names is appearance, so `kit.Tabs` draws it, turns a press into a selection, and hands everything else to the pane in the pane's own coordinates. Both expose structural semantics through the headless controller. |
| **A table with a cursor** | `bubbles/table` | `headless.Table` is a `List` with an order. Sorting carries the cursor with the row it was on by sorting a permutation rather than the rows, so where the selected one went is known exactly instead of guessed at by comparing rows only the caller knows how to tell apart. `kit.Table` hands out its header and one row of cells separately, so the geometry stays in one place. |
| **A list with a filter** | `bubbles/list` | `headless.Filter`: a list, a fuzzy match, and deliberately no text field. Where the pattern is typed stays the caller's, or it would be two widgets in a trench coat and would decide where the field goes for everybody. |
| **The window title, the bell, a notification** | OSC 0/2, OSC 9 | One sequence each on the terminal, exposed together by `Runtime.Session()`. The title is pushed before it is replaced and popped on the way out — a shell whose window is still called "building oolong" an hour later is a program that left something behind — and every one of them strips what cannot go inside a sequence first, because the text is a file name or a model's answer as often as it is a constant. |
| **Somewhere to log while the terminal is taken** | `charmbracelet/log`, `tea.LogToFile` | `term.LogTo`: a file opened for appending, and nothing else. Pointing the standard logger at it is one line and is deliberately the caller's line. |
| **Springs, and a timeline** | `harmonica`, `opentui` | `anim.Spring` keeps the speed it already had when its target moves, which is the whole difference from a transition, and steps by the exact solution rather than a small step of it — stepping a stiff spring approximately at a terminal's frame rate is how one turns into an oscillation that grows. `anim.Timeline` is the sequence neither of the other two can say. |
| **A form that can be answered without a screen** | `huh`'s accessible mode | The four fields expose `Ask` and `Reply`; `kit.Ask` declares and consumes that optional method set locally instead of making `headless` prescribe an adapter contract. A label that is a prefix of two choices is refused rather than guessed at, because somebody answering in words cannot see which one was taken. The conversation stays small because the hard half lives in the fields. |

### 7.3 Where `layout` actually falls short

Not flexbox — section 4 refuses that, and it is still the right refusal. Three smaller
things, and one of them has already been written twice:

- **A gap between slots.** `kit.Table` and `headless.Form` each have their own, which is
  the signal that it belongs one layer down.
- **Cross-axis alignment.** `Align` places text inside a width; a slot cannot say where
  a child narrower than it should sit.
- **A share expressed as a fraction of the whole.** `Flex` is a share of what is left,
  which is not the same thing and cannot express "half of this, whatever else happens".

**All three are `layout.Flow`, `Slot.Cross` and `layout.Part`.** Taking the table's
copy of the arithmetic out fixed a defect in it: a floor on a flexible column was
handed out even when there was no room for it, so a narrow table returned widths
adding up to more than it had and the last column drew past its own right edge.
`layout.Flow.Divide` has always honoured a floor only while there is room, because a widget
cannot see the clip and lays out against the size it was told. The form's copy was a
blank child between every pair of fields, which put things in the focus ring that are
not children; a `Container` has a `Gap` now.

### 7.4 What the comparison says not to build

Recorded because a roadmap that only lists gaps invites filling in the ones already
answered.

- **Cell diffing, against line-string diffing.** `bubbletea`'s standard renderer diffs
  rendered strings by line; this diffs cells. Wide characters, combining marks and emoji
  are exactly where the first produces damage the second cannot.
- **Compositing that asks the terminal first.** `lipgloss` blends with alpha; nothing
  there asks what the terminal draws on, so a translucent layer over a cell left at the
  terminal's own colours is a guess. `Ground`, `Blend` and `Fade` are the answer to that
  and they cost one round trip at startup.
- **Printed output that belongs to the terminal.** Finished blocks go into the
  scrollback and are not redrawn, with a tail column so a stream need not stop at a line
  boundary. There is no equivalent: an inline `bubbletea` program owns every row it has
  ever written.
- **A transcript with selection, search and sticky headers.** Built here, and built by
  hand in every program that wants it there.
- **Keys as a table of names.** `bubbles/key.Binding` is a keystroke and a description
  in one value, which is what `headless.Binding` was until it was taken out. Sequences
  are in neither.
- **Focus and press capture in a container.** `bubblezone` bolts mouse hit-testing onto
  a string renderer after the fact; here it falls out of the container knowing where it
  put its children.
- **A harness that runs the real binary on a real pty**, and arch tests that fail the
  build when an import points the wrong way. `teatest` drives a model; neither of the
  others checks that every mode a session turned on was turned off.

### 7.5 Order

By what blocks what, as before. All of it is done, in this order:

1. ~~**The SGR decoder**~~ — smallest of the four in 7.1 and the one whose absence is
   hit daily.
2. ~~**Handing over the terminal, and suspend**~~ — the mechanism was already here, and
   the reader was not.
3. ~~**A progress bar, the window title, the bell**~~ — an afternoon, and they stopped
   being sentences people say out loud.
4. ~~**The gap and the alignment in `layout`**~~ — a thing written twice is a thing that
   belongs lower down, and the copy that came out had a bug in it.
5. ~~**Markdown, as its own module**~~ — the largest, and the one the module boundary
   was drawn for.
6. ~~**A tree, tabs, a table with a cursor, a list with a filter**~~ — three of the four
   turned out to be a list with something added.

Three things are not on this list and are not omissions. A terminal emulator, for the
reason given in 7.1. Framework bindings of the kind `opentui` ships for React, Solid and
Vue, which Go does not have the same need for. And serving an interface over ssh, as
`wish` does: `core/term.OpenOn` is the primitive that makes it possible, and everything
above it — the server, the keys, the sessions — is a program rather than a library.

---

## 8. What the second round turned up

Kept for the same reason section 6 is: so it is not rediscovered from scratch.

- ~~**A cell cannot hold an image.**~~ Done, and section 7.1 records what the answer
  turned out to be: a frame is cells and regions something else paints.
- ~~**`text.Span` carries a style and not a destination.**~~ Done. It carries one now,
  and it has to be there rather than stamped on afterwards: by the time cells exist
  the columns are gone, and something holding byte offsets into the text it was made
  from cannot say which cells the third word ended up on. A span survives wrapping,
  truncation and drawing, so the address survives with it — which is what lets the
  decoder keep a command's hyperlinks and markdown put an address on the words
  instead of in brackets after them.
- ~~**Handing the terminal over is Unix-only.**~~ Done, and not with `CancelIoEx`: a
  console is a waitable object, so the wait that already existed on Unix is the same
  wait there. What changed is that it stopped being a question about the platform and
  became one about the session — a console can be waited on and a pipe pretending to
  be a terminal cannot, which is the case that has to say so.
- ~~**There was no way out of the grid without a terminal.**~~ Not on any list, and
  found by asking what an interface does when its output is a pipe: everything above
  `core/grid` draws into a view and could not be asked for text, so every test in this
  repository had written the same walk over the cells. `grid.Render` is that walk.
- ~~**Code was not highlighted.**~~ Done, as its own module, which is what the
  seam in markdown was for.
- **`headless.Table` is a `List`, `Tree` is a list of its shown rows, and `Filter` is a
  list of what matched.** Three of the four widgets in 7.2 came out as a list with
  something added, which is worth noticing before the fifth is written: the question to
  ask of anything that shows rows is what it adds to a list, and the answer is usually
  one field.
- **A block of markdown and a printed block of output are the same shape.** Both are
  lines with an indent that a caller wraps at the width it has. `Inline.Append` wants
  text wrapped into what is left of a row, section 6 says the wrapping cannot live in
  `core/grid`, and `markdown.Doc` now does exactly that job one layer up. Whatever
  finally turns a stream of text into rows should be one thing and not two.

---

## 9. What is left

Two things, and neither is a gap in the library.

- **A picture that is not a PNG, and a terminal that only speaks sixel.**
  `core/graphics` reads a PNG's size out of its header, which is why it needs no
  decoder and therefore no dependency; producing sixel means decoding an image into
  pixels, which needs one. A caller holding an encoder is told the terminal will take
  what it makes, and that is the honest place for the boundary.
- **A second worked example.** The one there is streams an answer and proves the
  probe, the theme, the clipboard and the glyph fallback. Everything added since —
  the tree, the tabs, the table, the filter, the images, the handover — is proved by
  its own tests and by nothing anybody can run. That is the next thing worth doing,
  and it is a program rather than a library.

Everything else this was read against is either built, refused on purpose in section
4 and 7.4, or an application's: what a tool call looks like, what an `@` refers to,
how a session is persisted. Those are the things a library that meant to be general
must not decide.
