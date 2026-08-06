package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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
type Text struct {
	field
	// Label is what the field is asking for.
	Label string
	// Value is where what is typed goes. It is read once, when the field is first used,
	// and written whenever the text changes.
	Value Accessor[string]
	// Check says what is wrong with what has been entered, or nil. It is asked when the
	// keyboard leaves the field and when the form is submitted.
	Check func(s string) error
	// Placeholder is shown while the field is empty, and Mask is what each cluster is
	// drawn as for something the screen should not show.
	Placeholder, Mask string
	// Keys say which keystrokes edit. Nil reads through [DefaultEditorKeys].
	Keys *keymap.Map

	editor Editor
	seeded bool
}

// Editor is the field itself, for a caller that needs the cursor or the clipboard.
func (t *Text) Editor() *Editor { return &t.editor }

// Prompt is what the field is asking for.
func (t *Text) Prompt() string { return t.Label }

// Measure is the label, a row of text, and the problem with it if there is one.
func (t *Text) Measure(int) int { return 1 + t.rows(t.Label) }

// Draw paints the label, the field and whatever was wrong with the answer.
func (t *Text) Draw(v grid.View) {
	t.ensure()
	t.editor.Look = t.look
	t.editor.Draw(t.frame(v, t.Label))
}

// Handle passes input to the field and keeps the value in step with it.
func (t *Text) Handle(ev input.Event) bool {
	t.ensure()
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := t.within(mouse)
		if !in {
			return false
		}
		return t.editor.HandleMouse(local, t.inner.X)
	}
	if !t.editor.Handle(ev) {
		return false
	}
	t.store()
	return true
}

// Do runs one of the field's actions by name. See [Doer].
func (t *Text) Do(action keymap.Action) bool {
	t.ensure()
	if !t.editor.Do(action) {
		return false
	}
	t.store()
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

// ensure keeps the field in step with what the caller set, and gives it its first look
// at the value it is collecting.
func (t *Text) ensure() {
	t.editor.SingleLine = true
	t.editor.Mask = t.Mask
	t.editor.Placeholder = t.Placeholder
	t.editor.Keys = t.Keys
	if t.seeded {
		return
	}
	t.seeded = true
	if t.Value != nil {
		t.editor.SetText(t.Value.Get())
	}
}

// store puts what has been typed where the caller keeps it.
func (t *Text) store() {
	if t.Value != nil {
		t.Value.Set(t.editor.Text())
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
type Select[T any] struct {
	field
	// Label is what the field is asking for.
	Label string
	// Options are what is on offer.
	Options []Option[T]
	// Value is where the choice goes. It is read once, to put the cursor on what is
	// already chosen, and written whenever the cursor moves.
	Value Accessor[T]
	// Same says whether two values are the same one, which is what puts the cursor on
	// the choice already made. Nil compares what is shown instead, which is right
	// whenever the labels are distinct and is the only thing possible for a type Go
	// will not compare.
	Same func(a, b T) bool
	// Check says what is wrong with the choice, or nil.
	Check func(v T) error
	// Rows caps how many options are shown at once. Zero shows them all.
	Rows int
	// Keys say which keystrokes move the cursor. Nil reads through [DefaultListKeys].
	Keys *keymap.Map

	list   List[Option[T]]
	seeded bool
}

// Prompt is what the field is asking for.
func (s *Select[T]) Prompt() string { return s.Label }

// Chosen is the option under the cursor, and whether there is one.
func (s *Select[T]) Chosen() (Option[T], bool) {
	s.ensure()
	return s.list.Current()
}

// Measure is the label, the options within their cap, and the problem if there is one.
func (s *Select[T]) Measure(int) int {
	s.ensure()
	rows := len(s.Options)
	if s.Rows > 0 {
		rows = min(rows, s.Rows)
	}
	return max(rows, 1) + s.rows(s.Label)
}

// Draw paints the label, the options and whatever was wrong with the choice.
func (s *Select[T]) Draw(v grid.View) {
	s.ensure()
	s.list.Draw(s.frame(v, s.Label))
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
	if !s.list.Handle(ev) {
		return false
	}
	s.store()
	return true
}

// Do runs one of the field's actions by name. See [Doer].
func (s *Select[T]) Do(action keymap.Action) bool {
	s.ensure()
	if !s.list.Do(action) {
		return false
	}
	s.store()
	return true
}

// Validate checks the choice.
func (s *Select[T]) Validate() error {
	s.ensure()
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
	if s.leaving(has) {
		_ = s.Validate()
	}
}

func (s *Select[T]) ensure() {
	s.list.Items = s.Options
	s.list.Keys = s.Keys
	if s.seeded {
		return
	}
	s.seeded = true
	// Wired once rather than every frame: it reads the look and the options through
	// this field, so it does not go stale, and a Draw that assigns to the thing it is
	// about to draw is a Draw with a side effect.
	s.list.Row = func(v grid.View, _ int, option Option[T], under bool) {
		s.look.choice(v, option.Label, under, under)
	}
	if s.Value == nil {
		s.store()
		return
	}
	// The cursor starts on the choice already made, which is what makes a form somebody
	// is coming back to show what they said last time.
	want := s.Value.Get()
	for i, option := range s.Options {
		if option.holds(want, s.Same) {
			s.list.Select(i)
			break
		}
	}
	s.store()
}

func (s *Select[T]) store() {
	if s.Value == nil {
		return
	}
	if chosen, ok := s.list.Current(); ok {
		s.Value.Set(chosen.Value)
	}
}

// MultiSelect is a field holding any number of choices out of several.
//
// The cursor and the choice are two things here, unlike in a [Select]: moving is not
// choosing, and something has to be pressed. That is the whole difference between
// picking one and picking some.
type MultiSelect[T any] struct {
	field
	// Label is what the field is asking for.
	Label string
	// Options are what is on offer.
	Options []Option[T]
	// Value is where the choices go, in the order the options are listed.
	Value Accessor[[]T]
	// Same says whether two values are the same one — see [Select.Same].
	Same func(a, b T) bool
	// Check says what is wrong with the choices, or nil.
	Check func(v []T) error
	// Limit is how many may be taken at once. Zero is as many as there are.
	Limit int
	// Rows caps how many options are shown at once. Zero shows them all.
	Rows int
	// Keys say which keystrokes move and take. Nil reads through
	// [DefaultMultiSelectKeys].
	Keys *keymap.Map

	list    List[Option[T]]
	taken   []bool
	seeded  bool
	pending keymap.Pending
}

// Prompt is what the field is asking for.
func (m *MultiSelect[T]) Prompt() string { return m.Label }

// Taken is what has been chosen, in the order the options are listed.
func (m *MultiSelect[T]) Taken() []T {
	m.ensure()
	var out []T
	for i, option := range m.Options {
		if i < len(m.taken) && m.taken[i] {
			out = append(out, option.Value)
		}
	}
	return out
}

// Toggle takes the option under the cursor, or gives it back, and reports whether
// anything changed. Nothing changes when the limit is reached.
func (m *MultiSelect[T]) Toggle() bool {
	m.ensure()
	at := m.list.Selected()
	if at < 0 || at >= len(m.taken) {
		return false
	}
	if !m.taken[at] && m.Limit > 0 && len(m.Taken()) >= m.Limit {
		return false
	}
	m.taken[at] = !m.taken[at]
	m.store()
	return true
}

// Measure is the label, the options within their cap, and the problem if there is one.
func (m *MultiSelect[T]) Measure(int) int {
	m.ensure()
	rows := len(m.Options)
	if m.Rows > 0 {
		rows = min(rows, m.Rows)
	}
	return max(rows, 1) + m.rows(m.Label)
}

// Draw paints the label, the options and whatever was wrong with the choices.
func (m *MultiSelect[T]) Draw(v grid.View) {
	m.ensure()
	m.list.Draw(m.frame(v, m.Label))
}

// Handle moves the cursor and takes choices.
func (m *MultiSelect[T]) Handle(ev input.Event) bool {
	m.ensure()
	if key, ok := ev.(input.Key); ok {
		if action, mine := m.keys().Lookup(key, &m.pending); mine {
			if action == "" {
				return true
			}
			return m.Do(action)
		}
		return false
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
	if m.leaving(has) {
		_ = m.Validate()
	}
}

func (m *MultiSelect[T]) ensure() {
	m.list.Items = m.Options
	// The list inside has no map of its own: this field resolves every keystroke
	// against one that has the movement and the key that takes a choice in it, and
	// drives the list by name. Offering the event to both would resolve it twice.
	if m.list.Row == nil {
		m.list.Row = func(v grid.View, at int, option Option[T], under bool) {
			m.look.choice(v, option.Label, under, at < len(m.taken) && m.taken[at])
		}
	}
	if len(m.taken) != len(m.Options) {
		taken := make([]bool, len(m.Options))
		copy(taken, m.taken)
		m.taken = taken
	}
	if m.seeded {
		return
	}
	m.seeded = true
	if m.Value == nil {
		return
	}
	for _, want := range m.Value.Get() {
		for i, option := range m.Options {
			if option.holds(want, m.Same) {
				m.taken[i] = true
				break
			}
		}
	}
}

func (m *MultiSelect[T]) store() {
	if m.Value != nil {
		m.Value.Set(m.Taken())
	}
}

func (m *MultiSelect[T]) keys() *keymap.Map {
	if m.Keys != nil {
		return m.Keys
	}
	return multiSelectKeys()
}

// Confirm is a field holding a yes or a no.
type Confirm struct {
	field
	// Label is what the field is asking.
	Label string
	// Value is where the answer goes.
	Value Accessor[bool]
	// Yes and No are the two answers as they are shown. Empty uses "yes" and "no".
	Yes, No string
	// Check says what is wrong with the answer, or nil.
	Check func(v bool) error
	// Keys say which keystrokes answer. Nil reads through [DefaultConfirmKeys].
	Keys *keymap.Map

	yes     bool
	seeded  bool
	pending keymap.Pending
	// split is the column the second answer begins at in the last frame, which is what
	// a press has to be answered against.
	split int
}

// Prompt is what the field is asking.
func (c *Confirm) Prompt() string { return c.Label }

// Answer is what has been answered.
func (c *Confirm) Answer() bool {
	c.ensure()
	return c.yes
}

// Say answers the field.
func (c *Confirm) Say(yes bool) {
	c.ensure()
	c.yes = yes
	if c.Value != nil {
		c.Value.Set(yes)
	}
}

// Measure is the label, the two answers on one row, and the problem if there is one.
func (c *Confirm) Measure(int) int { return 1 + c.rows(c.Label) }

// Draw paints the label and the two answers.
func (c *Confirm) Draw(v grid.View) {
	c.ensure()
	row := c.frame(v, c.Label)
	w, h := row.Size()
	if w <= 0 || h <= 0 {
		return
	}
	x := 0
	for _, yes := range []bool{true, false} {
		if !yes {
			// Where one answer ends and the other begins, which is the whole of what a
			// press needs to know.
			c.split = x
		}
		style := c.look.Text
		if yes == c.yes {
			style = c.look.Selection.Merge(c.look.Accent)
		}
		if mark, width := c.look.mark(yes == c.yes); width > 0 {
			x += row.Text(x, 0, mark, style)
			x++
		}
		x += row.Text(x, 0, c.word(yes), style)
		x += row.Text(x, 0, "  ", c.look.Text)
	}
}

// Handle answers the field, by key or by pressing one of the two answers.
func (c *Confirm) Handle(ev input.Event) bool {
	c.ensure()
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := c.within(mouse)
		if !in || local.Action != input.MouseDown || local.Button != input.ButtonLeft {
			return false
		}
		c.Say(local.Pos.X < c.split)
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	action, mine := c.keys().Lookup(key, &c.pending)
	switch {
	case !mine:
		return false
	case action == "":
		return true
	}
	return c.Do(action)
}

// Do runs one of the field's actions by name. See [Doer].
func (c *Confirm) Do(action keymap.Action) bool {
	c.ensure()
	switch action {
	case SelectPrev:
		c.Say(true)
	case SelectNext:
		c.Say(false)
	case Toggle:
		c.Say(!c.yes)
	default:
		return false
	}
	return true
}

// Validate checks the answer.
func (c *Confirm) Validate() error {
	c.ensure()
	if c.Check == nil {
		return c.check(nil)
	}
	return c.check(c.Check(c.yes))
}

// Focus takes the keyboard or gives it up, and checks the answer on the way out.
func (c *Confirm) Focus(has bool) {
	c.ensure()
	if c.leaving(has) {
		_ = c.Validate()
	}
}

func (c *Confirm) ensure() {
	if c.seeded {
		return
	}
	c.seeded = true
	if c.Value != nil {
		c.yes = c.Value.Get()
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
