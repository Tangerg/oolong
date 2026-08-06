# Changelog

All notable changes to this repository. Modules are versioned and tagged
separately (`core/vX.Y.Z`, `components/vX.Y.Z`, `markdown/vX.Y.Z`,
`ptytest/vX.Y.Z`).

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
these modules are pre-1.0: anything exported may still change, and that is the
point of tagging them low rather than not at all.

## [Unreleased]

### Added

- **Output read back into styled text.** `core/text.Decoder` turns the escape
  sequences a command wrote into spans, a chunk at a time: the style in force carries
  across chunks, a sequence split down the middle is held rather than printed, and the
  line no newline has ended yet is there to be drawn. `core/ansi` is the syntax the
  two readers now share, so `core/input` and this cannot come to disagree about what a
  sequence is made of.
- **Handing the terminal to a child, and suspend.** `term.Terminal.Hand` gives the
  terminal away and takes it back — the modes off in the opposite order, then cooked
  mode, then the whole of it again in reverse — with the reader off the terminal
  first, so a byte typed at the child is a byte this process never took.
  `term.Suspend` and `Loop.Suspend` are Ctrl+Z. The reader now waits before it reads
  rather than blocking in a read nothing can interrupt, which also means closing a
  session takes it off the terminal at once instead of on the next byte. Unix only,
  and it says so rather than dropping keystrokes elsewhere.
- **What a session says beside its frames.** `SetTitle`, `Bell` and `Notify` on the
  terminal, the host and the loop. The title is pushed before it is replaced and
  popped on the way out; every one of them strips what cannot go inside a sequence
  first. And `term.LogTo`, which is somewhere to write while the terminal is taken.
- **`kit.Progress`** — a bar for work with a total, with the cell it ends in drawn as
  a fraction of itself and a fixed field for the percentage.
- **`layout.Flow`, `Slot.Cross` and `layout.Part`** — room between slots, where content
  narrower than its slot sits, and a share of the whole division rather than of what is
  left. `kit.Table` and `headless.Container` use the first instead of each having their
  own; taking the table's copy out fixed a floor that was handed out when there was no
  room for it.
- **A tree, tabs, a table with a cursor, and a list with a filter**, in `headless` with
  appearances in `kit`. Three of the four are a list with something added, which is
  what keeps the selection, the scrolling, the wheel and the click in one place.
- **`anim.Spring` and `anim.Timeline`** — movement that keeps the speed it had when its
  target moves, stepped by the exact solution rather than a small step of it; and a
  sequence of keyframes, which neither of the other two can say.
- **A form answerable without a screen.** `headless.Spoken` is a question in one line
  and a line back, on all four fields; `kit.Ask` is the conversation.
- **One way to dress a widget that draws itself.** `headless.Look` is it, and the
  three that draw themselves take it: a field, the editor inside one, and a
  completion. `kit.Theme.Look` builds one from a palette, so a form and the editor
  in it cannot disagree about what a placeholder looks like. The style fields those
  widgets carried — `Editor.Style`, `PlaceholderStyle`, `SelectionStyle`,
  `Completion.RowStyle`, `SelectedStyle`, `MatchStyle`, `DetailStyle` — are gone.
- **`markdown`, a module of its own**, carrying goldmark. `Stream` publishes what is
  certainly finished once and re-renders only what is still arriving; what comes out is
  `core/text` lines, and the drawable form is a `Drawer` and a `Measurer` and nothing
  else. Code highlighting is a seam rather than a dependency.

- **A terminal can now be asked things.** `core/input` decodes the two shapes an
  answer arrives in — operating system commands and device attributes — and
  `core/term.Options.Probe` puts the two questions worth asking during startup:
  what colour the terminal draws on, and what it claims to support. The device
  attributes query goes last because every terminal answers it, so its answer
  arriving alone is how a terminal says it did not understand the other one.
- **`core/clipboard`**, and `Copy`/`Paste` on the loop and the host. The terminal
  does the copying, because over ssh or through a multiplexer it is the only end
  of the connection the user is at. A read comes back as an ordinary
  `input.Paste`, so anything that already inserts a paste needs nothing further.
- **`core/term.Relaunch`** starts the program again in place of itself, keeping
  the terminal and, on Unix, the process. It is the only way to move an interface
  between the alternate screen and the terminal's own.
- **`components/headless.Transcript`** — a session's output as blocks in one
  coordinate space, measured incrementally. Everything below is answered in it.
- **Selection** over a transcript, with copy that rejoins what the width broke and
  takes a wide character only when it lies wholly inside.
- **Search** over a transcript, off the interface's goroutine, newest query wins.
- **Sticky headers**, which keep the question on screen while the answer scrolls.
- **Editor selection, clipboard and atomic elements.** Shift with any way of
  moving is what selects; an element is a run of text that behaves as one
  character, with an identity that survives editing around it and undo.
- **`components/headless.History`** and **`Commands`** — the two halves of a
  prompt. History gives back the draft a walk interrupted; the registry ranks by
  what was typed and by what was used last, and is the first consumer `core/fuzzy`
  has had.
- **`components/kit.Transcript`** and **`Palette`**, the default look for the
  above, plus **`Glyphs`** with an ASCII fallback and **`Suited`**, which picks a
  theme from what the terminal said.
- `core/link.At` and `core/link.Map`, and `core/text.StampLink`, which is what
  finally connects finding a URL to making it clickable.
- `core/graphics` names iTerm2 and sixel alongside kitty, and says which of them
  can be used in a region that redraws.
- `core/text.Wrapped` records the byte range each row came from, which makes the
  wrap invertible — needed by links, selection and search alike.
- **File paths are links.** `core/link` finds rooted, relative and quoted paths,
  the line and column written after one, and says whether a link points at a URL
  or a file — two destinations with different meanings, not one with two
  spellings. A path is usually *not* given to the terminal as a hyperlink,
  because the terminal finds paths itself and knows the directory.
- **`core/input.Wheel`** says what a terminal's wheel reports are worth. They
  carry no magnitude and terminals send between one and three per notch, so a
  fixed number of rows per report scrolled three times as far on half of them.
- **Double-click and triple-click**, with `core/text.WordAt` deciding where a
  word is — including the three scripts written without spaces, where the script
  itself is the boundary.
- **The mouse reaches a text field.** Click to place the cursor, drag to select.
- **`headless.Transcript.Commit`** gives the leading run of finished blocks to
  the terminal, in order and once each.
- `headless.Scroll.Reveal` brings a row into the window, without which a search
  match could be found and not reached.
- A style for the selection in a text field, and `Spans`, which is what draws one
  in a wrapped field. (It is `Editor.Look.Selection` now — see the polish below.)
- **A terminal is asked what it is.** The startup probe now sends the version
  query and the secondary attributes query, and `core/input` decodes device
  control strings and the two private-marker reports. What the terminal said
  outranks the environment everywhere it is used, because environment variables
  describe the terminal a shell started in and not the one on the other end of an
  ssh connection.
- **`Terminal.Keyboard`** reads back which of the Kitty keyboard enhancements
  actually took. Asking for them is not the same as getting them, and a terminal
  that accepts unambiguous codes and gives nothing for releases is
  indistinguishable from a user who has not lifted a key.
- **`Terminal.ReportDirectory`** tells the terminal where the program is working,
  which is what lets its own path handling resolve relative paths in the output.
- `Loop.Wheel` and `Host.Wheel` carry the wheel profile to a component.

- `core/term.OpenOn` takes over a terminal that is not the process's own, which
  a program serving a session over a pty needs and which makes the package's
  lifecycle testable against a real terminal.
- `core/grid.Screen.Cursor` and `core/grid.Inline.Cursor` report where the last
  frame asked the terminal's cursor to go. Nothing could observe it before
  short of decoding the escape stream.
- `components/headless.Editor.SetCursor` moves the caret to a line and offset,
  clamped and pulled back to a cluster boundary. The editor could report where
  its cursor was and could not be told where to put it.
- Fuzz targets over the input parser, asserting properties rather than the
  absence of panics — above all that the same bytes decode the same way however
  a read happened to split them.
- Benchmarks over the cell diff, drawing and wrapping.

- **Compositing.** `core/grid.View.Blend` paints a translucent sheet of colour over
  a region, and `View.Fade` dissolves what is in one into whatever it is drawn on.
  A terminal has no alpha channel, so both resolve to opaque colours before
  anything is written — which needs to know what a cell left at the terminal's own
  colours actually is. `grid.Ground` is that answer, `core/term` asks for it with
  OSC 10 and 11, and the program hands it to the surface being drawn on. What
  cannot be resolved is left alone rather than guessed at, everywhere.
- **`components/kit.Scrim`** is what a layer paints over what it covers, and it
  lives on `Theme`: how far an interface recedes is part of its look, and a light
  one takes less of it than a dark one.
- **`components/headless.Container`** — widgets arranged in a region, and the answer
  to which of them an event is for. A key goes to the widget that has the keyboard;
  a mouse event goes to the widget it is over, in that widget's own coordinates, and
  a press is captured until the release. Every interface with two things on screen
  was doing all three by hand, and the last of them can only be answered while a
  frame is being drawn.
- **`components/headless.Focusable`** — a widget that can hold the keyboard, and is
  told when it does. A frame has one cursor, so two fields both asking for it is not
  two cursors: it is one, wherever the last of them drew. A widget nobody has told
  anything assumes it has the keyboard, which is what makes a lone field work.
- `core/layout.Axis`, `Axis.Rects` and `Wanted`. The geometry without a view, because
  a click arrives between two frames and has to be answered against the frame that is
  on screen.
- **`core/text.Edit` and `core/text.Mark`.** An insertion, a deletion and a
  replacement are one thing said three ways, and a range that has to stay over the
  same words while the text around it changes is one problem however many things want
  it — a chip in a prompt, a highlight, a search result, a diff region.
  `headless.Element` is now a mark, and the editor's two shifting routines are one
  call to `text.Shift`.
- **`core/input.Keymap`, and an action is a name.** A widget names what it can do —
  `delete-word-back`, `select-all` — and a map says which keystrokes produce which
  name. A widget owning both could express neither: there was nowhere to put a binding
  two chords long, rebinding one key meant replacing a whole struct, and the same
  widget in two interfaces could not mean two things by escape. `Chord`, `Keys`,
  `Pending` and `ParseChord` are the rest of it, and a keybinding now survives being
  written to a file and read back.
- **`Do` on everything that reads a key.** `Editor`, `List`, `Scroll`, `Completion`,
  `Stack` and `Container` answer to the name of an action, which is what makes one
  reachable from somewhere that is not the keyboard — a menu, a command typed by name,
  a test that presses nothing. It is also what lets a completion drive the list inside
  it without offering the same keystroke to the same map twice.
- `core/input.Key.At`, stamped by the terminal's reader as `Mouse.At` already was. Two
  chords in one burst are a sequence and a terminal never says so; the same two with a
  pause between them are two keystrokes that happen to be adjacent.
- **`components/headless.Viewport`** — content in a box, scrolled. There was a scroll
  position and something that drew a bar, and nothing that put the two together. It is
  short because a view is already a clipped window onto a surface: content drawn into
  one that begins above the box lays itself out at its full height and loses what is
  outside, so nothing has to be taught about being scrolled.
- **A field that holds one line**, as `Editor.SingleLine`: nothing puts a line break in,
  and text wider than the box slides sideways instead of wrapping. `Editor.Mask` is what
  each cluster is drawn as for something the screen should not show, and it holds one
  line whether it was asked to or not — a secret is one value. It is a mode of the field
  rather than a field of its own because that is the whole difference: what a cursor is,
  what selecting means and where a click lands are the same questions with the same
  answers.
- **`core/diff`** — what changed between two texts, line by line, and the hunks worth
  showing. It is beside `core/fuzzy` for the same reason that is: what changed is a fact
  about two strings and has nothing to do with a terminal. `diff.Between(old, new)` is
  a `Script`, which knows how to break itself into hunks and how to write itself out.
- **A keybinding survives a configuration file.** `Chord` and `Keys` marshal to and
  from text, so any of the usual decoders fills a keymap in without being told how.
- **A list row can be pressed**, and so can a choice and a yes-or-no. A list could be
  walked with the arrow keys and not pointed at, which is not something any list
  anywhere does. `List.Row` is told which row it is drawing, because a row is often
  about more than the item and finding the index again by comparing items is a guess
  whenever two of them are alike.
- **`components/kit.Diff`** draws one, which is what `Theme.Added`, `Removed` and
  `Context` were waiting for. It is sized, so a change taller than its pane goes in a
  `Viewport` and scrolls with no further arrangement.
- **Forms, in `components/headless`.** `Field` is one thing a form collects — it holds a
  value, says whether it is valid and answers input; `Form` is the walk between fields
  and the check across them; `Accessor` is where the value lives, which is the caller's
  own variable and not a copy the field keeps. `Text`, `Select`, `MultiSelect` and
  `Confirm` are the four of them, and `kit.Form` is the look: a theme becomes the
  handful of roles a field draws itself in, and a glyph set becomes the marks beside a
  choice. A field draws itself because a field is generic over what it holds, and
  nothing could name every kind of one — so the look travels down instead.
- **`core/grid.Inline.Append`, `Tail` and `Break`, and `InlineLoop.Append`.** Printed
  output no longer has to begin at column zero. Streaming output does not arrive on
  line boundaries, and a printer that started every piece on a row of its own turned a
  reply delivered three words at a time into three rows. What is appended is offered
  what is left of the open row and no more, so it cannot wrap and take the block's
  anchor with it, and the loop keeps offering whole rows until the caller says it has
  finished — which is the only way to lay text out against room that is not knowable
  from another goroutine.

### Changed

- `core/input.WheelFor` and `core/graphics.DetectIn` take what the terminal called
  itself, which outranks the environment.
- `Background()` on the terminal, the host and the loop is now `Ground()`, which
  carries both of the terminal's own colours and says which of them it gave. The
  background alone decides a theme; the pair is what a translucent layer mixes
  with. `kit.Suited` takes the pair.
- `kit.Overlay.Shade` and `kit.Dialog`'s backdrop are a scrim rather than a style
  merged over every cell — a style could only set one colour over everything,
  which erases what is behind instead of dimming it. A dialog dims with its
  theme's scrim and no longer carries an opinion of its own.
- A pinned header now dissolves as the next one pushes it off.
  `headless.Pinned.Fade` was being worked out and never drawn.
- **There is one way to dress a widget, and it is a field.** Every `kit` widget takes
  a `Theme`, and a `Glyphs` as well if it draws furniture; none of them carries styles
  of its own. `Palette.Dress`, `Dressed`, and the package-level `Rounded` and `Square`
  are gone — the last two were borders built from whatever glyphs the machine that
  compiled the program could draw. Text is the exception: `Label` and `Paragraph`
  take a style, because which role a piece of text plays is the caller's to say.
- `kit.Transcript.Handle` takes an `input.Event` like every other widget, and the
  click counter it used to be handed lives on `headless.Selection`, which is the only
  thing that asks.
- **`headless.Binding` is gone, and with it every `…Keys` struct.** A widget's `Keys`
  field is an `*input.Keymap`, and one map can serve a whole interface: a program binds
  its own send key alongside the field's editing keys and hands the same table to both.
  `kit.Help`, `Composer.Hints` and `Dialog.Hints` name actions and read the keystroke
  back out of the map, so a hint cannot disagree with what the key does. There is no
  `Hidden` flag any more: hiding a hint is not listing it. There is no description
  field either — an action's name is what there is to say about it, and a description
  kept beside the name is one that drifts from it.
- **Shift and tab are one keystroke.** `input.Backtab` is gone. A terminal sending the
  legacy sequence and a terminal speaking the Kitty protocol reported the same physical
  key two different ways, so a binding could only ever match one of them.
- `headless.Stack` owns the interface its layers float over, as `Base`. Owning it is
  what lets the stack say who has the keyboard.
- **An escape sequence that never completed is now read as a chord.** It used to
  become the Escape key plus the character, which made Alt+[, Alt+] and
  Alt+Shift+O unbindable. An escape pressed on its own is followed by a human
  gap and is settled by the grace timer while nothing has followed it at all;
  both bytes arriving before the pause means one burst, and a burst is a chord.
- `components/headless.Sticky` — the modal that the escape key does not close —
  is now `Insistent`, with `Insists()`. The name was needed for sticky headers,
  and two unrelated concepts under one name is cheap to fix now and confusing
  forever afterwards.
- `Transcript.Rows` returns rows that say how they join; what was `Rows()` (the
  total height) is `Height()`.
- `core/program.Host` gained `Background`, `Copy` and `Paste`. A test host that
  can say the terminal is light is a test that can check a look both ways round.
- `core/program.InlineLoop.Print` takes something that can measure itself and
  measures it on the loop's goroutine. The row-count form is `PrintRows`.
- `core/grid.EncodeRow` takes a colour depth.

### Fixed

- **A terminal's answers were being read as keystrokes.** A control sequence
  carrying a private marker is a report and never a key, and dispatch was looking
  only at the final byte: the Kitty keyboard flags reply arrived as an invisible
  control character, and the version string as fifteen keystrokes. Both are now
  covered by one rule rather than two special cases.
- The input parser's cap on a control sequence's parameter section depended on
  where a read happened to split, so the same bytes could decode differently.
- Bracketed paste could carry text that was not valid UTF-8.
- `core/term.Writer.Drain` took the wake-up the loop was owed, which hung a
  program on a machine slow enough for Drain to reach the channel first.
- A layout gave out more rows than it had when several floors did not fit.
- **Shift+Tab did not walk the keyboard backwards.** The container was bound to tab
  with shift held and terminals that do not speak the Kitty protocol send a key of
  their own for it, which nothing matched. One keystroke, one spelling.

## [0.0.1] — 2026-08-05

First tagged version of `core`, `components` and `ptytest`.
