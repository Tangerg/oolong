# Changelog

All notable changes to this repository. Modules are tagged separately —
`core/vX.Y.Z`, `components/vX.Y.Z`, `markdown/vX.Y.Z`, `highlight/vX.Y.Z`,
`latex/vX.Y.Z`, `ptytest/vX.Y.Z`, `ssh/vX.Y.Z` — and share one version number: a release is a state of the
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

The command registry now owns only reusable command mechanics: canonical names,
aliases, descriptions, fuzzy ranking and recency. `headless.Commands[T]` carries an
opaque caller value beside that metadata, so an application keeps behavior and
registration in one place without teaching the framework its argument shape or
execution callback. Slash-prefixed parsing has moved into the agent example that owns
that product grammar.

Command search now uses one ranking path for empty, canonical-name and alias queries.
Fuzzy score leads within name and alias matches, recent use breaks equal scores, and a
name match remains ahead of every alias match because the canonical name is what the
result presents.

Clipboard read ownership now expires at its deadline, not one clock tick after it.
Issuing a replacement and accepting an answer share one live-request predicate, so
they cannot disagree about who owns an unidentified OSC 52 response at the boundary.

`program.(*Runtime).After` now schedules a non-positive delay for the next owner turn
instead of silently discarding its callback. It still never calls application code
inline. `Runtime.Every` continues to schedule nothing for a non-positive interval,
because a repeating clock with no positive period has no stable cadence.

Controlled scalar operations now report whether the caller-owned accessor actually
accepted a different value, rather than reporting that the component merely requested
one. Accessor validation and normalization therefore remain the single source of truth
for `Slider.Set`, `Slider.Sync`, and tab synchronization results.

The same accessor postcondition now governs every controlled field. Text immediately
adopts a normalized or rejected edit and settles externally supplied text onto its
one-line storage form; single and multiple choice fields likewise reconcile their
cursor or chosen set to what the owner accepted. `MultiSelect.Toggle` no longer reports
a change rejected by its accessor.

Every exported mutable owner now states and mechanically enforces its no-copy contract
directly, including headless controllers, frame-local scroll refinement and kit
appearances that own committed routing state. The architecture gate follows value
containment transitively across repository packages, so a future wrapper cannot become
copylocked through a nested owner without documenting and marking its own identity.

Breaking. `headless.Command.Takes` and `headless.Command.Run` were removed. Store that
application policy in the value passed to `headless.(*Commands[T]).Add`; retrieve both
the command description and value from `headless.(*Commands[T]).Lookup`.
`headless.Parse` was removed; parse the application's command syntax at its input
boundary.

Every module now requires Go 1.27. The repository has one language floor rather than
letting workspace builds hide a newer dependency from modules that claimed an older
one. CI exercises every module independently with `GOWORK=off` on the initial Go 1.27
release as well as the current patch release.

Generic methods are used where the receiver owns a boundary but an individual
operation introduces its own result type. The runtime test owner now observes typed
results from its goroutine through one `running.fromLoop[T]` method instead of a
package function that repeated the test owner beside its dispatch target. The production
generic audit found no erased-value API to repair: generic component state already
lives on generic receiver types, while `Bind`, `Options`, and generic constructors are
package-level creation operations and remain functions rather than acquiring
artificial namespace receivers.

Breaking. Go 1.27 is the minimum supported toolchain for every module.

The pinned formatter, linter, reachability, API-diff and vulnerability toolchain is
updated to revisions that understand the Go 1.27 syntax. In particular, gofumpt is
pinned to the first upstream revision with method-type-parameter support rather than
silently dropping formatting enforcement until its next tagged release.

Terminal feature requests and terminal screen ownership are now different types.
`program.Config.Root` or `Inline` is the sole rendering-model decision; its
`Terminal` field can request mouse, focus, keyboard and capability probing but can no
longer express a contradictory alternate-screen choice that validation must reject or
the runtime must overwrite. `program.Config.TerminalConfig` is the one adapter
projection used by local and SSH transports, combining those shared `term.Features`
with the screen selected by the program root.

Breaking. `program.Config.Terminal` now accepts `term.Features`. Direct terminal
owners put the same feature value in `term.Config.Features`; `term.Config.AltScreen`
remains only on that lower-level session configuration.

`kit.NewForm` no longer materializes a default key map into its caller-owned
`headless.Form`. The appearance resolves the same public defaults read-only for help
text, so constructing a view cannot mutate controller behavior and a later explicit
key map still drives both input and hints.

Lower-layer documentation now rejects unqualified references to uniquely owned upper
types as well as package-qualified links. The same dependency direction therefore
governs API vocabulary even when a concrete type was written as `Runtime.After`
without its package name.

`term.LogTo` has been removed. Opening and configuring a diagnostic file is ordinary
application logging policy; the terminal neither owns that file nor adds semantics to
`os.OpenFile`, so keeping a wrapper in infrastructure created a second entry point for
an operation outside its responsibility.

Construction APIs now name complete state with explicit Config values. Diff
appearance, hunks and initial line-number policy are one `kit.DiffConfig`; a panel's
box and focusable content are one `kit.PanelConfig`; and bounded ingress ownership is
one `program.ByteIngressConfig`. Runtime setters remain the one path for later state
changes. The invariant-free `kit.NewCell` spelling was removed because `kit.Cell`
already exposes the same two fields and normalizes its preferred width when measured.

`headless.Root` now owns root replacement through `SetContent` and exposes the next
tree through `Content`. The transaction boundary, committed input target and held
pointer gesture can no longer be bypassed by assigning a semantic owner field.

Clearing a `headless.Selection` now resets only its selected range and in-progress
click run. It preserves the caller's configured `Clicks.Within` policy instead of
silently replacing configuration while removing transient state.

Terminal image geometry now has one representation. Pixel sizes, cell sizes and
cell limits travel as `image.Point`, matching the rectangles and points already used
by the grid instead of splitting every two-dimensional fact into swappable integer
parameters. PNG configuration is read by the standard `image/png` decoder; graphics
owns terminal transport and no longer publishes a second partial PNG parser.

Kit appearances no longer mirror selected operations from their headless behavior
owners. A composer exposes its complete editor through `Editor`, while dialogs,
settings and trees expose their complete headless state through `Controller`.
The appearance still implements the capability methods required for composition,
but state has one discoverable API rather than an arbitrary façade subset that can
drift from the owner below. A source-derived gate rejects future one-call state
forwarders from kit to its controller or editor.

Component ownership changes are now idempotent. Reinstalling a demonstrably identical
viewport content, stack base, dialog appearance or body, or panel child no longer
fabricates a focus loss and gain. Containers, tabs and stacks likewise notify the
current holder only when either the holder or the outer focus state changes. Field
validation now consumes one actual focus-loss transition instead of treating every
later `Focus(false)` as another departure. The safe identity rule for open interface
implementations lives once in a module-private package shared by headless routing and
kit ownership boundaries.

The permanent lint gate now also enforces bounded public interfaces, unambiguous iota
blocks, production maintainability and the modern syntax available at the repository's
declared Go floor. All four analyzers were first run across every module with zero
production findings; maintainability deliberately excludes table-driven tests and
architecture registries whose job is to keep a complete contract together.

Mutable stream, key-map and rendering owners now make their no-copy contract visible
to `go vet` instead of relying on prose. In particular, copying a populated
`keymap.Map` can no longer silently create one binding slice and one stale lookup tree
that disagree. A source-derived architecture gate requires every exported type that
states this contract to carry a direct `copylocks` marker, and verifies that a private
marker still implements the methods the analyzer recognizes. The runtime reachability
gate excludes exactly those two static-only marker methods through a self-tested
filter instead of inventing calls that can never be part of the program.

That ownership contract is now checked in both directions. Every exported struct
with a direct `copylocks` marker must also tell its caller that it must not be copied,
so search workers, byte ingress, terminal sessions and writers, and both testing
harnesses no longer hide pointer-identity requirements in private synchronization
fields.

The polished passive transcript block is now `kit.Entry`: generic labelled text whose
label role is supplied explicitly by the application. The shared component no longer
names a speaker or decides whether content is a user's own rather than an answer;
conversation roles, process roles and their visual emphasis are product grammar.

Boolean inverses and degenerate range wrappers were removed from headless state.
Selection has one presence query, modal stacks have one cardinality query, and both
committed and frame-local scrolling reveal one inclusive range through the same
method. A single row is that range with equal endpoints rather than a second API.

Terminal directory reports now construct file URLs with `net/url`, preserving path
separators while escaping URL syntax, control bytes and platform separators through
one standard-library path. The handwritten partial percent encoder was removed.

The OSC 52 payload bound is now the constant `clipboard.MaxPayload`. A fixed protocol
fact no longer has a function-shaped accessor, so compile-time and runtime consumers
use the same one representation.

### Breaking API migration

#### core

- `clipboard.Limit` was removed. Use the byte constant `clipboard.MaxPayload`.

- `program.Config.Terminal` changed from `term.Config` to `term.Features`. Supply
  only optional feature requests there; `Root` and `Inline` now decide screen
  ownership exclusively.
- `term.Config.Mouse`, `term.Config.Focus`, `term.Config.Keyboard`, and
  `term.Config.Probe` moved together to `term.Config.Features`. A direct terminal
  owner uses `term.Config{Features: term.Features{...}, AltScreen: ...}`.
- `term.LogTo` was removed. Applications open and configure their diagnostic sink
  directly with the standard library or their logging package.
- `program.NewByteIngress` now accepts one `program.ByteIngressConfig`. Name the
  dispatcher, byte limit and owner-side consumer in that value.
- `programtest.New` now accepts `programtest.Config` beside the owning `testing.TB`.
  Name the test terminal's positive `Width` and `Height` there.
- `graphics.Image.Width` and `graphics.Image.Height` were replaced by
  `graphics.Image.Size`. Read `Size.X` and `Size.Y`; the point is in pixels.
- `graphics.Fit` now accepts three `image.Point` values for pixel size, cell size and
  cell limit and returns the fitted cell size as one point.
- `graphics.Image.Paint` and `grid.Painter.Paint` now accept the painted cell size as
  one `image.Point` instead of separate column and row parameters.
- `graphics.Inline` now accepts its cell box as one `image.Point`.
- `graphics.PNGSize` and `graphics.ErrNotPNG` were removed. Call
  `png.DecodeConfig` when an application independently needs PNG dimensions or format
  validation; terminal transmission still validates its own input and returns the
  standard decoder error with graphics context.

#### components

- `headless.Commands` now has a caller-value type parameter. Declare, for example,
  `headless.Commands[Action]`, pass the value beside its description to `Add`, and
  receive the description, value, and presence result from `Lookup`.
- `headless.Command.Takes` was removed. Keep argument policy in the value registered
  with `headless.Commands[T]`.
- `headless.Command.Run` was removed. Keep execution behavior in that same opaque
  caller value; the registry never invokes it.
- `headless.Parse` was removed. Parse slash commands or any other product input
  grammar in the application before searching the registry.
- `kit.Message` was replaced by `kit.Entry`. Put the generic source name in `Label`,
  and put application-owned emphasis such as `theme.Accent` in `LabelStyle`; there is
  no replacement for the product-specific `Speaker` and `Own` decisions.
- `headless.Copyable` was renamed to `headless.TextProjector`. Its `Rows(width)`
  method projects meaningful text for selection, search and copying; the name no
  longer implies that a mutable Go value is safe to copy.
- `headless.(*Selection).Empty` was removed. Use `!selection.Active()` so selection
  presence has one query.
- `headless.(*Stack).Empty` was removed. Use `stack.Depth() == 0` so layer count has
  one query.
- `headless.(*Scroll).Reveal` and `headless.(*ScrollLayout).Reveal` now accept the
  inclusive `first, last` row range. Pass the same row twice for one row.
- `headless.(*Scroll).RevealRange` and `headless.(*ScrollLayout).RevealRange` were
  removed; use the corresponding `Reveal(first, last)` method.
- `headless.Root.Of` was removed. Use `root.SetContent(widget)` to replace the next
  tree and `root.Content()` to observe it; `headless.NewRoot` remains the sole
  construction entry.
- `headless.(*Table[T]).Unsorted` was renamed to `headless.(*Table[T]).ClearSort`. The
  name now describes a state-changing operation rather than reading like a bool query.
- `kit.NewDiff` now accepts one `kit.DiffConfig`. Put `Theme`, `Glyphs`, `Hunks`, and
  the optional initial `Numbers` policy in that value.
- `kit.(*Diff).ShowNumbers` was renamed to `kit.(*Diff).SetNumbers`, matching the
  other explicit lifetime mutations on Diff.
- `kit.NewPanel` now accepts one `kit.PanelConfig`. Put the complete `Box` and
  focusable `Content` in that value.
- `kit.NewCell` was removed. Construct `kit.Cell{Preferred: ..., Paint: ...}` directly;
  `kit.LabelCell` remains the one adapter for plain labels.
- `kit.(*Composer).Text`, `kit.(*Composer).SetText`, `kit.(*Composer).Empty`, and
  `kit.(*Composer).Reset` were removed. Use `composer.Editor().Text`, `SetText`,
  `Empty`, and `Clear`; the editor is the sole owner of content and history.
- `kit.(*Dialog).Show`, `kit.(*Dialog).Dismiss`, `kit.(*Dialog).Open`, and
  `kit.(*Dialog).Trigger` were removed. Use the same operations on
  `dialog.Controller()` so every dialog behavior remains discoverable on one owner.
- `kit.(*Settings[T]).SetItems`, `kit.(*Settings[T]).Items`,
  `kit.(*Settings[T]).Current`, `kit.(*Settings[T]).Selected`, and
  `kit.(*Settings[T]).Scroll` were removed. Use the settings `Controller()` for all
  list state and navigation.
- `kit.(*Tree[T]).Focused` was removed. Use `tree.Controller().Focused()` beside the
  tree's other hierarchy and selection operations.

## [0.12.0] — 2026-08-14

Every value here has one owner and one boundary where ownership changes hands, and
each of those boundaries now fails a test when it stops being one. An editor's text
has a single canonical storage rule, a single replacement operation and a single
grapheme-and-element settlement. A render projection owns its storage rather than
sharing slices with the editor it reflects, so drawing cannot reach into undo history
that belongs to someone else. A caller's accessor is the current value rather than a
seed that a component then shadows and lets drift.

The gates came with them. The draw purity comparison now includes complete undo, redo
and kill histories rather than only the text and cursor a reader would think to check;
a source-derived contract test requires every exported accessor owner to prove both
directions of ownership, so a new controlled component cannot inherit only the
convenient half; and `go vet` refuses to copy the mutable owner at all.

Consolidating the boundaries is what made them cheap. Once each question had exactly
one place to ask it, that place could be made to inspect only what its answer depends
on: ASCII cursor stepping, ordinary typing and byte-to-column projection no longer
rescan a logical line to prove something about a part of it they never read.

Editor text now has one canonical storage boundary. `text.Printable` replaces invalid
UTF-8 and removes control characters while retaining tabs; setting,
inserting, replacing, pasting and atomic-element insertion all pass through that same
rule. CRLF and carriage returns become logical line breaks in a multi-line editor and
spaces in a one-line editor, so byte offsets, cursor positions, element ranges and
drawn cells cannot disagree about invisible input.

Handled no-op edit actions, vertical movement and loss of focus now close the current
typing run. The next insertion therefore owns an undo snapshot even when it follows an
empty kill, yank, yank-pop, redo, cut or paste, a rejected control character, a move to
another visual row, or a focus round trip.

Drawing a `headless.Text` now builds an independently owned render projection instead
of applying semantic setters to a shallow copy of its editor. Draw therefore cannot
clear shared undo storage or mutate another slice-backed editor subsystem. The draw
purity gate now includes complete undo, redo and kill histories rather than observing
only current text and cursor state. `Editor` also carries the standard `go vet`
no-copy marker, turning future copies of the mutable owner into build-time findings.
The controlled and initialized branches now fill one explicit projection shape and
share the same one-line normalization boundary, so appearance and sanitization cannot
drift according to whether an event happened before the first frame.

ASCII cursor stepping and ordinary typing no longer rescan the complete logical line.
Shared immutable line-ending replacers remove per-keystroke construction, grapheme
settlement has a constant-time ASCII path, known cursor boundaries skip redundant
validation, and word movement walks each cluster once. Arbitrary external byte offsets
retain the complete Unicode validation path. Printable ASCII also has one shared
byte-to-column fast path for width, offset and column projection; Unicode, tabs and
controls continue through the complete grapheme and terminal-width authority. Offset
and column queries inspect only the prefix their answer depends on, so a cursor near
the start of a long line does not scan an unrelated suffix merely to prove it is
ASCII.

Editor cursor placement now has one grapheme-and-element settlement path. Public byte
ranges expand to whole grapheme clusters, `text.NextCluster` and `text.PrevCluster`
handle offsets inside UTF-8 and grapheme clusters symmetrically, vertical movement
maps terminal columns rather than copying byte offsets, and word movement cannot land
inside adjacent atomic elements. Insertion also settles segmentation that reforms
across its boundary, so subsequent edits cannot split the resulting cluster.

Transcript row windows now consistently mean the documented absolute half-open
interval. Drawing, visible-block lookup and copied rows share one saturating interval
intersection, including negative starts and counts that would otherwise overflow.

Breaking. `headless.Editor.SingleLine` and `headless.Editor.Mask` are now observed and
changed through methods. `Editor.SetSingleLine` and `Editor.SetMask` own the semantic
transition instead of allowing public fields to hide existing multi-line content from
the one-line renderer. Enabling either mode flattens existing line breaks once and,
when that changes content, preserves cursor and element offsets and advances
`Editor.Revision`. It always clears history that could restore content invalid in the
new mode. A mask must have stable, visible cell geometry.

An empty `text.Edit` now leaves every `text.Mark` unchanged. `Edit.Shift` recognizes
the identity edit after clamping, before atomic-mark reachability is considered, so a
zero-width no-op inside an atomic mark cannot delete it while a real insertion still
does.

Atomic editor elements now canonicalize every line break before insertion, independent
of the editor's own single-line mode. The stored text, mark range and returned
`headless.Element` therefore describe the same one-line body, and the ordinary
separator after an element cannot be swallowed by a range measured from the
pre-normalized input.

`headless.Editor.Revision` now exposes one monotonic, process-local observation token
for semantic content. Text input, programmatic editing, atomic elements, undo and redo
advance it once; cursor, selection, scrolling, focus, clipboard requests and handled
no-ops do not. Consumers can therefore drive persistence, validation and dirty state
from the editor's result instead of coupling those policies to today's keys and action
names. All range edits now pass through one internal replacement operation, which also
makes empty replacement inside an atomic element a true no-op rather than silently
dropping the element. Controlled `headless.Text` fields use the same revision boundary,
so handled cursor and no-op editing actions no longer write unchanged values through
their accessor. Real content replacements also settle the selection at that boundary,
so whole-text programmatic changes cannot leave an anchor outside the new content;
equivalent typing does not open an unsnapshotted undo run.

Caller-owned component state now has one meaning: the accessor is the current value,
not an initialization seed plus an eventually drifting private copy. Confirm uses the
same scalar ownership path as tabs, sliders and dialogs. Text, single-choice and
multiple-choice fields project later caller writes during drawing and reconcile their
derived editor, cursor or taken-set state at the next semantic operation. A
source-derived contract test requires every exported accessor owner to prove both
directions, so adding a controlled component cannot silently test only component-to-
owner writes. Re-selecting a tab or confirmation, navigating at a boundary, repeating
an unchanged spoken answer and opening or dismissing a dialog twice remain handled
operations where appropriate without becoming false persistence events. Multi-choice
limit changes settle directly against the final limit and cannot publish an
intermediate canonical value. Tabs, sliders and dialogs now consistently report from
`Sync` whether reconciliation changed their correlated state.

Newline insertion uses the editor's sole replacement path, so it replaces a selection
and a rejected single-line newline closes rather than corrupting the surrounding undo
run. Atomic-element cursor settlement is total for the zero editor, and its storage
boundary documentation now names the intentionally separate one-line element path.

### Breaking API migration

#### components

- `headless.Editor.SingleLine` changed from a public field to a getter. Assign with
  `headless.Editor.SetSingleLine` and observe with `headless.Editor.SingleLine()`.
- `headless.Editor.Mask` changed from a public field to a getter. Assign with
  `headless.Editor.SetMask` and observe with `headless.Editor.Mask()`.
- `headless.Text.Mask` was removed. Configure and observe the field's sole editor with
  `headless.Text.Editor().SetMask` and `headless.Text.Editor().Mask()`. Invalid masks
  now panic at that setter rather than later during `Draw`.
- `headless.(*Dialog).Sync` now returns whether stack membership changed. Existing
  statement calls remain valid; callers that schedule work from reconciliation should
  use the result instead of comparing open state around the call.
- `headless.(*Tabs).Sync` now returns whether focus or the stored selection changed,
  matching `headless.(*Slider).Sync` and giving all controlled scalar controllers one
  transition result.

## [0.11.0] — 2026-08-11

One semantic operation per entry point, and every rule about that now fails a test
when it stops being true. Section 4.2's six render prohibitions are all executable:
three by a type-resolved call-graph analysis over every `Measure` and `Draw*` entry,
one by a callback-phase registry, one by staging state that a second producer cannot
overwrite, and one by the projection comparison that was already there.

This is also the largest source break in the project's history. The migration ledger
below is derived from the published tags by `apidiff` on every push; an incompatible
change cannot ship unless its exact name appears there.

Breaking. Tabs, sliders, and dialogs now have one constructor per concrete type.
Their explicit `Config` values select local or caller-owned state through an optional
typed accessor, so ownership no longer creates parallel `New` and `NewControlled`
APIs. Kit configurations include behavior, content, and appearance in one value and
construct exactly one headless controller internally; callers that need behavior
without the kit appearance construct the headless type directly.

Long positional kit constructors for forms, trees, and settings now take named
`Config` values. A dressed form reads help bindings from its headless controller
instead of carrying a second key map, so displayed shortcuts and accepted shortcuts
cannot drift. Trees keep an explicitly supplied headless controller because dressing
must not replace its caller-owned hierarchy, while settings construct their sole
controller from one complete value.

Exact semantic aliases are removed. Emptying a clipboard is the protocol's existing
`Channel.Copy(selection, "")` operation, reported through the same success result as
every other copy. `KeyboardFlags.Features.Has` is the one feature-set query, and a PTY
session uses its standard `io.Writer` contract with `io.WriteString` instead of a
second text-writing method. A directly owned terminal suspends through
`Terminal.Hand(term.Suspend)` instead of a forwarding method. Architecture tests now
reject undeclared multiple exported `New...` or `Open...` entries returning the same
concrete type, turning the one-construction-language rule into a repository invariant.
The process-terminal `term.Open` and caller-owned-PTY `term.OpenOn` entries are an
explicit, live ownership-boundary declaration rather than an invisible naming-rule
exception. The pinned `apidiff` gate now requires an exact migration entry for every
source-incompatible API change, including signature changes and interface growth,
instead of guarding removals alone.

Breaking. `kit.Tree` now has pointer receivers throughout, matching its constructor
and every other controller-backed kit component instead of allowing copied
controller values to masquerade as independent widgets.

Controlled tabs now normalize an out-of-range bound selection through the binding
itself before settling focus, so caller state, active content, and keyboard routing
cannot disagree. PTY session and transcript waits preserve `context.Cause`, allowing
test harnesses to distinguish an ordinary cancellation from the failure that caused
their surrounding operation to stop.

LaTeX symbol resolution retains only successful terminal mappings. Arbitrary invalid
macro names therefore cannot grow process-global state, while measured repeated
formula rendering keeps the bounded cache's lower latency and allocation cost.
Wheel gesture evolution and transcript viewport geometry now live with their state
objects, and inline and screen painters share one numeric CSI encoder.

Inline publication no longer emits erase-to-end after a row fills the terminal's
right margin, preserving its final cell across ASCII, wide text, styled text and
hyperlinks. The PTY assertion screen now models a terminal's pending-wrap state, so
the original failure and future right-margin regressions are observable in tests.
Input transports stamp parser batches through one `input.Stamp` operation instead of
maintaining local key and mouse type switches. `highlight.Styles` now returns the
same `highlight.Style` values accepted by `highlight.New`. `text.Line.Wrap` keeps
per-cluster presentation by reference and reserves its bounded working sets up front,
substantially reducing allocation without adding mutable cache state. Every method on
a nil `*clipboard.Channel` is now uniformly inert; the useful zero value remains the
direct OSC 52 path.

Repository gates now reject unreachable private implementation and public operations
without executable contract coverage across Linux, macOS, and Windows with a pinned
`deadcode` analyzer. Repository usage is explicitly not an API-retention criterion:
downstream capabilities survive unless an independent design review finds their
responsibility or abstraction unsound. White-box tests must use the explicit
`_internals_test.go` suffix; ordinary tests remain caller-side, making private
coupling and public evidence mechanically reviewable.

Public APIs now enforce one semantic operation per entry point. Terminal and PTY
construction use explicit `term.Config` and `ptytest.Config` values; the former
`Options` types are removed. `ptytest.Start` accepts its configuration directly and
transcript waiting is context-only; `headless`
constructs both row and column containers through `NewContainer`; the useful zero
`Editor` replaces its redundant constructor; `kit.Transcript.Commit` takes one
explicit limit instead of splitting bounded and unbounded publication across two
methods; and `Box.InnerRect` is the sole geometry-only interior query. Highlighting
now uses one reusable concrete `highlight.Renderer` for standalone and Markdown
composition, LaTeX composition calls the same rich `latex.Render` entry directly,
and image-protocol detection has one explicit `graphics.Detect` function. Link
detection now takes its optional filesystem predicate on the sole `link.Detect`
entry; wheel report conversion uses one timestamp-aware `Advance.Rows` method; and
ANSI parameters use indexed `Params.At` without a first-element alias. Layout now
has one `Flow` entry for rectangle placement, size allocation, and wanted extent;
the no-gap case is an explicit zero-gap flow rather than parallel `Axis` and
package-level convenience APIs.

Automatic colour is now a fact of the driven host rather than the process running
the framework. `term.DetectDepth` takes one explicit environment lookup, local and
SSH terminals freeze the result as `ColorHost`, and `program.Environment.Color`
exposes the same resolved fact. A custom host that omits the capability safely draws
without colour instead of accidentally consulting the server process environment.

Rendering purity now has executable coverage for intrinsic effects as well as state.
Architecture tests derive every production `Measure` and `Draw*` entry, follow its
type-resolved package-local call graph, and reject goroutines, I/O, clocks, randomness,
logging, publication, and callbacks assigned to a non-projection phase. Every retained
callback has one exact, anti-stale phase declaration, while component suites execute
every render receiver twice and keep event-callback counts in the observed state.
Filter matching and paragraph path discovery now happen at semantic update boundaries,
never lazily from `Measure` or `Draw`.

Presentation state now has one producer in each root frame. Snapshots, scrolls, and
transcript placement reject duplicate staging instead of letting sibling order choose
the last pending value; rich frame-local layouts expose explicit refinement behavior,
and frame generations prevent a saved frame from becoming valid during a later draw.
`ScrollLayout.Resize` refines the one staged scroll layout used by sticky transcripts.

### Breaking API migration

This is the complete source-breaking ledger relative to v0.10.0. CI derives it again
from the immutable module tags with the pinned `apidiff`; an incompatible exported API
change cannot ship unless its exact API name appears in this section.

#### components

- `headless.NewControlledDialog`, `headless.NewControlledSlider`, and
  `headless.NewControlledTabs` are folded into `headless.NewDialog`,
  `headless.NewSlider`, and `headless.NewTabs`; set the accessor field on their
  explicit `Config` values for caller-owned state.
- `kit.NewControlledDialog`, `kit.NewControlledSlider`, and `kit.NewControlledTabs`
  are folded into the corresponding `kit.New...` constructor. Each kit `Config`
  now carries both ownership and appearance and exposes its constructed controller
  through `Controller`.
- `kit.NewDialog`, `kit.NewSlider`, and `kit.NewTabs` now take their corresponding
  explicit `Config` value. Set `Open`, `Value`, or `Selection` there when state is
  caller-owned; the same constructor handles locally owned state when it is nil.
- `kit.NewSettings` now takes `kit.SettingsConfig`, replacing five positional values
  with named item, projection, action, key-map, wrapping, and width settings.
- `kit.NewTree` now takes `kit.TreeConfig`; its required controller and text projection
  are named alongside appearance and indentation.
- `kit.Form.Keys` is removed. Set `kit.Form.Controller().Keys` (normally through the
  controller supplied to `kit.NewForm`) and the hint row reads that same map.
- `headless.(*PointerRegion).Clear` is removed. Stage an empty rectangle and nil target
  through `headless.(*PointerRegion).Stage`, the sole operation for replacing its
  frame-local region.
- `kit.Paragraph.Links` and `kit.Paragraph.Exists` are removed. Call
  `kit.(*Paragraph).SetLinks` with `kit.LinkConfig`; detection then runs when link
  configuration or text changes rather than during frame projection.
- `headless.Columns` and `headless.Rows` are replaced by
  `headless.NewContainer`, whose axis makes the variant explicit.
- `headless.NewEditor` is removed; the useful zero `headless.Editor` is the sole
  construction path.
- `kit.(*Transcript).CommitN` is folded into `kit.(*Transcript).Commit`, whose limit
  is explicit at the one publication entry point.
- `kit.Box.Inner` is replaced by the geometry-only `kit.Box.InnerRect`.
- The value method set `kit.Tree[T].Controller`, `kit.Tree[T].Draw`,
  `kit.Tree[T].Focus`, `kit.Tree[T].Focused`, `kit.Tree[T].Handle`, and
  `kit.Tree[T].Measure` is removed. Keep the `*kit.Tree[T]` returned by its
  constructor; copying a controller-backed tree never made an independent widget.

#### core

- `clipboard.(*Channel).Clear` is replaced by
  `clipboard.(*Channel).Copy(selection, "")`, the protocol's single spelling for
  emptying a selection.
- `input.KeyboardFlags.Has` is replaced by
  `input.KeyboardFlags.Features.Has`, keeping membership on the feature-set type.
- `ansi.Params.First` is replaced by `ansi.Params.At(0)`.
- `graphics.Place` and `graphics.Delete` are folded into `graphics.Image.Paint` and
  `graphics.Image.Erase`; the image now owns operations that require its identity.
- `graphics.DetectIn` is replaced by the sole `graphics.Detect` entry.
- `input.(*Advance).At` and `input.(*Advance).By` are replaced by
  `input.(*Advance).Rows`, which owns timestamped gesture evolution.
- `layout.Axis.Rects`, `layout.Divide`, and `layout.Wanted` are replaced by methods
  on one explicit `layout.Flow` value.
- `link.DetectIn` is folded into `link.Detect`, whose optional filesystem predicate
  is explicit.
- `term.DetectDepthIn` and `term.DetectLocaleIn` are replaced by `term.DetectDepth`
  and `term.DetectLocale`, both of which require the driven terminal's environment
  lookup.
- `term.Options` is replaced by `term.Config`.
- `program.Config.Terminal` now has type `term.Config` rather than the removed
  `term.Options`; its field name and zero-value behavior are unchanged.
- `term.DetectGraphics` has no process-global replacement. An opened terminal owns
  the answer through `term.(*Terminal).Graphics`; an adapter that owns terminal facts
  calls `graphics.Detect` directly.
- `term.(*Terminal).Suspend` is replaced by `term.(*Terminal).Hand(term.Suspend)`.
  `program.Session.Suspend` remains a distinct host-capability operation: unlike a
  generic handover, it must not stop a process when a custom host has no handover.

#### highlight

- `Background`, `Lines`, and `Of` are replaced by the reusable `Renderer` returned
  by `New`; standalone and Markdown rendering now use the same object.
- `Styles` now returns `[]Style`, the values accepted directly by `New`, instead of
  requiring every caller to convert names from `[]string`.

#### latex

- `Of` is removed. Call `Render` inside the consumer-owned Markdown adapter so the
  same rich `Formula` result remains observable in every composition.

#### ptytest

- `(*Session).Type` is replaced by `io.WriteString(session, text)`; `Session` already
  implements `io.Writer`.
- `(*Transcript).WaitWithin` is replaced by `(*Transcript).WaitFor` with a caller-owned
  context.
- `Options` is replaced by `Config`.
- `StartWith` is folded into `Start`, which accepts that `Config` directly.

## [0.10.0] — 2026-08-10

Optional content is a fork rather than a chain, and the claims this repository makes
about itself now have guards that fail when they stop being true.

0.9.0 was prepared but never tagged: its gate refused a half-finished documentation
move, and a version nobody can fetch does not belong in this file as one. Its notes
are part of this release.

The polished kit now proves all three caller-owned controller paths end to end.
Executable package examples cover controlled dialogs, sliders, and tabs; the
dashboard coordinates tab shortcuts and settings through bound application values,
and the streaming example observes its bound modal state without a shadow copy.

The program package is organized around its existing responsibilities — public
runtime contract, configuration, host boundary, event loop, output settlement,
inline publication, and clocks — without adding an interface or changing an exported
signature. GitHub Actions now hashes every module `go.sum` through one recursive
cache path, so the workspace root no longer disables setup-go caching and a new
module does not require another cache list entry.

Committed fuzz crashers can no longer become silent orphans when a target is renamed
or removed. The architecture gate matches every `testdata/fuzz/<Name>` directory to
an actual `func <Name>(*testing.F)`, and the parser's invalid-UTF-8 paste regression
now follows the broader valid-text property that owns it. Fuzz corpus files and
visual goldens both have repository-level LF contracts on every checkout platform.

The bilingual documentation now builds as a searchable VitePress site with stable
English and Chinese routes, task-oriented navigation, module selection, symptom-led
troubleshooting, and a fourteen-command learning catalog. CI compiles the site and
GitHub Pages deploys the exact checked artifact from `main`; generated files are not
committed.

Documentation tests now require every page to declare its purpose, keep one matching
Chinese route, preserve heading structure, and link every executable example exactly
once in both catalogs.

Same-repository GitHub links are now checked against the current checkout instead of
being mistaken for uncheckable web links. Missing source files, directories and
Markdown anchors therefore fail the architecture gate whichever link form a page
needs for VitePress.

`grid.Screen` and `grid.Inline` now compose one private frame buffer that owns their
front and back surfaces, dimensions, terminal ground, and colour depth. Resize,
frame start and buffer exchange consequently have one implementation, and colour
depth no longer has two candidate sources of truth.

Brand exploration batches are ignored by path, so only the two approved marks enter
the documentation site and a future batch is excluded by default. Documentation
audit, lint, route validation, and static compilation are now one locked
`npm run docs:check` gate shared by CI, Pages, contributors, and releases.
Visual golden files now declare LF as repository data, so Windows and Unix runners
compare the same bytes instead of inheriting each checkout's line-ending policy.
Documentation checks normalize CRLF at their input boundary, preserving Markdown's
platform-neutral semantics while still validating frontmatter and anchors on Windows.

Optional content is a fork rather than a chain. Markdown, highlighting and
mathematics are peers that terminate at `core/text`; none imports another, any subset
composes, and each is usable on its own. A program that shows no mathematics never
acquires a TeX parser, and one that shows no code never acquires a lexer.

Added the optional `latex` module. It parses a deliberately bounded mathematical
LaTeX subset into immutable, copyable terminal rows with fractions, roots, scripts,
symbols, Unicode/ASCII furniture, source-preserving errors, and no external process
or image-only path. `Formula` shares the repository's `grid.Drawable`, measurement,
and row-selection contracts; parser panics and hostile input do not cross its API.

Breaking. Markdown code highlighting and display mathematics now use one
`Look.SetRenderer` registry keyed by stable semantic blocks. `Look.Highlight` is
removed rather than kept beside a second extension API. `$$` blocks and fenced
`math` blocks both publish `DisplayMath`; fenced code publishes `FencedCode`. Parser
nodes remain private, sibling modules return only `core/text` lines, and missing
renderers preserve source.

Breaking. `core/text.UTF8Locale` and the unused `WrapAll` convenience function are
removed. `core/text.PrefersUnicode` is the single locale policy shared by component
and formula glyph selection, including the modern default for an absent locale.

The `read` example exercises the complete composition while arbitrary chunk
boundaries cross code and `$$` blocks: Markdown retains each open fence,
`highlight.Of` and `latex.Of` render their semantic bodies, and only finished blocks
are published to scrollback. Both optional modules retain their independent entry
points: highlighting returns styled lines and a formula is a measured, selectable
drawable without involving Markdown.

Fixed extension renderers now preserve the documented distinction between declining
with nil and intentionally producing no rows. LaTeX scripts allocate only the bands
they occupy, and unbraced numeric superscripts and subscripts compose in either order
despite the external parser's Go-number tokenization. CI derives local module
replacements from the workspace module list instead of maintaining two more lists.

Three focused commands now show the content layers before the streaming example adds
lifetime management: `examples/markdown` renders a finished document,
`examples/latex` draws a formula directly, and `examples/content` composes Markdown,
Highlight, and LaTeX through the consumer-owned semantic registry.

Documentation now provides one bilingual learning path from a core-only component,
through headless composition and optional content, to bounded streaming and a full
agent interface. Every step names one tested example, and the architecture gate
keeps the order, translations, root entry points, links, and executable slices from
drifting apart.

## [0.8.0] — 2026-08-10

One thing should exist once. A measured-drawing contract, a module list, a piece of
transcript geometry, a width memo, a documentation link: each of these had two copies
somewhere in this repository, and every pair could disagree without anything noticing.

Breaking. `kit.Diff` is now constructed with `kit.NewDiff`; its hunks and appearance
are changed through methods rather than exported fields. `kit.GlyphsFor` accepts an
already-resolved locale, and `program.Environment.Locale` obtains that fact from the
new optional `LocaleHost` capability. Clipboard read requests now report admission:
`PasteHost.Paste`, `program.Clipboard.Paste`, `headless.Clipboard.Paste`, and
`headless.Editor.Paste` return `bool`. There are no compatibility aliases.
`headless.MultiSelect.Limit` is replaced by `SetLimit` and `Limit`, so every change
settles the selected set through the controller instead of bypassing its invariant.

This round is also deliberately breaking. Frame-derived transcript and scroll
geometry now has one public path: `Scroll.Layout`, `Transcript.Resize`,
`Transcript.Layout`, and `Transcript.Draw` are removed in favour of `Stage` and the
layout value it returns. Permanent inline output likewise has one path:
`program.Printable` and `InlineRuntime.PrintRows` are removed; passive content shares
the lower `grid.Drawable` contract, and both `grid.Inline.Print` and
`InlineRuntime.Print` accept that object directly.
`kit.Message` now implements passive content through a pointer so it can own its
private wrap cache.

### Changed

- **The reader path now grows from the actual minimum.** `examples/hello` uses only
  `core` and states the complete two-method component contract. The README, first
  interface tutorial, streaming guide, testing guide, and example catalog then add
  layout, headless behavior, appearance, publication, and real-PTY proof in that
  order. English and Chinese task guides carry the same compilable starter program.

- **Module membership has one executable source.** CI and the coordinated release
  script both consume `scripts/modules.sh`, which derives the complete and public
  module sets from `go.work`. A module can no longer be added to the workspace while
  remaining absent from one of those gates.

- **Passive content pays for visible work.** Paragraphs, code gutters, messages,
  palettes, diffs, tables, line numbers, and Markdown all consume the clip already
  carried by `grid.View` instead of walking hidden rows. On an M4 with 10,000 retained
  rows and a 40-row viewport, steady-state code drawing falls from about 11 ms and
  799 kB to 0.12 ms and 5 kB; a message falls from about 12 ms and 34 MB to 35 µs and
  1.2 kB; and a 10,000-item palette falls from roughly 90–112 ms to 0.24 ms.

- **Measurement and publication have one vocabulary.** `grid.Drawable` is the
  lowest common measured-drawing contract. Headless blocks add lifetime meaning by
  embedding it, while the runtime and kit printer consume it directly. Callers no
  longer choose between a measured object and a separate row-count callback.

- **Search closure settles ownership synchronously.** `Search.Close` now waits for
  its worker to exit and for `Results` to close; it returns with no live corpus or
  unread result retained. Joining rows and matching them are separate cancellation
  phases, so a close or newer query arriving during the join does not also wait for
  a now-unobservable regular-expression pass.

- **A diff owns and memoises its physical layout.** Hunks are copied on entry and all
  mutations invalidate one private width cache, so measurement and drawing consume
  the same rows without exposing stale-cache states. In the committed frame benchmark,
  a 1,000-line diff at 100 columns and a 60-row viewport falls from about 23.2 ms,
  107 MB and 73,000 allocations per frame to about 0.65 ms, 1.4 kB and 60 allocations
  on an M4. Stale diff, paragraph, markdown document and stream caches release their
  references as soon as their source is replaced.

- **Clipboard backpressure is observable.** OSC 52 can correlate only one read at a
  time. A caller now receives false when a host cannot read or an unidentified request
  is still live, while accepted answers continue to arrive as ordinary paste events.
  The consumer-side editor interface and the runtime capability use the same shape.

- **Character locale follows the driven terminal.** Local terminals freeze the
  environment passed to `OpenOn`; SSH hosts expose the accepted client's locale; the
  kit interprets only that stable value. A server process's locale can no longer choose
  ASCII or Unicode furniture for a remote client.

- **Wrapped links are detected once per logical line.** Drawing and hit testing now
  share one row projection, so a long linked paragraph no longer rescans its complete
  source for every visible wrapped row. In the 40-row M4 benchmark this falls from
  about 28 ms and 281 allocations to 0.86 ms and 46 allocations.

### Added

- Documentation links and GitHub heading anchors are checked from the repository
  architecture module. The getting-started programs are extracted from both
  languages and compiled against the current local `core`; the example catalog
  similarly discovers every command and requires one entry, package comment, and
  test.
- Release and community facilities now include a maintainer release guide, private
  security policy, structured bug and capability reports, and a pull-request
  checklist tied to ownership, dependency direction, tests, and documentation.
- CI and the release gate pin `shfmt` for the scripts that derive module membership
  and execute coordinated releases; shell formatting is no longer a review-only rule.
- Example visual goldens can be deliberately regenerated with `go test -update`.
- Regression gates cover diff layout reuse and retention, locale precedence and SSH
  propagation, clipboard request admission, and a thematic break used as list content.
- Clipped-drawing benchmarks and tests cover large paragraphs, code, messages,
  palettes, and table callbacks, keeping viewport cost tied to visible work.

### Fixed

- Paragraph measurement now uses the same one-column floor as messages, diffs, and
  Markdown. A parent asking before it has assigned width no longer receives a false
  zero height, including when the paragraph's indent consumes the available width.

- The Go reference badge now points to the published `core` module rather than the
  intentionally empty repository root. Contributor commands include `ssh`,
  Dependabot scans every dependency-bearing `go.mod`, and stale documentation anchors now fail
  locally instead of becoming dead links after merge.

- `clipboard.Channel.Answer` now documents and tests its actual ownership rule:
  malformed or unrelated parameters leave the live request intact, while a matching
  selection with an unreadable payload settles it as a failed answer.

- Width memos are immutable snapshots. Copying a paragraph, code block, message,
  diff, or Markdown document after layout and then changing the copy can no longer
  clear or rewrite rows the original still considers valid. Markdown documents also
  detach their block storage before appending, so independently grown copies cannot
  overwrite one another through spare slice capacity.

- Shrinking a command history while its current entry is removed now leaves the walk
  immediately before the oldest retained entry; moving forward no longer yields
  successful empty entries. Multi-select limits also constrain initial bindings,
  option replacement, interactive toggles, and runtime limit changes through one
  enforcement path.

## [0.7.0] — 2026-08-10

An environment fact belongs to the terminal being driven, not to the process doing
the driving. Following that one sentence through the library is most of this release.

Breaking, and the widest set so far. Every function that reads the environment now
takes `func(string) (string, bool)` rather than `func(string) string`, because the
difference between unset and empty is a fact a remote session has and a `getenv`
shape throws away: `graphics.DetectIn`, `input.WheelFor`, `kit.GlyphsFor`.
`term.OpenOn` and `term.Options.Modes` take that lookup as an argument.
`term.Options.Keyboard` is an `input.KeyboardFeatures` set rather than a bool, and
the five `input.Kitty*` constants are `input.Keyboard*` of that type;
`input.KeyboardFlags.Flags` is `.Features`. The `clipboard` package's `Copy`,
`Clear`, `Request` and `Parse` are methods on `*clipboard.Channel`, `Parse` is
unexported, and `Selection`'s values are ordinals rather than their wire bytes.
`markdown.Block`'s six exported fields are gone; a block is measured and drawn
instead. There are no compatibility aliases.

### Changed

- **Terminal facts follow the terminal, not the process.** `term.Terminal` resolved
  its wheel profile and image protocol from `os.Getenv` on every call, so a process
  driving two terminals had one answer for both and a test could pin neither. They
  are now resolved once, from the lookup `OpenOn` was given, and frozen for the
  session. `os.LookupEnv` survives at exactly three entry points — `term.Open`,
  `DetectDepth`, `DetectGraphics` — where the terminal really is this process's own.

  Over SSH this is not a tidiness argument. The `ssh` host now reports the client's
  wheel profile, negotiates keyboard features against the client's PTY environment,
  and reaches the clipboard beside the user rather than one attached to the server
  process. It had none of those before, and what it did read was the server's `TERM`.

- **Keyboard enhancement is a feature set that is negotiated.** Asking for the whole
  Kitty protocol was one bool and one hardcoded `\x1b[>31u`. It is now a set the
  caller states, `term.KeyboardCompatible` is the portable subset — unambiguous codes
  and alternate keycodes, without releases or text — and `Options.Modes` derives the
  exact request from the driven terminal's environment. Two known-bad combinations are
  refused there rather than by a package global: VS Code's bridge under WSL, which
  acknowledges the mode and then loses what it carries, and iTerm2, which can leak a
  release into the parent shell.

- **The clipboard is a channel with one live request.** OSC 52 has no request
  identity, so a boolean "a paste is outstanding" could hand an unrelated future
  answer to whoever asked last. `clipboard.Channel` owns encoding, tmux DCS
  passthrough, one outstanding read, selection correlation and a ten-second expiry;
  `term` and `ssh` consume the same channel through one `input.OSC.Paste`, so neither
  adapter has its own copy of the rule. The carried size is 100 kB rather than 1 MiB:
  the far end's limit is silent, and a copy that reports success and vanishes is worse
  than one that is refused.

- **A markdown table keeps its cells until its width is known.** It used to be padded
  to its widest cell at parse time, which left drawing a choice between wrapping a
  pre-joined grid and cutting the last column off. A `Block` now retains cells,
  alignments and styles; column room is water-filled at the width it is given, and a
  table that cannot afford a readable column each becomes labeled records instead of a
  grid of vertical fragments.

- **A diff wraps rather than losing its tail.** `kit.Diff` had one ellipsis path that
  silently removed the right-hand end of every long line — in a review pane, the part
  of a proposed change nobody saw. Measurement and drawing now share one width-aware
  layout, continuation rows keep the sign, colour and gutter, and line numbers yield
  before they starve the content.

  This has a cost worth stating: the layout is a value rather than cached state, so a
  measured slot builds it twice per frame and pays for every line, not the visible
  ones. A 300-line diff at 100 columns is about 5.5 ms and 17 MB per frame on an M4.
  `markdown.Doc` caches by width behind a pointer; `Diff` does not yet.

- **`Suited` fits a theme to the terminal instead of picking one.** Body text stays on
  the terminal's own foreground, and neutral structure — subtle, muted, borders,
  surfaces, selection, scrim — is mixed from the reported foreground and background, so
  a pane reads as part of the user's terminal. Accent and outcome colours still come
  from `Dark` or `Light`. If either ground colour is unknown the built-in theme is
  returned unchanged, because guessing a missing colour can erase text.

- **`Divide` and `Wanted` answer from the same code.** They agreed by inspection and
  drifted in one case. Both now ask `Slot.wanted`, and the allocation itself is a
  `division` value that cannot have its remaining room updated without its weight sum.

- **Opening a terminal acquires in one direction and rolls back through one edge.**
  Each failure path used to assemble its own list of what to undo, which is a shape
  where adding a resource creates a path that forgets an older one. Acquisition,
  activation and rollback are now three functions, and goroutines start only after
  nothing left can fail. `program.Run` is split the same way, so a host that validates
  badly cannot partly construct an interface before the component builder runs.

### Added

- **Representative screens are guarded at more than one width and colour depth.**
  `examples/internal/visualtest` composes `programtest`'s in-process host with
  `ptytest`'s screen model, so an example asserts what a terminal would actually show.
  The agent review is checked at 44 and 90 columns. This is deliberately not a blanket
  snapshot policy: state transitions stay behavioural, and goldens guard the few
  layouts whose relationships are the behaviour.

### Fixed

- `Flex(0).AtLeast(n)` was counted as wanting `n` by `Wanted` and given nothing by
  `Divide`. A weight of zero asks for no proportional share; an explicit floor is
  still a real constraint.
- A thematic break as the only content of a list item kept its bullet's column and
  dropped the bullet. It now draws.
- An empty markdown table cell no longer depends on the parser having produced a line
  for it.

## [0.6.0] — 2026-08-10

The purity invariant grows a second half and a sharper instrument, and it immediately
finds something.

Breaking. `Writer.Progress` and `FrameWriter.Progress` are `Changes`, because progress
now means the terminal's own progress indicator and one word cannot be both.
`Accessor.Get` is `Value`, so a bound value is read and written through the same pair
everywhere. `Scroll.Rows` is gone: a scroll offset is committed state and the row loop
belonged to whoever draws. There are no compatibility aliases.

### Changed

- **Measurement is held to the same purity as drawing.** Section 4.2 has always said
  "measurement and drawing are observationally pure", and only drawing had a guard.
  The classifier now walks `Measure` receivers as well, and every stateful case
  measures twice: the same answer, and no change to its semantic projection. A
  container asks every child to measure on every frame, so this is the hotter of the
  two paths and was the unguarded one.

- **A drawn field no longer chooses on the application's behalf.** The new guard found
  it. Measuring or drawing a controlled select used to seed the bound value the caller
  had not set yet, which is a rendering pass making a product decision. Initial
  presentation may read a bound value to place a cursor; only an input action or
  semantic validation writes one back.

  The instrument is what makes that observable: a counting accessor reports reads,
  writes and whether it was seeded, so a write that stores the value it already held
  is caught too. Comparing projections before and after could only ever see a value
  that ended up different.

- **An editor is drawn through a look without adopting it.** `Editor.DrawWith` lets an
  appearance component paint one projection in its own theme while the editor stays
  the single owner of its text, cursor and input configuration.

### Fixed

- A non-comparable value used as a child keeps its gesture from press to release. The
  conservative identity comparison existed and was documented; it had no test built on
  a type that would panic under interface equality, and now it does.

## [0.5.0] — 2026-08-10

Three capabilities the terminal-UI audit named, and one limitation this repository had
written into its own documentation and can now delete.

Breaking. `keymap.Pending` and `Map.Lookup` are replaced by `Map.Resolve` and
`Matcher`; `grid.View.PlaceCursor` takes the style the cursor should have. There are no
compatibility aliases.

### Added

- **A binding that is also a prefix is reachable.** `core/keymap` used to say so in its
  own documentation: without a clock, `g` could never run while `gg` might still
  arrive, so the longer sequence always won. The clock was the problem, and no layer
  inside the library is allowed to own one — the dependency graph keeps `runtime` away
  from `interaction` and `keymap` away from `program`. So the decision is a value the
  caller supplies. `Map.Resolve` is a `Resolver`: it schedules a callback and returns a
  cancel, `Runtime.After` has exactly that shape, and `keymap` still imports no clock.

  Nil resolves with the next key instead of a timer, which needs no clock at all: a
  key that can continue the sequence takes the longer binding, and a key that cannot
  settles the exact one first. A field that loses focus cancels whatever it was
  holding, so a late callback cannot move a cursor the user has already left.

  `Matcher` replaces the `Pending` value and the lookup-then-dispatch procedure every
  component had written out separately.

- **Cursor appearance is frame state.** A shape and a blink travel with the frame that
  placed the cursor, are diffed like anything else, and are set through
  `View.PlaceCursor`. `Editor.CursorStyle` is the field an application sets.

- **Native task progress is a session capability.** `term.Progress` and
  `Session.SetProgress` drive the terminal's own progress indicator through
  `ProgressHost`. It is keepalive-aware, restored across a handover, and cleared when
  the session closes, so a program that hands the terminal to an editor and comes back
  does not leave a stale bar behind on a host that shows one.

### Changed

- **The prior-art survey records outcomes, not intentions.** Six of its eight
  candidates are closed, each with the shape it took and what it deliberately did not
  become: the settings component routes actions without introducing a scope system, the
  kill ring stayed private to `Editor` rather than becoming a package, and the
  paste-into-chip recipe lives in `examples/composer` where the threshold and the label
  are the application's.

## [0.4.1] — 2026-08-09

Additive. Nothing exported changed shape, so an application on 0.4.0 compiles
unchanged; what arrived is evidence for claims that were already written down, and one
worked interface that exercises them together.

The performance model had six product questions and mostly micro-benchmarks. It now
has an executable observation per question, and the numbers argue the architecture
rather than the speed: raising the ingress limit from 4 KiB to 64 KiB cuts owner
batches from 257 to 24 at the same throughput, transferring a committed block costs the
same at any session age, resize is linear in retained blocks and not in blocks ever
seen, and a 256-fold larger open markdown tail costs fifteen times more with a constant
allocation count.

### Added

- Six named benchmark families now make the architecture's performance questions
  measurable: unchanged frames, bounded producer bursts, committed transcript age,
  retained resize cost, open-markdown tail size, and unchanged complex frames.
- The examples module now includes a composable prompt and a complete deterministic
  coding-agent mock. Together they demonstrate application-owned paste attachments,
  fuzzy completion, history, bounded streaming output, committed transcript history,
  live plans, explicit tool review, cancellation, and failure settlement without
  introducing product vocabulary into framework packages.

## [0.4.0] — 2026-08-09

This is a repository-wide correctness and ownership pass over the foundations added
in 0.3.0. It removes representational escape hatches, gives repeated low-level
operations one implementation, and turns the failure cases found by the audit into
permanent tests or lint gates.

Breaking. `layout.Sizing` is now opaque and is constructed with `Fixed`, `Part`,
`Flex`, `Measured`, and `AtLeast`; combinations that could not carry one coherent
meaning are no longer representable. `ansi.Params.Private`, `Groups`, `Empty`, and
`Count` are replaced by the immutable `Marker`, `Len`, `At`, and `Group` API, whose
`Parameter` values expose read-only access and iteration. `headless.BlockID` is now an
architecture-independent `uint64` identity. There are no compatibility aliases.

### Added

- **One bounded streaming ANSI scanner.** `ansi.Scanner` is the shared framing state
  machine wherever a terminal byte stream arrives in arbitrary chunks: styled-text
  decoding and the pty screen model. It preserves incomplete UTF-8 and escape syntax
  across chunk boundaries, bounds unfinished sequences, documents borrowed pieces, and
  has chunk-invariance fuzz coverage. The input parser keeps its own framing, because
  it consumes bytes against a reusable buffer and carries drop state that a chunk
  visitor cannot express; what it shares is `ansi.Escape`, `Body` and `Final`, so the
  two cannot disagree about where a sequence ends.
- **Structural duplication is a failing lint rule.** CI now rejects independently
  reintroduced copies of the extent, coordinate, writer, identity and ANSI framing
  primitives this pass centralized. It runs per module, so it catches a second copy
  beside the first and not one in another module; the module boundary is still read
  by people.
- **The prior-art survey has a complete Chinese edition.** The README links the
  English and Chinese architecture, brand, and prior-art documents as parallel entry
  points.

### Changed

- **Layout owns one safe integer algebra.** `Sum`, `Remaining`, `Translate`,
  `Relative`, and `Scale` are the shared operations for non-negative extents, signed
  coordinates, and proportional placement. Layout sizing is a real sum type rather
  than six public fields whose precedence callers had to infer; every component now
  uses the same saturating boundary semantics.
- **Retained values cross explicit ownership cuts.** Styled lines, highlights,
  markdown blocks, diff hunks, key bindings, terminal titles, editor kill entries,
  and text-edit metadata detach from larger caller buffers before they are retained.
  `Decoder.Open` and `headless.Snapshot` now state their borrowed/reference-bearing
  contracts precisely.
- **Long-lived collections have one amortized storage model.** A generic FIFO private
  to `core` backs dispatch, program tests and terminal frames; removal clears
  references and periodically sheds oversized backing arrays. An editor's kills and a
  prompt history keep their own small bounded slices, because `core/internal` is not
  reachable from `components` and a sixteen-entry ring does not earn a boundary
  crossing a module. Text
  decoding, markdown ingress, inline assembly, and markdown rendering grow
  iteratively instead of rebuilding per chunk or recursing with document depth.
- **Stable identities are monotonic and architecture independent.** Transcript
  blocks, modal layers, tree nodes, and editor elements use non-recycled `uint64`
  namespaces and refuse exhaustion before wraparound. Arbitrary interface values are
  compared conservatively, so non-comparable application values cannot panic routing
  or selection.
- **Transport settlement releases the whole boundary.** Completed byte ingresses and
  frame writers release callbacks, dispatchers, payloads, errors, and destination
  writers once no future work can use them. Frame sequence progress is an unbroken
  publication watermark rather than the largest completion observed.
- **Tables, trees, and inline rendering compute once through their owning models.**
  Column sizing has one scan, deep trees and markdown use explicit stacks, and inline
  composition separates measurement, frame assembly, and publication instead of
  mixing phases.

### Fixed

- `io.Writer` implementations and every rendering/protocol output path now preserve
  short-write counts and errors, returning `io.ErrShortWrite` when a writer reports
  incomplete success instead of silently losing output.
- Bracketed paste remains in paste mode after a bounded chunk is emitted and until
  the actual closing sequence arrives; escape-shaped bytes across that boundary stay
  pasted text. Malformed ANSI parameters remain malformed instead of becoming valid
  defaults.
- Fuzzy matching applies Unicode simple folding, diff context arithmetic cannot
  overflow at `math.MaxInt`, and select/multiselect option replacement distinguishes
  choice identity from a possibly non-comparable bound value.
- Unknown graphics protocols and placements are unsupported rather than inheriting a
  known capability. Extreme shimmer periods, zero-span looping timelines, and clocks
  that move backwards retain monotonic, bounded state.
- Grid projection, viewport movement, transcript ranges, editor motion, scrolling,
  pointer capture, and click proximity no longer wrap at integer boundaries. Negative
  table measurements and zero-sized box interiors remain empty instead of becoming
  valid geometry elsewhere.
- Terminal and frame identity sequences cannot reuse a live identifier; ambiguous
  partial writes invalidate presentation state, and a later frame cannot make an
  older unfinished frame appear published.

## [0.3.0] — 2026-08-08

A seventh module, five components that could not be built here before, and a pass over
what a component is allowed to own.

`ssh` is the first host this repository ships that is not the terminal it is running
on, and it is one exported function: an application that has already accepted and
authenticated a session hands it over, and `core` learns nothing about servers. It is
also the first evidence that `program.Host` at three methods is a seam somebody else
can reach.

The rest of the additions are the components the
[prior-art survey](docs/prior-art.md#a-general-behaviour-now-implemented) found
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
