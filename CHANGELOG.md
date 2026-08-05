# Changelog

All notable changes to this repository. Modules are versioned and tagged
separately (`core/vX.Y.Z`, `components/vX.Y.Z`, `ptytest/vX.Y.Z`).

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
these modules are pre-1.0: anything exported may still change, and that is the
point of tagging them low rather than not at all.

## [Unreleased]

### Added

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
