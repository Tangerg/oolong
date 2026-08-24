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

	editor Editor
	seeded bool
}

// Editor is the field itself, for a caller that needs the cursor, clipboard, or
// one-line appearance such as [Editor.SetMask].
func (t *Text) Editor() *Editor { return &t.editor }

// Prompt is what the field is asking for.
func (t *Text) Prompt() string { return t.Label }

// Measure is the label, a row of text, and the problem with it if there is one.
func (t *Text) Measure(int) int { return layout.Sum(1, t.rows(t.Label)) }

// Draw paints the label, the field and whatever was wrong with the answer.
func (t *Text) Draw(v Frame) {
	t.drawField(v, Look{})
}

func (t *Text) drawField(v Frame, look Look) {
	editor := t.projection()
	editor.drawWith(t.frame(v, t.Label, look), look, &t.editor.presentation)
}

// projection is the field as it should appear without making presentation the event
// that initializes its semantic editor. A form normally focuses every field before
// its first frame, but a lone field must still show a caller-owned initial value and
// Draw must remain a pure read of that value.
func (t *Text) projection() *Editor {
	// drawWith receives its look explicitly; keys and the clipboard are input
	// services; MaxRows and scrolling are multi-line concerns; element identity has
	// no distinct appearance. This is the complete state a one-line draw reads.
	projected := &Editor{
		Placeholder: t.Placeholder,
		Gutter:      t.editor.Gutter,
		CursorStyle: t.editor.CursorStyle,
		blurred:     t.editor.blurred,
		singleLine:  true,
		mask:        t.editor.mask,
		presentation: Snapshot[editorPresentation]{
			current: t.editor.presentation.Value(),
		},
	}
	value := ""
	if t.Value != nil {
		value = oneLineText(t.Value.Value())
	}
	if t.Value == nil || t.seeded && value == t.editor.Text() {
		// A render projection owns its storage and directly expresses Text's one-line
		// view. Editor is intentionally a mutable owner, not a value safe to copy and
		// then mutate: its history, marks and kill ring all contain slice headers.
		lines := t.editor.lines
		if len(lines) == 0 {
			lines = []string{""}
		}
		projected.lines = []string{strings.Join(lines, " ")}
		projected.col = offsetInLines(lines, Caret{Line: t.editor.line, Col: t.editor.col})
		projected.anchor.Col = offsetInLines(lines, t.editor.anchor)
		projected.selecting = t.editor.selecting
		return projected
	}
	projected.lines = []string{value}
	projected.col = len(value)
	return projected
}

// Handle passes input to the field and keeps the value in step with it.
func (t *Text) Handle(ev input.Event) bool {
	t.ensure()
	before := t.editor.Revision()
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := t.within(mouse)
		if !in {
			return false
		}
		handled := t.editor.Handle(local)
		t.storeSince(before)
		return handled
	}
	if !t.editor.Handle(ev) {
		return false
	}
	t.storeSince(before)
	return true
}

// Do runs one of the field's actions by name. See [Doer].
func (t *Text) Do(action keymap.Action) bool {
	t.ensure()
	before := t.editor.Revision()
	if !t.editor.Do(action) {
		return false
	}
	t.storeSince(before)
	return true
}

// Validate checks what has been entered.
func (t *Text) Validate() error {
	t.ensure()
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
	t.ensure()
	t.editor.Focus(has)
	if t.leaving(has) {
		_ = t.Validate()
	}
}

// ensure makes the caller-owned value authoritative at semantic boundaries. Drawing
// projects that value without mutating the editor; input first reconciles the editor
// so cursor, history and the subsequent write all describe the same text.
func (t *Text) ensure() {
	t.editor.SetSingleLine(true)
	t.editor.Placeholder = t.Placeholder
	t.editor.Keys = t.Keys
	if t.Value == nil {
		t.seeded = true
		return
	}
	owned := t.Value.Value()
	value := oneLineText(owned)
	if value != owned {
		value = oneLineText(setAccessor(t.Value, value))
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

// store offers what has been typed to its owner and immediately adopts the value the
// owner accepted. A validator or normalizer must not leave the editor as a private
// shadow of the request it declined or changed.
func (t *Text) store() {
	if t.Value != nil {
		requested := t.editor.Text()
		accepted := oneLineText(setAccessor(t.Value, requested))
		if accepted != requested {
			t.adopt(accepted)
		}
	}
}

// storeSince writes through a controlled field only when its semantic answer changed.
// Handling a cursor key or an impossible deletion is not an assignment merely because
// the editor consumed the action.
func (t *Text) storeSince(before uint64) {
	if t.editor.Revision() != before {
		t.store()
	}
}

// Option is one thing a choice offers.
type Option[T any] struct {
	// Label is the row as shown.
	Label string
	// Value is what choosing it means.
	Value T
}

// holds reports whether the option is the one standing for a value, by the caller's
// rule or, with none, by what the option is shown as.
//
// Comparing what is shown is the only thing possible without a rule: Go will not
// compare two values of a type parameter, and a type it cannot compare at all — a
// slice, a map — would panic on the attempt.
func (o Option[T]) holds(want T, same func(a, b T) bool) bool {
	if same != nil {
		return same(o.Value, want)
	}
	switch shown := any(want).(type) {
	case string:
		return o.Label == shown
	case interface{ String() string }:
		return o.Label == shown.String()
	}
	return false
}

// sameOption reports whether two complete options are the same choice. Unlike
// holds, this comparison has both labels available, so arbitrary values do not need
// to become comparable merely to preserve a choice while options are reordered.
func (o Option[T]) sameOption(other Option[T], same func(a, b T) bool) bool {
	if same != nil {
		return same(o.Value, other.Value)
	}
	return o.Label == other.Label
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
	// options are private because replacing them has to reconcile the selected value
	// with the list cursor and its bound accessor.
	options []Option[T]
	// Value is the caller-owned choice. A caller change moves the cursor at the next
	// semantic operation and is projected by drawing; cursor movement writes it
	// immediately and adopts the value the owner accepts. A value that names no option
	// falls back to the current choice and is written when the field is validated. Nil
	// keeps the choice local.
	Value Accessor[T]
	// Same says whether two values are the same one, which is what puts the cursor on
	// the choice already made. Nil matches a bound string or Stringer value against
	// the label, and matches old and new options by label when they are replaced. A
	// different value type whose initial bound value must identify an option supplies
	// Same; Go cannot safely compare an arbitrary T on the field's behalf. Same may run
	// during drawing and must not mutate either value or unrelated state.
	Same func(a, b T) bool
	// Check says what is wrong with the choice, or nil.
	Check func(v T) error
	// Rows caps how many options are shown at once. Zero shows them all.
	Rows int
	// Keys say which keystrokes move the cursor. Nil reads through [DefaultListKeys].
	Keys *keymap.Map

	list   List[Option[T]]
	seeded bool
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
// choice still exists under Same, or has the same label when Same is nil, the cursor
// follows it to its new position; otherwise the cursor is clamped and the bound
// value follows the resulting choice.
func (s *Select[T]) SetOptions(options []Option[T]) {
	previous, hadPrevious := s.list.Current()
	s.options = own(s.options, options)
	for i := range s.options {
		s.options[i].Label = strings.Clone(s.options[i].Label)
	}
	s.list.SetItems(s.options)
	if !s.seeded {
		return
	}
	if s.Value != nil {
		s.sync()
		s.store()
		return
	}
	s.synced = false
	if hadPrevious {
		for i, option := range s.options {
			if option.sameOption(previous, s.Same) {
				s.list.Select(i)
				s.store()
				return
			}
		}
	}
	s.store()
}

// Options returns a copy of what is on offer.
func (s *Select[T]) Options() []Option[T] { return slices.Clone(s.options) }

// Chosen is the option under the cursor, and whether there is one.
func (s *Select[T]) Chosen() (Option[T], bool) {
	s.ensure()
	return s.list.Current()
}

// Measure is the label, the options within their cap, and the problem if there is one.
func (s *Select[T]) Measure(int) int {
	rows := len(s.options)
	if s.Rows > 0 {
		rows = min(rows, s.Rows)
	}
	return layout.Sum(max(rows, 1), s.rows(s.Label))
}

// Draw paints the label, the options and whatever was wrong with the choice.
func (s *Select[T]) Draw(v Frame) {
	s.drawField(v, Look{})
}

func (s *Select[T]) drawField(v Frame, look Look) {
	selected := s.list.Selected()
	if s.Value != nil {
		selected, _ = s.indexOf(s.Value.Value())
	}
	s.list.drawRows(s.frame(v, s.Label, look), selected, func(v grid.View, _ int, option Option[T], under bool) {
		look.choice(v, option.Label, under, under)
	})
}

func (s *Select[T]) indexOf(want T) (int, bool) {
	for i, option := range s.options {
		if option.holds(want, s.Same) {
			return i, true
		}
	}
	return s.list.Selected(), false
}

// Handle moves the cursor, and takes the choice with it.
func (s *Select[T]) Handle(ev input.Event) bool {
	s.ensure()
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := s.within(mouse)
		if !in {
			return false
		}
		ev = local
	}
	before := s.list.Selected()
	if !s.list.Handle(ev) {
		return false
	}
	if s.list.Selected() != before {
		s.store()
	}
	return true
}

// Do runs one of the field's actions by name. See [Doer].
func (s *Select[T]) Do(action keymap.Action) bool {
	s.ensure()
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
	s.ensure()
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
	s.ensure()
	s.list.Focus(has)
	if s.leaving(has) {
		_ = s.Validate()
	}
}

func (s *Select[T]) ensure() {
	s.list.Keys = s.Keys
	s.seeded = true
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
	// options are private because replacing them has to move the taken set by value,
	// not attach old boolean positions to unrelated new choices.
	options []Option[T]
	// Value is the caller-owned set, in option order. A caller change is observed at the
	// next semantic operation and by drawing. Unavailable choices are discarded,
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

	list    List[Option[T]]
	taken   []bool
	limit   int
	seeded  bool
	matcher keymap.Matcher
}

// Prompt is what the field is asking for.
func (m *MultiSelect[T]) Prompt() string { return m.Label }

// SetOptions replaces what is on offer. MultiSelect owns the slice and preserves
// each taken choice that remains available under Same, or under its label when Same
// is nil, wherever it moved.
func (m *MultiSelect[T]) SetOptions(options []Option[T]) {
	var previous []Option[T]
	if m.seeded && m.Value == nil {
		previous = m.takenOptions()
	}
	m.options = own(m.options, options)
	for i := range m.options {
		m.options[i].Label = strings.Clone(m.options[i].Label)
	}
	m.list.SetItems(m.options)
	m.taken = make([]bool, len(m.options))
	if !m.seeded {
		return
	}
	if m.Value != nil {
		m.sync()
		return
	}
	for _, want := range previous {
		for i, option := range m.options {
			if !m.taken[i] && option.sameOption(want, m.Same) {
				m.taken[i] = true
				break
			}
		}
	}
	clampTaken(m.taken, m.limit)
	m.store()
}

// Options returns a copy of what is on offer.
func (m *MultiSelect[T]) Options() []Option[T] { return slices.Clone(m.options) }

// SetLimit changes how many choices may be taken at once. Zero allows every option;
// a negative limit is a programmer error. Lowering the limit keeps the earliest
// choices in option order and writes the settled set back to a bound value. A nil
// receiver ignores the change.
func (m *MultiSelect[T]) SetLimit(limit int) {
	if m == nil {
		return
	}
	if limit < 0 {
		panic("headless: multi-select limit cannot be negative")
	}
	if m.limit == limit {
		if m.seeded {
			m.sync()
		}
		return
	}
	m.limit = limit
	if m.seeded {
		// Reconcile against the new limit once. Canonicalizing against the old limit
		// first could publish two intermediate values for one semantic operation.
		m.sync()
	}
}

// Limit reports how many choices may be taken at once. Zero allows every option.
func (m *MultiSelect[T]) Limit() int {
	if m == nil {
		return 0
	}
	return m.limit
}

// Taken is what has been chosen, in the order the options are listed.
func (m *MultiSelect[T]) Taken() []T {
	m.ensure()
	return m.takenValues()
}

func (m *MultiSelect[T]) takenValues() []T {
	var out []T
	for i, option := range m.options {
		if i < len(m.taken) && m.taken[i] {
			out = append(out, option.Value)
		}
	}
	return out
}

func (m *MultiSelect[T]) takenOptions() []Option[T] {
	var out []Option[T]
	for i, option := range m.options {
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
	m.ensure()
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

// Measure is the label, the options within their cap, and the problem if there is one.
func (m *MultiSelect[T]) Measure(int) int {
	rows := len(m.options)
	if m.Rows > 0 {
		rows = min(rows, m.Rows)
	}
	return layout.Sum(max(rows, 1), m.rows(m.Label))
}

// Draw paints the label, the options and whatever was wrong with the choices.
func (m *MultiSelect[T]) Draw(v Frame) {
	m.drawField(v, Look{})
}

func (m *MultiSelect[T]) drawField(v Frame, look Look) {
	taken := m.taken
	if !m.seeded || m.Value != nil {
		taken, _ = m.selection()
	}
	m.list.DrawRows(m.frame(v, m.Label, look), func(v grid.View, at int, option Option[T], under bool) {
		look.choice(v, option.Label, under, at < len(taken) && taken[at])
	})
}

func (m *MultiSelect[T]) selection() ([]bool, bool) {
	if m.Value == nil {
		taken := make([]bool, len(m.options))
		copy(taken, m.taken)
		return taken, !clampTaken(taken, m.limit)
	}
	return m.selectionOf(m.Value.Value())
}

func (m *MultiSelect[T]) selectionOf(bound []T) ([]bool, bool) {
	taken := make([]bool, len(m.options))
	for _, want := range bound {
		for i, option := range m.options {
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
	for i, option := range m.options {
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
	m.ensure()
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
	m.ensure()
	if action == Toggle {
		m.Toggle()
		return true
	}
	return m.list.Do(action)
}

// Validate checks the choices.
func (m *MultiSelect[T]) Validate() error {
	m.ensure()
	if m.Check == nil {
		return m.check(nil)
	}
	return m.check(m.Check(m.Taken()))
}

// Focus takes the keyboard or gives it up, and checks the choices on the way out.
func (m *MultiSelect[T]) Focus(has bool) {
	m.ensure()
	if !has {
		m.matcher.Clear()
	}
	if m.leaving(has) {
		_ = m.Validate()
	}
}

func (m *MultiSelect[T]) ensure() {
	// The list inside has no map of its own: this field resolves every keystroke
	// against one that has the movement and the key that takes a choice in it, and
	// drives the list by name. Offering the event to both would resolve it twice.
	m.seeded = true
	m.sync()
}

// sync makes the caller-owned set the selection model's source. Unlike Draw, this is
// a semantic boundary, so it also settles a non-canonical value exactly once.
func (m *MultiSelect[T]) sync() {
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

// Measure is the label, the two answers on one row, and the problem if there is one.
func (c *Confirm) Measure(int) int { return layout.Sum(1, c.rows(c.Label)) }

// Draw paints the label and the two answers.
func (c *Confirm) Draw(v Frame) {
	c.drawField(v, Look{})
}

func (c *Confirm) drawField(v Frame, look Look) {
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
