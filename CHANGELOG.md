# Changelog

All notable changes to this repository. Modules are versioned and tagged
separately (`core/vX.Y.Z`, `components/vX.Y.Z`, `ptytest/vX.Y.Z`).

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
these modules are pre-1.0: anything exported may still change, and that is the
point of tagging them low rather than not at all.

## [Unreleased]

### Added

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
- `headless.Editor.SelectionStyle`, and `Spans`, which is what draws a selection
  in a wrapped field.

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

### Changed

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

- The input parser's cap on a control sequence's parameter section depended on
  where a read happened to split, so the same bytes could decode differently.
- Bracketed paste could carry text that was not valid UTF-8.
- `core/term.Writer.Drain` took the wake-up the loop was owed, which hung a
  program on a machine slow enough for Drain to reach the channel first.
- A layout gave out more rows than it had when several floors did not fit.

## [0.0.1] — 2026-08-05

First tagged version of `core`, `components` and `ptytest`.
