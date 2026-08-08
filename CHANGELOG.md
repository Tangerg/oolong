# Changelog

All notable changes to this repository. Modules are tagged separately —
`core/vX.Y.Z`, `components/vX.Y.Z`, `markdown/vX.Y.Z`, `highlight/vX.Y.Z`,
`ptytest/vX.Y.Z`, `ssh/vX.Y.Z` — and share one version number: a release is a state of the
repository.

From 0.1.0 they are also one coordinated release train, as
[the release policy](docs/architecture.md#13-breaking-change-and-release-policy)
requires: every public module is tagged at every release with the same version and
these same notes, whether or not its own files changed, and users upgrade the set
together. Before 0.1.0 an unchanged module kept the tag it already had, which is why
some earlier versions are missing from some modules.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
these modules are pre-1.0: anything exported may still change, and that is the
point of tagging them low rather than not at all.

## [Unreleased]

## [0.3.0] — 2026-08-08

A seventh module, five components that could not be built here before, and a pass over
what a component is allowed to own.

`ssh` is the first host this repository ships that is not the terminal it is running
on, and it is one exported function: an application that has already accepted and
authenticated a session hands it over, and `core` learns nothing about servers. It is
also the first evidence that `program.Host` at three methods is a seam somebody else
can reach.

The rest of the additions are the components the
[prior-art survey](docs/prior-art.md#a-general-behaviour-and-it-passes-the-gates) found
missing outright: a bounded numeric value, line numbers, an assembled code block,
columns sized from their content, and a settings list. Each is behaviour in `headless`
and appearance in `kit`, and the two sliders in the dashboard example drive the same
controller.

The changed half is one idea applied everywhere it was still missing: **a component
owns what it derives from.** Forms, trees, tables, timelines, histories, registries and
searches took their collections through exported mutable fields, which meant nobody
could release the storage behind them, settle focus when they changed, or promise that
drawing saw a finished state. They take them through methods now.

Breaking. `Form.Fields`, `Tree.Nodes`, `Stack.Base`, `Viewport.Content`,
`DialogTrigger.Of`, `Panel.Of`, `Spinner.Frames`, `Scrollbar.Thumb`, `kit.Braille` and
`DefaultFrameRate` are replaced by constructors and methods; `Container.Give`,
`Stack.Push`, `Stack.Contains`, `Stack.Remove` and `History.Limit` change shape because
identity is an index or a `LayerID` rather than an interface value that could panic on
comparison. There are no deprecated aliases: see the
[breaking-change policy](docs/architecture.md#13-breaking-change-and-release-policy).

### Added

- **An SSH PTY is now a first-class optional host.** The separate `ssh` module
  adapts an already accepted `charm.land/ssh` session into the ordinary program
  transport without teaching `core` about servers or authentication. It owns byte
  decoding, terminal modes, causal failures and coalesced window changes for the
  call, resolves colour from the client environment, bounds peer-controlled cell
  allocation, and leaves channel closure and exit status with the application.

- **A renderer-sized screen assertion for tests.** `ptytest.Screen` incrementally
  applies the text, movement, erasure, bounded scrolling, SGR, OSC and mode syntax
  emitted by Oolong's cell and inline renderers. It exposes fixed cell text while
  explicitly rejecting device-control painters and declining terminal queries,
  input and alternate-buffer ownership.
- **A bounded editor kill ring.** Consecutive backward and forward kills accumulate
  in reading order, `Yank` restores the newest entry, and `YankPop` cycles older
  entries only while the yank remains the immediately preceding edit. The ring owns
  its strings and retains at most sixteen entries.
- **A bounded numeric control now has one behavior model and any number of
  appearances.** `headless.Slider` owns inclusive bounds, controlled or local value,
  keyboard steps, pointer dragging, committed track geometry, focus, and typed
  semantics. `kit.Slider` supplies the polished labelled track without duplicating
  those rules, and the dashboard uses the same control to change its live work rate.
- **Source gutters and code blocks share visual-row provenance.** `text.Row` can name
  its logical source line, `headless.RowGutter` is the appearance seam used by the
  editor and passive code blocks, and `kit.LineNumbers` is its reusable line-number
  implementation. `kit.Code` assembles styled `text.Line` values without making the
  component graph depend on a syntax highlighter.
- **Tables can fit columns to the content they actually draw.** A `kit.Cell` carries
  its preferred width with its painter, `Column.Fit` delegates allocation to the
  existing measured-layout path, and `TableLayout` computes the scan once and reuses
  it for every row drawn and for committed heading hit tests.
- **Settings are a real two-layer component.** `headless.Settings` composes list
  selection and scrolling with actions for the selected value; `kit.Settings`
  projects application-owned labels and values into a fitted two-column appearance.
  The dashboard uses it to edit the same live rate controller shown in another pane.

### Changed

- **Forms own their field collection and its focus transitions.** `NewForm`, `Set`,
  `Add`, and `Fields` replace the mutable `Form.Fields` slice. Replacing the field at
  a focused position now releases the old controller and transfers keyboard input to
  the new one through `Container`'s single settlement path; drawing no longer rebuilds
  children behind that owner's API.

- **Trees own hierarchy changes instead of discovering them during drawing.**
  `NewTree`, `SetNodes`, and `Nodes` replace the mutable `Tree.Nodes` slice, recursively
  copy nested node collections, rebuild the visible list only at semantic transitions,
  and release expansion paths that no longer name a branch. Drawing and measurement
  now read one already-settled selection state.

- **A table layout is an actual snapshot.** `Table.Layout` owns the column
  definitions it measured, so later edits to the table configuration cannot pair old
  boxes with new titles, alignment, or sort furniture.

- **Stateful value sequences own and validate their configuration.** `Timeline`
  keyframes now enter through `NewTimeline` or `SetFrames`, are copied, and must be
  strictly ordered; `Frames` returns a snapshot. History capacity now has the
  conventional `SetLimit`/`Limit` pair, a clearly named `DefaultHistoryLimit`, and
  rejects negative limits instead of silently treating them as defaults.

- **Every host now shares one bounded geometry contract.** `program.ValidateSize`
  guards both opening geometry and later resize events before they reach a grid
  allocation; non-positive, overflowing, and excessive peer-controlled surfaces
  return `ErrInvalidSize` instead of drawing blank, panicking, or allocating without
  a bound. `programtest` and the optional SSH adapter consume the same rule.

- **Program configuration has one side-effect-free validator.** `Config.Validate`
  owns the same Root/Inline contradictions enforced by `Run`, so a transport adapter
  can reject an impossible program before it acquires input or writes terminal
  modes instead of copying runtime rules. `FrameInterval` names its duration by what
  it is (rather than calling it a rate) and rejects a negative value instead of
  silently interpreting an invalid duration as the default.

- **Composed controls have one configuration path.** `kit.Composer` now projects
  its settings into the enclosed editor once per public entry point, and its private
  default key map is the same map used for both editing and help. Measurement,
  drawing and input can no longer observe different editor configurations.
- **Drawing a select no longer chooses for the application.** Initial presentation
  may read a bound value to position the cursor, but only an input action or semantic
  validation writes the selected option back. Rendering is therefore observationally
  pure even when the caller's current value is not among the options.

- **Retained component identity is explicit and total.** `Container` addresses focus
  by item index, optional `Item.Key` carries it across reordering, and `Stack.Push`
  returns a `LayerID` used by `Contains` and `Remove`; replacing the stack base goes
  through `SetBase`. Containers, tabs, pointer regions and modal stacks no longer
  compare open `Widget` or `Modal` interface values, so valid value implementations
  containing slices or maps cannot panic routing or become impossible to remove.

- **Search has one explicit latest-question state machine.** A mutex-owned mailbox,
  rather than a channel used as both storage and notification, now defines replacement,
  cancellation, result publication and close. Clearing a query invalidates running and
  unread work, close releases every retained snapshot, and the standard regexp engine
  remains the sole definition of matching semantics.

- **A table has one geometry API.** `Table.Layout` is now the sole operation that
  measures columns; drawing headings and cells, reading widths, and hit-testing all
  go through the resulting `TableLayout`. The convenience methods that silently
  recomputed fitted columns are removed, preventing a composed table from turning a
  linear content scan into one scan per row.
- **Built-in themes share one semantic mapping.** Dark and light themes now differ
  only in their raw palettes; the translation from palette colours to interface roles
  is defined once, so adding or changing a role cannot leave one built-in theme with
  different semantics.
- **Long-lived registries and editor histories own their storage completely.**
  Commands and completion candidates deep-copy nested values at their boundaries,
  command history detaches retained strings, editor line mutations use the standard
  slice operations that clear removed elements, and undo/redo stacks clear popped and
  evicted snapshots instead of retaining text behind spare capacity. Component keys,
  tab and option labels, dialog semantics, filter patterns, slider labels, and editor
  text likewise detach from caller backing strings at their ownership cuts.
- **Coverage-sensitive furniture has one source.** `Spinner`, `Status`, and
  `Scrollbar` now consume `Glyphs` like every other kit component; the parallel
  `Frames`, `Track`, `Thumb`, and `Braille` paths are removed. Scrollbar geometry also
  uses the same overflow-safe proportional operation as layout and sliders.

- **Proportional integer coordinates use one overflow-safe operation.**
  `layout.Scale` is now shared by layout allocation and slider value-to-track mapping;
  progress and slider rows likewise share one private label/track/value layout instead
  of maintaining two narrow-width policies.
- **Editor gutters participate in measurement, scrolling, pointer routing and
  selection geometry.** Decoration is outside copied text, clicks in the gutter are
  not mistaken for text clicks, and a masked field exposes only its mask to an
  appearance callback.
- **A table cell has one definition for measurement and drawing.** `Table.Cell` now
  returns a behaviour-rich `kit.Cell` instead of accepting a paint-only callback;
  custom cells use `NewCell` and text cells use `LabelCell`.
- **Lists can accept a frame-local row renderer.** `List.DrawRows` preserves the
  list's selection, scrolling and committed routing while a composed appearance
  reuses geometry it computed once for the frame.
- **Composed row appearances no longer rewrite their controllers.** `Tree.DrawRows`
  mirrors the list contract, and trees and filters now supply a renderer for one
  frame instead of assigning through a retained `Row` field during drawing.
- **Styled-text ownership has one primitive.** `text.Line.Clone` deep-copies spans
  and detaches their text and links; kit paragraphs, code blocks, and markdown
  documents use it at their long-lived ownership boundaries.
- **Command aliases resolve to one canonical identity.** Exact names outrank aliases,
  and recording use through an alias moves the command itself rather than adding a
  duplicate recent row.
- **Detection and image sizing close their extreme-input paths.** Link targets detach
  from source documents at the result boundary and overlapping shapes are filtered
  once instead of rescanning URLs per path; image fitting uses overflow-safe ceiling
  and nearest-ratio arithmetic for every positive `int` input.

## [0.2.0] — 2026-08-08

Ownership, everywhere it was still implicit: of a gesture handed through a boundary,
of the storage behind a value that outlived the chunk it came from, of a failure that
used to arrive as a boolean. Exported mutable fields became accessors because a
collection a caller can write to is one whose storage nobody can release, which is
what made the retention work possible rather than merely tidy.

Breaking. `Editor.HandleMouse`, `Doc.Blocks`, `Stream.Look`, `Paragraph.Lines`,
`kit.Dots` and the exported slice fields on lists, filters, containers, sticky headers
and choices are replaced by methods; `Drain` and `Present` return errors instead of
booleans. There are no deprecated aliases: see the
[breaking-change policy](docs/architecture.md#13-breaking-change-and-release-policy).

### Changed

- **One boundary owns a gesture handed to a child, and it lives with the behaviour.**
  `headless.PointerRegion` replaces the private copy each appearance wrapper had:
  composer, dialog, form, tabs and panel now compose the same object, and so can an
  appearance layer this repository never ships. It keeps the rule `Container` and
  `Stack` already keep — a press decides the owner, the drag and release follow it —
  and it keeps it the same way they do, by remembering *which child* rather than
  *which rectangle*. That is the substantive change: a held gesture is now translated
  by where this frame drew its child, so a selection no longer jumps by the distance
  a resize moved the pane under it, and a child that has stopped being presented has
  its remainder dropped rather than measured against a rectangle it has left.

- **An editor routes a pointer against the frame it drew.** `Editor.HandleMouse` is
  gone; `Editor.Handle` answers a mouse event using the width from its own committed
  presentation. The width was never the caller's to choose — a press is aimed at what
  is on the screen — and asking for it was the reason wrappers needed a second way to
  reach a child at all. `Editor.At` remains for geometry queries at an explicit width.

- **A box with no room has no interior.** `Box.InnerRect` and `Box.Inner` now agree
  with `Box.Draw` at degenerate sizes instead of reporting an origin inside a region
  that does not exist. A collapsing layout produces those sizes, and a caller that
  measured with one and drew with the other was laying out against a rectangle the
  frame did not have.

- **Transcript highlighting now scales with the visible window.** Match collections
  are explicitly ordered and non-overlapping, the shape already returned by
  `headless.Search`, so a frame locates visible matches without walking session-old
  results. A fixed viewport with 100,000 earlier matches fell from roughly 174 µs to
  18 ns in the repository benchmark, with zero allocations in both cases.

- **Presentation and frame-drain failures are explicit API results.**
  `present.Presenter.Present` now preserves an owed request when frame construction
  fails, and `term.Writer.Drain` returns an error with `ErrDrainTimeout` rather than
  collapsing transport state into a boolean. `program.FrameWriter` follows the same
  contract, so custom hosts cannot silently turn a failed logical frame into an
  accepted one.

- **Cached component models now own every input that controls their derivations.**
  Documents, paragraphs, lists, filters, containers, sticky headers and form choices
  accept content through mutation methods and return snapshots; multi-choice
  selection migrates by value rather than stale position. Filter text projections,
  table ordering and streaming-markdown looks now change through
  invariant-preserving methods. The exported mutable spinner slices are gone:
  `kit.Braille()` returns an owned set and `Unicode().Spinner` is the coverage-aware
  fallback set.

### Fixed

- **Presented identity and pointer ownership are now transactional at every
  composition boundary.** `headless.Root` publishes its target with the complete
  frame and retains accepted gestures across root replacement. Kit wrappers share one
  committed child-region model, so a drag or release cannot be lost to chrome,
  collapsed layout, or a replacement child. Transcript selection settles its original
  owner even outside the viewport, and container, stack, and pointer cancellation now
  clear stale gestures before they can resume.

- **Search result streams now have a complete lifetime.** The worker closes
  `Search.Results` when `Search.Close` stops it, allowing consumers to range over the
  channel without leaking a goroutine or coordinating a second private stop signal.

- **Released content no longer survives in reusable backing storage.** Shrunk
  surfaces, paint regions, inline publication queues, component child caches,
  filtered links, document wraps, and tree rows clear discarded pointer-bearing
  slots. Streaming markdown, terminal input, and ANSI text decoders clone only their
  small undecided tails at ownership cuts, so published or consumed prefixes cannot
  remain alive through a substring or subslice.

- **Frame output is transactional through the writer boundary.** Screen and inline
  canvases include paint-region identity in their frame diff, propagate painter
  failures, complete short writes before settling, and leave failed frames pending
  instead of diffing from terminal state that never existed. Programs preserve those
  causes, stop queuing after a known output failure, and never retry a failed painter
  during teardown or handover.

- **Transport and display ownership now close as one causal lifecycle.** Programs
  capture each event and progress stream exactly once, reject missing or prematurely
  closed protocol channels, and preserve asynchronous writer failures during
  cancellation. Terminal open rolls back every acquired mode on partial failure;
  handover refuses undrained output, restores ownership even when a child panics, and
  never runs the child after a failed release.

- **Long-lived collection storage follows live state rather than peak state.**
  Owned component slices clear removed references and release disproportionate
  backing arrays after large contractions. A final byte-ingress consumer that panics
  can no longer leave producers waiting forever for its `Done` signal.

## [0.1.1] — 2026-08-07

Additive. Nothing exported was removed or changed shape, so an application on 0.1.0
compiles unchanged; the two additions are the supported way to test a program and the
live counterpart to `Box`.

### Added

- **A public in-process program harness.** `core/programtest.Host` replaces the
  examples-only fake with a supported application testing boundary. Its base type
  implements only the three required `program.Host` transport methods, so tests can
  still prove capability absence and can add individual optional capabilities by
  embedding it. Frame-driven assertions avoid polling sleeps, and all examples now
  test through this public path.

- **A live framed panel composition.** `kit.Panel` owns one focusable child, measures
  through its `Box` overhead, commits its inner routing rectangle with the root frame,
  translates pointer coordinates, and forwards keyboard ownership. A press its child
  takes owns the gesture until release, wherever the pointer then goes, which is the
  rule `headless.Container` and `headless.Stack` already keep and which a panel has to
  keep again because it is another place a gesture is handed through. `Box` remains the
  passive chrome for strings, blocks, widgets, or empty regions. The file-browser
  example proves the new composition with two framed, independently focusable panes.

## [0.1.0] — 2026-08-07

The first three architecture slices are complete and every invariant this document
can enforce now has a named executable guard. The minor version says that the shape
of a streaming program has settled; it is still pre-1.0, so [the breaking-change
policy](docs/architecture.md#13-breaking-change-and-release-policy) still applies and
the modules are still one coordinated release train.

### Added

- **Architecture invariants now have complete executable gates.** Every production
  `Draw` receiver in `headless`, `kit` and `markdown` is classified as a stateful
  contract or an immutable presentation value; stateful cases draw twice, preserve
  their semantic projection, and emit identical styled cells and cursor state. The
  classification is derived from the package's own source, so a new `Draw` fails until
  its ownership class is named. The release workflow now pins `gorelease`, compares a
  proposed public module with its preceding immutable tag, reports pre-1.0 changes,
  and rejects incompatible v1+ proposals.

- **Bounded lossless byte ingress.** `program.ByteIngress` batches adjacent writes
  behind at most one owner task, bounds pending producer data, applies backpressure,
  orders a final error after accepted bytes, and cancels with its dispatcher. The
  subprocess example now uses it instead of posting one unbounded closure per read.

- **The canonical streaming interface now closes the first architecture slice.**
  `examples/streaming` combines approval, focus restoration, bounded background
  ingress, an open markdown tail, stable publication, a selectable recent window,
  cancellation, failure, resize and real-PTY coverage in one path. The new
  `kit.Transcript.CommitN` transfers only an excess finished prefix, and transcript
  input now routes wheel and configurable scrolling actions to its owned scroll state.

### Changed

- **Copyable rows are now lower-level text values.** The old
  `headless.Row` leaked component vocabulary into any document that wanted to
  support transcript selection or search. `text.Row` now carries meaningful text,
  its rendered column offset, and reversible wrap metadata. Paragraphs, messages
  and markdown documents implement the same copyable shape without markdown
  depending upward on components; gutters and markers stay out of copied text while
  hit testing and search still report screen columns.

- **Dialog and tabs are controller-owned compound controls.** `headless.Dialog`
  now owns open state, stack membership and focus restoration through distinct
  content and trigger parts; `headless.Tabs` owns an encapsulated set of tab parts
  and selection. Both have explicit controlled and uncontrolled constructors and
  expose typed structural semantics. `kit.NewDialog` and `kit.NewTabs` provide the
  polished, themeable short path while leaving their headless controller and
  appearance parts reachable. The old direct `kit.Dialog` modal and public
  `headless.Tabs.Items` mutation paths are removed.

- **Live components now commit presentation state atomically.** `headless.Widget`
  draws into a transactional `Frame`, and `headless.Root` is the program boundary
  that commits every nested `Snapshot` only after a complete logical draw. Containers,
  stacks, fields, lists, editors, viewports and dressed controls route input against
  that committed frame; scroll bounds and transcript reflow use the same transaction.
  Finished drawable content is now the separate passive `headless.Block` contract and
  enters a live tree explicitly through `headless.Static`. `Paragraph.LinkAt` takes a
  width and computes its answer instead of publishing hidden hit geometry from Draw.

- **Host input is now one causal lifecycle.** `program.Host.Input` returns an
  `EventSource` whose event channel is followed by its terminal `Err` result. The old
  bare `Host.Events` contract could not distinguish clean EOF from a known terminal,
  pipe, or remote transport failure and has been removed.

- **Transcript identities survive prefix release.** `headless.Transcript.Append`
  returns a stable `BlockID`; block operations and sticky headers use that identity
  instead of a slice index. `StartRow` and `EndRow` describe the absolute live row
  range, while `Height` now reports only rows the program still owns. The obsolete
  `Committed`, `CommittedRows`, and row-cache `Generation` model is removed.

### Fixed

- **Windows sessions now receive later resize events.** Unix retains its owned
  `SIGWINCH` subscription; Windows samples console geometry inside the terminal
  adapter, coalesces unchanged observations, and stops the observer with the session.
  Both paths feed the same ordered `input.Resize` stream without splitting ownership
  of the console input handle.

- **Modal pointer ownership now follows the visible stack.** A layer that accepts a
  press keeps its drag and release after the pointer leaves its box. Wheel and move
  events outside the top layer route through visible lower layers to the Stack-owned
  base, so a dialog no longer makes the transcript behind it silently unscrollable.

- **Partial output failures now have an end-to-end ownership proof.** A writer that
  accepts an unknown frame prefix and fails is never retried and no later frame is
  written; `Run` preserves the cause. A real-PTY teardown fault also proves that a
  failed restore sequence does not skip the independent raw-mode restoration attempt.

- **Input transport failures now reach `program.Run`.** The terminal pump records its
  read result before closing the event channel; clean EOF remains a successful end,
  while a known failure is returned with its cause intact.

- **Committing output now transfers ownership in memory as well as on screen.** A
  committed prefix has its payload references cleared and its placement storage
  removed with amortized compaction. Scroll, selection and sticky state advance with
  the released prefix, and background search keeps a row snapshot only for the scan
  using it. Deterministic storage checks and an isolated `N` versus `2N` heap test lock
  the bounded-lifetime contract.

## [0.0.5] — 2026-08-06

Every module moves. The shape of what a program is handed changed, so this is the
release that breaks most: `Loop` is gone, `Host` is three methods and a set of
optional ones, and layout speaks `image` rather than `grid`.

### Changed

- **Runtime is a concrete resource, not a provider-owned wide interface.** `Loop` and
  `InlineLoop` are replaced by `*program.Runtime` and `*program.InlineRuntime`.
  Background work keeps the concrete, zero-safe `Runtime.Dispatcher()` handle and
  posts only its state transition back; consumers needing fewer runtime operations
  declare that interface locally. Terminal facts, clipboard, session control and
  image transport are concrete `Environment`, `Clipboard`, `Session` and `Images`
  subresources instead of a flat capability catalogue.
- **A host is transport plus optional capabilities.** A `program.Host` now implements
  only `Events`, `Size` and `Writer`. The last returns the consumer-defined
  `program.FrameWriter` interface instead of exposing `*term.Writer`. Terminal
  independently useful services are named by single-method consumer interfaces:
  `GroundHost`, `WheelHost`, `KeyboardHost`, `DirectoryHost`, `CopyHost`,
  `PasteHost`, `HandoverHost`, `TitleHost`, `BellHost` and `NotifyHost`. Implementing
  one never silently depends on implementing its neighbours. `ImageHost` remains a
  coherent group because image transport and its placement geometry form one
  protocol.
- **Layout is pure standard-library geometry.** `layout.Size`, `layout.Rows`,
  `layout.Columns` and every `grid.View` dependency are removed. Layout accepts
  `image.Point`, produces `image.Rectangle` through `Axis` and `Flow` objects, and a
  `grid.View` projects those results with `View.Subs`. Flex weights are normalized to
  an architecture-safe relative range, so allocation stays in bounded integer
  arithmetic even for hostile values.
- **Protocol packages no longer prescribe widget contracts.** The unused
  provider-owned `grid.Drawer` and `input.Handler` interfaces are removed. Drawing
  and event method sets now live in `program` and `headless`, where they are consumed.
- **Input decoding and interaction policy are separate layers.** `core/input` retains
  terminal events plus serializable `Chord` and `Keys` values; actions, bindings,
  sequence progress and timeout policy move from `input.Keymap` to `keymap.Map`.
  `Action.Does` is removed because turning identifiers into labels is appearance
  policy, now owned by `kit`. The provider-owned `headless.Spoken` interface is also
  removed; the spoken-form adapter declares the smaller method set it consumes.
- **Link hits retain their domain value.** `Paragraph.LinkAt` returns a complete
  `link.Link` instead of flattening it to a string, preserving file line and column.
  Absolute file hyperlinks are encoded with `net/url`, so paths containing spaces or
  reserved characters remain valid OSC 8 targets. Detection returns the rich
  `link.Links` collection, whose `At` method replaces the detached package function.
- **Grid inspection cannot mutate storage.** `Surface.CellAt`, `View.CellAt` and
  `Surface.Row` return copies. `CellAt` uses the standard `(Cell, bool)` lookup shape
  instead of returning a misleading pointer to a detached copy. Code that only
  changes appearance uses `View.MergeStyle`; content and links continue to go through
  drawing operations that preserve wide-grapheme pairs.
- **The `core`, `internal` and `ptytest` Go floor is 1.25.** It is the lowest version
  required by the current `x/sys` and `x/term` dependencies; using a Go 1.26-only
  error helper on Windows no longer makes those modules claim a higher floor. The
  complete workspace remains on 1.26 until a coordinated release lets higher modules
  replace their existing 1.26-tagged Oolong dependencies.

### Fixed

- **A slow terminal cannot fill a hidden fixed-size queue and stop its UI.** The
  program dispatcher and terminal writer use concurrency-safe FIFO storage with
  coalesced wake-ups. Sequence assignment and frame insertion are one operation, and
  closing concurrently with producers cannot reorder frames or make `Drain` skip an
  older blocked write. `Runtime.Every` also keeps at most one tick pending while the
  interface is busy instead of growing a catch-up queue.
- **Display ownership never changes behind an unwritten frame.** Shutdown and
  `Session.Hand` now report `program.ErrFrameTimeout` when the frame writer cannot
  drain; handover does not run its callback in that state. `Session.Suspend` also
  requires a real `HandoverHost` instead of suspending while a custom host remains
  active, and a component builder returning nil is rejected at startup.
- **Geometry has defined boundary behaviour.** Negative extents and gaps normalize to
  zero, oversized insets and margins produce an empty rectangle, and aggregate sizes,
  fractions, flex weights and rectangle endpoints saturate instead of wrapping.
  Surface allocation reports an overflowing area explicitly rather than reaching an
  accidental slice panic.
- **Ambiguous interaction is refused instead of guessed.** A key binding that is also
  a prefix waits for its longer sequence, including stateless lookup. Spoken choices
  accept a unique exact label before prefixes, while duplicate labels and yes/no words
  sharing a prefix are reported as ambiguous.
- **Repository checks cover module boundaries without the workspace.** CI also tests
  each module with `GOWORK=off` and explicit local replacements, exposing undeclared
  or upward dependencies that `go.work` would otherwise mask. The Go 1.25 floor also
  compiles Linux, Windows and Darwin source and test sets, so platform-specific files
  cannot silently raise a module's declared floor.
- **The architecture rules say which way a dependency may point, not which ways it
  may not.** Each ring now declares its immediate dependencies and the rest is
  computed: the transitive closure decides whether an import is allowed, and two more
  checks refuse a ring nobody declared and a graph with a cycle in it. The rules
  themselves went from fourteen lists of everything a ring must avoid — which had to
  be edited in thirteen places to add a ring — to seventeen edges, which have to be
  edited in one.
- **A compiled example is no longer source.** The tracked `streaming` Mach-O binary is
  removed and root-level example binaries are ignored.

## [0.0.4] — 2026-08-06

Nothing exported changed. This is the tests and the inside of four functions.

`markdown` and `highlight` did not change and stay at `0.0.3`, still asking for
`core v0.0.3` — which is the version they were built against and which has not gone
anywhere. `ptytest` changed only in its own tests and takes `0.0.4` anyway: a tag
names a state of a module, and its files are not the same ones `0.0.1` named.

### Fixed

- **The test hosts put their writer away.** `term.NewWriter` has a goroutine behind
  it, and neither the `program` package's fake host nor the demonstrations'
  `fake.Host` ever closed theirs, so every test left one running. Both are now given
  the test and close the writer when it ends — a test's context is cancelled before
  its cleanups run, so whatever was drawing has been told to stop by then.

### Changed

- **A test drives the clock instead of waiting on it.** The tests that have to prove
  a negative — a stopped clock ticks no more, an idle program writes nothing, a burst
  of updates does not become a frame each — run inside a `testing/synctest` bubble.
  Waiting a while and looking again only ever proved that nothing had happened *yet*,
  and it cost the wait every run; a bubble's clock moves only when every goroutine in
  it has nothing left to do, so the same sentence is now about the program rather than
  about the machine it ran on. It also holds the program to a claim nobody had written
  down, which is how the leak above was found: a bubble does not end until the
  goroutines inside it do.
- **Three hand-rolled waiting loops are gone.** The one that watched a variable until
  a component had been built is a channel, which says the same thing at the moment it
  becomes true; the one that waited for an inline interface to draw is the `waitFor`
  that was already there; and the one in the search tests waits for the answer to be
  sitting unread, which is what that test is about, rather than for fifty
  milliseconds.
- **Current Go where it was not.** `slices.SortStableFunc` sorts a table's permutation
  rather than `sort.SliceStable`, which sorted it through reflection; `errors.Is` and
  `errors.AsType` where a bare comparison and an out-parameter were; `strings.SplitSeq`
  where a slice was allocated only to be ranged over; `t.Context()` in every test that
  had reached for `context.Background()`. `go fix -diff` reports nothing across all
  seven modules.

## [0.0.3] — 2026-08-06

First tagged version of `highlight`, which starts at this repository's version for
the reason `markdown` did. `ptytest` did not change and stays at `0.0.1`.

### Added

- **A frame can keep room for what a cell cannot hold.** `grid.Painter` and
  `View.Paint`: a region something else writes into, with one rule — a painter must
  leave the cursor where it found it, which is what makes it possible in a block
  whose whole position is relative and is exactly the property an image protocol
  needs to be usable in a region that redraws. Regions are named by what is in them,
  so an unchanged one writes nothing, a moved one is erased before it is painted, and
  a full repaint says both again. `graphics.Image` is a painter; `term.Transmit`
  sends one and `term.CellSize` says how big a cell is; `kit.Image` places it and
  draws the alternative text where there is no picture, no cell size, or no terminal
  that draws them.
- **A span carries where it points.** `text.Span.Link` survives wrapping, truncation
  and drawing, which byte offsets into the original text cannot — so the decoder
  keeps a command's hyperlinks and markdown puts an address on the words rather than
  in brackets after them.
- **`grid.Render`** turns a drawing into rows of text, for a program with no
  terminal: output being piped, a run under a build server, a transcript in a file.
- **`highlight`, a module of its own**, carrying chroma: source code into styled
  lines, which is exactly the function a markdown look asks for. One line plugs it
  in, and nothing of the highlighter reaches its API — a style is its name, a
  language is its name, and what comes back is text.
- **Seven more examples**, shallowest first: a key count, a form that is also
  answerable in words, a picker, a two-pane file browser, a dashboard of tabs and a
  sortable table, a command runner that reads its child's colours and hands the
  terminal to `$EDITOR`, and an answer arriving a few characters at a time. Each has
  a test that drives it without a terminal, which is what `program.Config.Host` is
  for.
- **A list can hold the keyboard**, so it can be one of a container's children —
  which is what a two-pane interface is made of. A dressed tree, tabs, form and
  dialog pass the keyboard and the events through as well, so a widget with a look on
  it is a widget like any other. Without the last of those the news stopped at a
  dialog's frame: a stack hands the keyboard to the layer on top, and a form inside
  one took every keystroke while drawing no caret.
- **Handing the terminal over works on Windows**, over a wait on the console and an
  event. Whether it can is now a question about the session rather than the platform:
  a console can be waited on, and a pipe pretending to be a terminal cannot.

## [0.0.2] — 2026-08-06

First tagged version of `markdown`, which starts at this repository's version
rather than at its own: the changelog is one list and a module's tag says which
entry of it a checkout is. `ptytest` did not change and stays at `0.0.1`.

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
