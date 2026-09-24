package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// The fields a form is made of: a line of text, one choice, several, and a yes or no.
// They are four shapes of the same thing — something is asked for, an answer is given,
// and the answer is checked — which is why the part they have in common is a struct
// they embed rather than four copies of the same twenty lines.

// Text is a field holding a line of text.
//
// It is a one-line [Editor] with a label and a check around it, so everything a field
// does — the cursor, selecting, undo, the clipboard, a click landing where the reader
// meant it — is that field's and was not written again.
//
// One of these on its own is a [Form] with one field in it, which is where its look
// comes from and is what an interface that wants a single input asks for.
//
// The zero value is ready. A Text value must not be copied after first use: its editor,
// validation and caller-owned reconciliation are one mutable field.
type Text struct {
	noCopy noCopy

	field
	// Label is what the field is asking for.
	Label string
	// Value is the caller-owned text. A caller change is observed at the next semantic
	// operation and by drawing; that operation writes back the one-line canonical form.
	// Edits write immediately and adopt the value the owner accepts. Nil keeps the text
	// local.
	Value Accessor[string]
	// Check says what is wrong with what has been entered, or nil. It is asked when the
	// keyboard leaves the field and when the form is submitted.
	Check func(s string) error
	// Placeholder is shown while the field is empty.
	Placeholder string
	// Keys say which keystrokes edit. Nil reads through [DefaultEditorKeys].
	Keys *keymap.Map
	// Clipboard supplies copy, cut and paste services. Nil disables those actions.
	Clipboard Clipboard
	// Gutter optionally draws beside the single text row.
	Gutter RowGutter
	// CursorStyle chooses the focused terminal cursor's shape and blink.
	CursorStyle grid.CursorStyle

	editor Editor
	seeded bool
	// settling says an edit of this field's own is with its owner, waiting to be
	// accepted, rejected or normalized.
	//
	// For the length of that call the editor holds what was typed and the owner holds
	// what it has decided, so the two differ by construction — and an owner that
	// normalizes from inside its own Set, then synchronizes this field, would find
	// that difference and read it as somebody else having replaced the value. What
	// followed was not a normalized edit but an adoption: the cursor at the end of the
	// line, the history gone, and the keystroke counted twice.
	settling bool
}

// Text reads the accepted one-line value without synchronizing or writing its owner.
func (t *Text) Text() string {
	if t.Value != nil {
		return oneLineText(t.Value.Value())
	}
	return t.editor.Text()
}

// SetText replaces the answer through the same acceptance and history boundary as input.
func (t *Text) SetText(value string) {
	t.Sync()
	edit := t.beginEdit()
	t.editor.SetText(value)
	t.storeSince(edit)
}

// Cursor returns the byte column in the current one-line projection.
func (t *Text) Cursor() int { return t.editor.lineView(t.Text()).cursor }

// SetCursor synchronizes the answer, then moves to a grapheme boundary.
func (t *Text) SetCursor(column int) { t.Sync(); t.editor.SetCursor(0, column) }

// Revision reports the accepted editor content generation, after the last Sync or edit.
func (t *Text) Revision() uint64 { return t.editor.Revision() }

// SetMask chooses what is drawn in place of each input grapheme; empty removes
// masking.
// Invalid masks panic on the same terms as [Editor.SetMask].
func (t *Text) SetMask(mask string) { t.editor.SetMask(mask) }

// Mask returns the configured mask.
func (t *Text) Mask() string { return t.editor.Mask() }

// Prompt is what the field is asking for.
func (t *Text) Prompt() string { return t.Label }

// HeightForWidth is the label, a row of text, and the problem with it if there is one.
func (t *Text) HeightForWidth(int) int { return layout.Sum(1, t.rows(t.Label)) }

// Draw paints the label, the field and whatever was wrong with the answer.
func (t *Text) Draw(v Frame) {
	t.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (t *Text) DrawWith(v Frame, look Look) {
	view := t.editor.lineView(t.Text())
	view.placeholder, view.gutter, view.cursorStyle = t.Placeholder, t.Gutter, t.CursorStyle
	view.draw(t.frame(v, t.Label, look), look, &t.editor.presentation)
}

// Handle passes input to the field and keeps the value in step with it.
func (t *Text) Handle(ev input.Event) bool {
	t.Sync()
	if key, ok := ev.(input.Key); ok {
		return t.editor.handleKey(key, t.Do, func(key input.Key) bool {
			edit := t.beginEdit()
			handled := t.editor.typed(key)
			t.storeSince(edit)
			return handled
		})
	}
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := t.within(mouse)
		// A release is a lifetime transition and not a hit test. Once this field took
		// the press, the release that ends it has to arrive however far the pointer
		// travelled in between — a selection dragged out of the field and let go is
		// an ordinary thing to do. Dropping it leaves the editor believing a selection
		// is still being made, and the next drag that crosses the field continues one
		// nobody started here.
		ending := mouse.Action == input.MouseUp && t.editor.dragging
		if !in && !ending {
			return false
		}
		ev = local
	}
	edit := t.beginEdit()
	handled := t.editor.Handle(ev)
	t.storeSince(edit)
	return handled
}

// Do runs one of the field's actions by name. See [Doer].
func (t *Text) Do(action keymap.Action) bool {
	t.Sync()
	edit := t.beginEdit()
	if !t.editor.Do(action) {
		return false
	}
	t.storeSince(edit)
	return true
}

// Validate checks what has been entered.
func (t *Text) Validate() error {
	t.Sync()
	if t.Check == nil {
		return t.check(nil)
	}
	return t.check(t.Check(t.editor.Text()))
}

// Focus takes the keyboard or gives it up, and checks the answer on the way out.
//
// On the way out and not on the way in: a form that greeted somebody with a column of
// complaints about answers they have not given yet would be a form nobody finishes. A
// field that has never had the keyboard has nothing to check.
func (t *Text) Focus(has bool) {
	t.Sync()
	t.editor.Focus(has)
	if t.leaving(has) {
		_ = t.Validate()
	}
}

// Sync makes the caller-owned value authoritative at semantic boundaries. Drawing
// projects that value without mutating the editor; input first reconciles the editor
// so cursor, history and the subsequent write all describe the same text.
func (t *Text) Sync() {
	t.editor.SetSingleLine(true)
	t.editor.Placeholder = t.Placeholder
	t.editor.Keys = t.Keys
	t.editor.Clipboard, t.editor.Gutter, t.editor.CursorStyle = t.Clipboard, t.Gutter, t.CursorStyle
	if t.Value == nil {
		t.seeded = true
		return
	}
	// An edit already with its owner has an owner: the operation that offered it,
	// which is the only one that knows what the answer coming back is an answer to.
	if t.settling {
		return
	}
	owned := t.Value.Value()
	value := oneLineText(owned)
	if value != owned {
		value = oneLineText(t.offer(value))
	}
	if !t.seeded || value != t.editor.Text() {
		t.adopt(value)
	}
	t.seeded = true
}

// adopt replaces the editor with an owner-written or owner-accepted value.
// Reconciliation is a new owner state, not a user edit inside the old one. Keeping
// previous snapshots would let Undo overwrite an accessor with a value that stopped
// being current before this operation began.
func (t *Text) adopt(value string) {
	t.editor.SetText(value)
	t.editor.history.clear()
}

// store offers what has been typed to its owner and settles the same edit onto the
// value that owner accepted. A validator or normalizer must not leave the editor as a
// private shadow of the request it declined or changed, but its answer is not a new
// external state transition and therefore must not clear this edit's history.
func (t *Text) store(edit textEdit) {
	if t.Value != nil {
		requested := t.editor.Text()
		accepted := oneLineText(t.offer(requested))
		switch {
		case accepted == requested:
		case edit.checkpointed && accepted == edit.checkpoint.text:
			t.editor.rejectEdit(edit.checkpoint)
		default:
			t.editor.reconcileEdit(accepted)
		}
	}
}

// offer hands a value to the owner and returns what the owner has afterwards. The
// exchange is this field's for the length of the call, whatever the owner does
// inside it.
func (t *Text) offer(value string) string {
	t.settling = true
	defer func() { t.settling = false }()
	return setAccessor(t.Value, value)
}

type textEdit struct {
	revision     uint64
	checkpoint   editorCheckpoint
	checkpointed bool
}

func (t *Text) beginEdit() textEdit {
	edit := textEdit{revision: t.editor.Revision()}
	// Bind is an exact assignment and therefore cannot reject or normalize. Every
	// open Accessor implementation gets a rollback checkpoint because its Set
	// postcondition, not its requested argument, decides whether an edit existed.
	if _, exact := t.Value.(bound[string]); t.Value != nil && !exact {
		edit.checkpoint, edit.checkpointed = t.editor.checkpointEdit(), true
	}
	return edit
}

// storeSince writes through a controlled field only when its semantic answer changed.
// Handling a cursor key or an impossible deletion is not an assignment merely because
// the editor consumed the action.
func (t *Text) storeSince(edit textEdit) {
	if t.editor.Revision() != edit.revision {
		t.store(edit)
	}
}

// Option is one thing a choice offers.
type Option[T any] struct {
	// Label is the row as shown.
	Label string
	// Value is what choosing it means.
	Value T
}

// Equal compares values by Go equality. Use Equal[T] as Same for comparable choices.
func Equal[T comparable](a, b T) bool { return a == b }

func (o Option[T]) holds(want T, same func(a, b T) bool) bool {
	return same(o.Value, want)
}

// Options is the usual case, where what is shown is what it means.
func Options[T ~string](values ...T) []Option[T] {
	out := make([]Option[T], len(values))
	for i, v := range values {
		out[i] = Option[T]{Label: string(v), Value: v}
	}
	return out
}

// Select is a field holding one choice out of several.
//
// The choice follows the cursor: what is under it is what is chosen, and there is
// nothing to press to confirm. A list that made somebody move to a row and then take it
// is a list that can be left on a row nobody took.
//
// The zero value is empty and ready. A Select must not be copied after first use: its
// options, cursor, scroll and controlled-state settlement are one mutable field.
type Select[T any] struct {
	noCopy noCopy

	field
	// Label is what the field is asking for.
	Label string
	// Value is the caller-owned choice. A caller change moves the cursor at the next
	// semantic operation and is projected by drawing; cursor movement writes it
	// immediately and adopts the value the owner accepts. A value that names no option
	// falls back to the current choice and is written when the field is validated. Nil
	// keeps the choice local.
	Value Accessor[T]
	// Same defines value identity, independently of Label. It is required before
	// installing non-empty options. Use Equal[T] for comparable values, or supply
	// domain equality for arbitrary T. It must be pure and remain stable while options
	// are installed; changing the identity rule requires constructing a new field.
	Same func(a, b T) bool
	// Check says what is wrong with the choice, or nil.
	Check func(v T) error
	// Rows caps how many options are shown at once. Zero shows them all.
	Rows int
	// Keys say which keystrokes move the cursor. Nil reads through [DefaultListKeys].
	Keys *keymap.Map

	// Row optionally draws a one-row choice with its cursor and taken states.
	// Nil uses Look's default choice row. Like Draw, the callback must be pure.
	Row func(grid.View, Option[T], bool, bool, Look)

	list    List[Option[T]]
	matcher keymap.Matcher
	// stored is the selected index last written to Value. synced distinguishes that
	// state from an unmatched initial value that happened to fall back to the same
	// index. Validation settles the latter once; it never uses an accessor write as
	// an acceptance event.
	stored int
	synced bool
}

// Prompt is what the field is asking for.
func (s *Select[T]) Prompt() string { return s.Label }

// SetOptions replaces what is on offer. Select owns the slice. If the selected
// choice still exists under Same, the cursor
// follows it to its new position; otherwise the cursor is clamped and the bound
// value follows the resulting choice. Non-empty options require Same; omitting it
// is a programmer error and panics. The initial unmatched value is settled by Validate.
func (s *Select[T]) SetOptions(options []Option[T]) {
	s.matcher.Clear()
	previous, hadPrevious := s.list.Current()
	if hadPrevious && s.Value != nil {
		previous, hadPrevious = s.Chosen()
	}
	if len(options) > 0 && s.Same == nil {
		panic("headless: Select requires Same before SetOptions")
	}
	s.list.SetItems(options)
	for i := range s.list.items {
		s.list.items[i].Label = strings.Clone(s.list.items[i].Label)
	}
	if s.Value != nil {
		if hadPrevious {
			s.sync()
			s.store()
		}
		return
	}
	s.synced = false
	if hadPrevious {
		for i, option := range s.list.items {
			if option.holds(previous.Value, s.Same) {
				s.list.Select(i)
				s.store()
				return
			}
		}
	}
	s.store()
}

// Options returns a copy of what is on offer.
func (s *Select[T]) Options() []Option[T] { return slices.Clone(s.list.items) }

// Chosen projects the current choice without changing the cursor or writing Value.
func (s *Select[T]) Chosen() (Option[T], bool) {
	at := s.list.Selected()
	if s.Value != nil {
		at, _ = s.indexOf(s.Value.Value())
	}
	return s.list.At(at)
}

// HeightForWidth is the label, the options within their cap, and the problem if there is one.
func (s *Select[T]) HeightForWidth(int) int {
	rows := len(s.list.items)
	if s.Rows > 0 {
		rows = min(rows, s.Rows)
	}
	return layout.Sum(max(rows, 1), s.rows(s.Label))
}

// Draw paints the label, the options and whatever was wrong with the choice.
func (s *Select[T]) Draw(v Frame) {
	s.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (s *Select[T]) DrawWith(v Frame, look Look) {
	selected := s.list.Selected()
	if s.Value != nil {
		selected, _ = s.indexOf(s.Value.Value())
	}
	s.list.drawRows(s.frame(v, s.Label, look), selected, func(v grid.View, _ int, option Option[T], under bool) {
		if s.Row != nil {
			s.Row(v, option, under, under, look)
		} else {
			look.choice(v, option.Label, under, under)
		}
	})
}

func (s *Select[T]) indexOf(want T) (int, bool) {
	for i, option := range s.list.items {
		if option.holds(want, s.Same) {
			return i, true
		}
	}
	return s.list.Selected(), false
}

// Handle moves the cursor, and takes the choice with it.
func (s *Select[T]) Handle(ev input.Event) bool {
	s.Sync()
	if key, ok := ev.(input.Key); ok {
		_, handled := s.matcher.Handle(s.list.keys(), key, s.Do)
		return handled
	}
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := s.within(mouse)
		if !in {
			return false
		}
		ev = local
	}
	before := s.list.Selected()
	handled := s.list.Handle(ev)
	if s.list.Selected() != before {
		s.store()
	}
	return handled
}

// Do runs one of the field's actions by name. See [Doer].
func (s *Select[T]) Do(action keymap.Action) bool {
	s.Sync()
	before := s.list.Selected()
	if !s.list.Do(action) {
		return false
	}
	if s.list.Selected() != before {
		s.store()
	}
	return true
}

// Validate checks the choice.
func (s *Select[T]) Validate() error {
	s.Sync()
	// An initial value that named no option is settled here, at the semantic validation
	// boundary rather than during presentation. Once settled, checking it again is not
	// another assignment.
	s.store()
	if s.Check == nil {
		return s.check(nil)
	}
	chosen, ok := s.list.Current()
	if !ok {
		var zero T
		return s.check(s.Check(zero))
	}
	return s.check(s.Check(chosen.Value))
}

// Focus takes the keyboard or gives it up, and checks the choice on the way out.
func (s *Select[T]) Focus(has bool) {
	if !has {
		s.matcher.Clear()
	}
	s.Sync()
	s.list.Focus(has)
	if s.leaving(has) {
		_ = s.Validate()
	}
}

// Sync reconciles the cursor with Value without accepting an unavailable value.
func (s *Select[T]) Sync() {
	s.list.Keys = s.Keys
	s.sync()
}

// sync reconciles the cursor with the caller-owned value without accepting an
// unavailable value. Validation or an actual selection transition performs that
// settlement through store; a read alone is not an assignment.
func (s *Select[T]) sync() {
	if s.Value == nil {
		return
	}
	s.syncValue(s.Value.Value())
}

func (s *Select[T]) syncValue(value T) {
	// The cursor starts on the choice already made, which is what makes a form somebody
	// is coming back to show what they said last time.
	at, matched := s.indexOf(value)
	s.list.Select(at)
	s.stored, s.synced = s.list.Selected(), matched
}

func (s *Select[T]) store() {
	if s.Value == nil {
		return
	}
	at := s.list.Selected()
	if s.synced && s.stored == at {
		return
	}
	if chosen, ok := s.list.Current(); ok {
		s.syncValue(setAccessor(s.Value, chosen.Value))
	}
}

// MultiSelect is a field holding any number of choices out of several.
//
// The cursor and the choice are two things here, unlike in a [Select]: moving is not
// choosing, and something has to be pressed. That is the whole difference between
// picking one and picking some.
//
// The zero value is empty and ready. A MultiSelect must not be copied after first use:
// its options, chosen set, cursor and matcher are one mutable field.
type MultiSelect[T any] struct {
	noCopy noCopy

	field
	// Label is what the field is asking for.
	Label string
	// Value is the caller-owned set. Reads and drawing project it in option order.
	// Sync and semantic operations discard unavailable choices;
	// duplicates are folded and option order is restored through one write; the field
	// then adopts the set the owner accepts. A value already in canonical form is not
	// rewritten. Nil keeps the set local.
	Value Accessor[[]T]
	// Same says whether two values are the same one — see [Select.Same]. It is a pure
	// projection callback and may run during drawing.
	Same func(a, b T) bool
	// Check says what is wrong with the choices, or nil.
	Check func(v []T) error
	// Rows caps how many options are shown at once. Zero shows them all.
	Rows int
	// Keys say which keystrokes move and take. Nil reads through
	// [DefaultMultiSelectKeys].
	Keys *keymap.Map

	// Row optionally draws a one-row choice with its cursor and taken states.
	// Nil uses Look's default choice row. Like Draw, the callback must be pure.
	Row func(grid.View, Option[T], bool, bool, Look)

	list    List[Option[T]]
	taken   []bool
	limit   int
	matcher keymap.Matcher
}

// Prompt is what the field is asking for.
func (m *MultiSelect[T]) Prompt() string { return m.Label }

// SetOptions replaces what is on offer. MultiSelect owns the slice and preserves
// each taken choice that remains available under Same, wherever it moved.
// Non-empty options require Same; omitting it is a programmer error and panics.
func (m *MultiSelect[T]) SetOptions(options []Option[T]) {
	m.matcher.Clear()
	hadOptions := m.list.Len() > 0
	var previous []Option[T]
	if m.Value == nil {
		previous = m.takenOptions()
	}
	if len(options) > 0 && m.Same == nil {
		panic("headless: MultiSelect requires Same before SetOptions")
	}
	m.list.SetItems(options)
	for i := range m.list.items {
		m.list.items[i].Label = strings.Clone(m.list.items[i].Label)
	}
	m.taken = make([]bool, len(m.list.items))
	if m.Value != nil {
		if hadOptions {
			m.Sync()
		}
		return
	}
	for _, want := range previous {
		for i, option := range m.list.items {
			if !m.taken[i] && option.holds(want.Value, m.Same) {
				m.taken[i] = true
				break
			}
		}
	}
	clampTaken(m.taken, m.limit)
	m.store()
}

// Options returns a copy of what is on offer.
func (m *MultiSelect[T]) Options() []Option[T] { return slices.Clone(m.list.items) }

// SetLimit changes how many choices may be taken at once. Zero allows every option;
// a negative limit is a programmer error and panics, because zero already means "no
// limit" and there is no smaller quantity of choices for a negative one to name.
// Lowering the limit keeps the earliest choices in option order and writes the settled
// set back to a bound value.
func (m *MultiSelect[T]) SetLimit(limit int) {
	if limit < 0 {
		panic("headless: multi-select limit cannot be negative")
	}
	m.limit = limit
	// Settle against the new limit directly, without publishing an intermediate set.
	m.Sync()
}

// Limit reports how many choices may be taken at once. Zero allows every option.
func (m *MultiSelect[T]) Limit() int {
	return m.limit
}

// Taken projects the available choices in option order without writing Value.
// Use Sync to explicitly settle duplicates, unavailable choices and the limit.
func (m *MultiSelect[T]) Taken() []T {
	taken, _ := m.selection()
	return m.valuesOf(taken)
}

func (m *MultiSelect[T]) takenValues() []T { return m.valuesOf(m.taken) }

func (m *MultiSelect[T]) valuesOf(taken []bool) []T {
	var out []T
	for i, option := range m.list.items {
		if i < len(taken) && taken[i] {
			out = append(out, option.Value)
		}
	}
	return out
}

func (m *MultiSelect[T]) takenOptions() []Option[T] {
	var out []Option[T]
	for i, option := range m.list.items {
		if i < len(m.taken) && m.taken[i] {
			out = append(out, option)
		}
	}
	return out
}

func (m *MultiSelect[T]) takenCount() int {
	count := 0
	for _, taken := range m.taken {
		if taken {
			count++
		}
	}
	return count
}

// clampTaken is the one enforcement point for a choice limit. It preserves option
// order, which is the order MultiSelect reports and the only stable priority
// available.
func clampTaken(taken []bool, limit int) bool {
	if limit <= 0 {
		return false
	}
	kept := 0
	changed := false
	for i, selected := range taken {
		if !selected {
			continue
		}
		if kept < limit {
			kept++
			continue
		}
		taken[i] = false
		changed = true
	}
	return changed
}

// Toggle takes the option under the cursor, or gives it back, and reports whether
// anything changed. Nothing changes when the limit is reached.
func (m *MultiSelect[T]) Toggle() bool {
	m.Sync()
	at := m.list.Selected()
	if at < 0 || at >= len(m.taken) {
		return false
	}
	if !m.taken[at] && m.limit > 0 && m.takenCount() >= m.limit {
		return false
	}
	before := slices.Clone(m.taken)
	m.taken[at] = !m.taken[at]
	m.store()
	return !slices.Equal(before, m.taken)
}

// HeightForWidth is the label, the options within their cap, and the problem if there is one.
func (m *MultiSelect[T]) HeightForWidth(int) int {
	rows := len(m.list.items)
	if m.Rows > 0 {
		rows = min(rows, m.Rows)
	}
	return layout.Sum(max(rows, 1), m.rows(m.Label))
}

// Draw paints the label, the options and whatever was wrong with the choices.
func (m *MultiSelect[T]) Draw(v Frame) {
	m.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (m *MultiSelect[T]) DrawWith(v Frame, look Look) {
	taken := m.taken
	if m.Value != nil {
		taken, _ = m.selection()
	}
	m.list.DrawRows(m.frame(v, m.Label, look), func(v grid.View, at int, option Option[T], under bool) {
		chosen := at < len(taken) && taken[at]
		if m.Row != nil {
			m.Row(v, option, under, chosen, look)
		} else {
			look.choice(v, option.Label, under, chosen)
		}
	})
}

func (m *MultiSelect[T]) selection() ([]bool, bool) {
	if m.Value == nil {
		taken := make([]bool, len(m.list.items))
		copy(taken, m.taken)
		return taken, !clampTaken(taken, m.limit)
	}
	return m.selectionOf(m.Value.Value())
}

func (m *MultiSelect[T]) selectionOf(bound []T) ([]bool, bool) {
	taken := make([]bool, len(m.list.items))
	for _, want := range bound {
		for i, option := range m.list.items {
			if option.holds(want, m.Same) {
				taken[i] = true
				break
			}
		}
	}
	clamped := clampTaken(taken, m.limit)
	count := 0
	for _, selected := range taken {
		if selected {
			count++
		}
	}
	if clamped || len(bound) != count {
		return taken, false
	}
	at := 0
	for i, option := range m.list.items {
		if !taken[i] {
			continue
		}
		if !option.holds(bound[at], m.Same) {
			return taken, false
		}
		at++
	}
	return taken, true
}

// Handle moves the cursor and takes choices.
func (m *MultiSelect[T]) Handle(ev input.Event) bool {
	m.Sync()
	if key, ok := ev.(input.Key); ok {
		_, handled := m.matcher.Handle(m.keys(), key, m.Do)
		return handled
	}
	mouse, ok := ev.(input.Mouse)
	if !ok {
		return false
	}
	local, in := m.within(mouse)
	if !in {
		return false
	}
	return m.list.Handle(local)
}

// Do runs one of the field's actions by name. See [Doer].
func (m *MultiSelect[T]) Do(action keymap.Action) bool {
	m.Sync()
	if action == Toggle {
		m.Toggle()
		return true
	}
	return m.list.Do(action)
}

// Validate checks the choices.
func (m *MultiSelect[T]) Validate() error {
	m.Sync()
	if m.Check == nil {
		return m.check(nil)
	}
	return m.check(m.Check(m.Taken()))
}

// Focus takes the keyboard or gives it up, and checks the choices on the way out.
func (m *MultiSelect[T]) Focus(has bool) {
	m.Sync()
	if !has {
		m.matcher.Clear()
	}
	if m.leaving(has) {
		_ = m.Validate()
	}
}

// Sync makes the caller-owned set the selection model's source. Unlike Draw, this is
// a semantic boundary, so it also settles a non-canonical value exactly once.
//
// Unlike [Select.Sync] it hands the list no key map. The list inside has none of its
// own: this field resolves every keystroke against one that has the movement and the
// key that takes a choice in it, and drives the list by name. Offering the event to
// both would resolve it twice.
func (m *MultiSelect[T]) Sync() {
	var canonical bool
	m.taken, canonical = m.selection()
	if !canonical {
		m.store()
	}
}

func (m *MultiSelect[T]) store() {
	if m.Value != nil {
		accepted := setAccessor(m.Value, m.takenValues())
		m.taken, _ = m.selectionOf(accepted)
	}
}

func (m *MultiSelect[T]) keys() *keymap.Map {
	if m.Keys != nil {
		return m.Keys
	}
	return multiSelectKeys()
}

// Confirm is a field holding a yes or a no.
//
// The zero value answers no and is ready. A Confirm must not be copied after first
// use: its answer, matcher and committed pointer split are one mutable field.
type Confirm struct {
	noCopy noCopy

	field
	// Label is what the field is asking.
	Label string
	// Value is the caller-owned answer. Reads observe it directly and answers write it
	// immediately. Nil keeps the answer local.
	Value Accessor[bool]
	// Yes and No are the two answers as they are shown. Empty uses "yes" and "no".
	Yes, No string
	// Check says what is wrong with the answer, or nil.
	Check func(v bool) error
	// Keys say which keystrokes answer. Nil reads through [DefaultConfirmKeys].
	Keys *keymap.Map

	answer  valueState[bool]
	matcher keymap.Matcher
	// split is the committed column where the second answer begins.
	split Snapshot[int]
}

// Prompt is what the field is asking.
func (c *Confirm) Prompt() string { return c.Label }

// Answer is what has been answered.
func (c *Confirm) Answer() bool {
	return c.answer.get(c.Value)
}

// Say answers the field.
func (c *Confirm) Say(yes bool) {
	c.answer.set(c.Value, yes)
}

// HeightForWidth is the label, the two answers on one row, and the problem if there is one.
func (c *Confirm) HeightForWidth(int) int { return layout.Sum(1, c.rows(c.Label)) }

// Draw paints the label and the two answers.
func (c *Confirm) Draw(v Frame) {
	c.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (c *Confirm) DrawWith(v Frame, look Look) {
	answer := c.Answer()
	row := c.frame(v, c.Label, look)
	w, h := row.Size()
	if w <= 0 || h <= 0 {
		c.split.Stage(v, 0)
		return
	}
	x, split := 0, 0
	for _, yes := range []bool{true, false} {
		if !yes {
			// Where one answer ends and the other begins, which is the whole of what a
			// press needs to know.
			split = x
		}
		style := look.Text
		if yes == answer {
			style = look.Selection.Merge(look.Accent)
		}
		if mark, width := look.mark(yes == answer); width > 0 {
			x = layout.Sum(x, row.Text(x, 0, mark, style), 1)
		}
		x = layout.Sum(x, row.Text(x, 0, c.word(yes), style))
		x = layout.Sum(x, row.Text(x, 0, "  ", look.Text))
	}
	c.split.Stage(v, split)
}

// Handle answers the field, by key or by pressing one of the two answers.
func (c *Confirm) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := c.within(mouse)
		if !in || local.Action != input.MouseDown || local.Button != input.ButtonLeft {
			return false
		}
		c.Say(local.Pos.X < c.split.Value())
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := c.matcher.Handle(c.keys(), key, c.Do)
	return handled
}

// Do runs one of the field's actions by name. See [Doer].
func (c *Confirm) Do(action keymap.Action) bool {
	switch action {
	case SelectPrev:
		c.Say(true)
	case SelectNext:
		c.Say(false)
	case Toggle:
		c.Say(!c.Answer())
	default:
		return false
	}
	return true
}

// Validate checks the answer.
func (c *Confirm) Validate() error {
	if c.Check == nil {
		return c.check(nil)
	}
	return c.check(c.Check(c.Answer()))
}

// Focus takes the keyboard or gives it up, and checks the answer on the way out.
func (c *Confirm) Focus(has bool) {
	if !has {
		c.matcher.Clear()
	}
	if c.leaving(has) {
		_ = c.Validate()
	}
}

func (c *Confirm) word(yes bool) string {
	switch {
	case yes && c.Yes != "":
		return c.Yes
	case yes:
		return "yes"
	case c.No != "":
		return c.No
	default:
		return "no"
	}
}

func (c *Confirm) keys() *keymap.Map {
	if c.Keys != nil {
		return c.Keys
	}
	return confirmKeys()
}
