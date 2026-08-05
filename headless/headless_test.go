package headless

import (
	"image"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/primitives/grid"
	"github.com/Tangerg/oolong/primitives/input"
	"github.com/Tangerg/oolong/program"
)

// Anything interactive here runs as a program's root without an adapter. The two
// interfaces are declared separately so the loop does not depend on the widgets;
// that they stay the same method set is a promise, and this is where it is kept.
//
// A test may reach across rings where production code may not, which is the whole
// reason this assertion can live beside the interface it is about.
var (
	_ program.Component = (*Editor)(nil)
	_ program.Component = (*List[string])(nil)
	_ program.Component = (*Completion)(nil)
	_ Interactive       = (*Editor)(nil)
)

var (
	file    = Trigger{Prefix: "@"}
	command = Trigger{Prefix: "/", AtStart: true}
)

// paint draws a widget into a surface of the given size and returns what it looks
// like, one string per row with a dot for a blank cell.
func paint(w, h int, draw func(grid.View)) []string {
	s := grid.NewSurface(w, h)
	draw(s.View())
	rows := make([]string, 0, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c := s.CellAt(x, y)
			switch {
			case c.Width() == 0:
			case c.Content == "":
				b.WriteByte('.')
			default:
				b.WriteString(c.Content)
			}
		}
		rows = append(rows, b.String())
	}
	return rows
}

func equalRows(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("drawn:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func key(code input.Code) input.Event { return input.Key{Code: code} }

// offer opens a completion on a token with plain candidates.
func offer(c *Completion, texts ...string) Token {
	token := Token{Start: 1, End: 4, Query: "src", Trigger: file}
	candidates := make([]Candidate, len(texts))
	for i, s := range texts {
		candidates[i] = Candidate{Text: s}
	}
	c.Offer(token, candidates)
	return token
}

// typeText sends each character of s to the editor as a keystroke.
func typeText(e *Editor, s string) {
	for _, r := range s {
		e.Handle(input.Key{Code: input.Character, Rune: r})
	}
}

func ctrlKey(r rune) input.Event {
	return input.Key{Code: input.Character, Rune: r, Mods: input.Ctrl}
}

func altKey(r rune) input.Event {
	return input.Key{Code: input.Character, Rune: r, Mods: input.Alt}
}

// cursorAt lays the editor out at a width and returns the cursor's row and column.

// cursorAt lays the editor out at a width and returns the cursor's row and column.
func cursorAt(e *Editor, w, h int) (int, int) {
	surface := grid.NewSurface(w, h)
	// A plain surface has no cursor slot, so the position is taken from the layout
	// the editor itself reports.
	e.Draw(surface.View())
	rows := e.layout.rowsFor(e.lines, w)
	row, column := rowOf(rows, e.lines, e.line, e.col)
	return row - e.scroll.Offset(), column
}

func wheel(action input.MouseAction) input.Event {
	return input.Mouse{Action: action}
}

// items builds a list of numbered strings.
func items(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = "item" + strconv.Itoa(i)
	}
	return out
}

// newList builds a list that draws each item as its text, marking the selected one.

// newList builds a list that draws each item as its text, marking the selected one.
func newList(n int) *List[string] {
	return &List[string]{
		Items: items(n),
		Keys:  DefaultListKeys(),
		Row: func(v grid.View, item string, selected bool) {
			prefix := " "
			if selected {
				prefix = ">"
			}
			v.Text(0, 0, prefix+item, grid.Style{})
		},
	}
}

// press builds a mouse event.
func press(x, y int, action input.MouseAction, button input.Button) input.Event {
	return input.Mouse{Pos: image.Pt(x, y), Action: action, Button: button}
}

func TestBindingMatchesTheKeystrokeItDescribes(t *testing.T) {
	// The hint and the handler have to be talking about the same thing, which is the
	// whole reason they are one value.
	b := Binding{Key: input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl}, Does: "save"}
	if !b.Matches(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl}) {
		t.Error("a binding does not match its own keystroke")
	}
	if b.Matches(input.Key{Code: input.Character, Rune: 's'}) {
		t.Error("a binding matched the same letter without its modifier")
	}
	if b.Matches(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl, Transition: input.Release}) {
		t.Error("a binding fired on the key coming back up")
	}
	if got := b.Key.String(); got != "ctrl+s" {
		t.Fatalf("hint text = %q", got)
	}
}

// widgetFunc adapts a function to [Widget].

func TestTokenAt(t *testing.T) {
	for _, tc := range []struct {
		what     string
		line     string
		cursor   int
		triggers []Trigger
		want     string // "start:end:query" or "" for no token
	}{
		{"a mention being typed", "look at @src/ma", 15, []Trigger{file}, "9:15:src/ma"},
		{"the prefix alone", "@", 1, []Trigger{file}, "1:1:"},
		{"a command at the start", "/hel", 4, []Trigger{command}, "1:4:hel"},
		{"a command not at the start", "and /hel", 8, []Trigger{command}, ""},
		{"a slash inside a path", "@src/ma", 7, []Trigger{file, command}, "1:7:src/ma"},
		{
			"the rightmost trigger wins", "/open @src", 10,
			[]Trigger{file, command}, "7:10:src",
		},
		{"an email address", "write to me@example.com", 23, []Trigger{file}, ""},
		{"the cursor before the trigger", "@src", 0, []Trigger{file}, ""},
		{
			"the cursor inside the token replaces all of it", "@source", 4,
			[]Trigger{file}, "1:7:sou",
		},
		{"the cursor past the token", "@src and more", 9, []Trigger{file}, ""},
		{"a token that ends at a space", "@src more", 4, []Trigger{file}, "1:4:src"},
		{"nothing to complete", "just words", 10, []Trigger{file, command}, ""},
		{"no triggers at all", "@src", 4, nil, ""},
		{"an empty trigger", "@src", 4, []Trigger{{}}, ""},
	} {
		token, ok := TokenAt(tc.line, tc.cursor, tc.triggers...)
		got := ""
		if ok {
			got = strings.Join([]string{
				strconv.Itoa(token.Start), strconv.Itoa(token.End), token.Query,
			}, ":")
		}
		if got != tc.want {
			t.Errorf("%s: TokenAt(%q, %d) = %q, want %q", tc.what, tc.line, tc.cursor, got, tc.want)
		}
	}
}

func TestTokenAtIsClampedToTheLine(t *testing.T) {
	// A cursor from somewhere that has since changed the text must not index out of it.
	for _, cursor := range []int{-5, 100} {
		if _, ok := TokenAt("@src", cursor, file); ok && cursor < 0 {
			t.Errorf("cursor %d found a token", cursor)
		}
	}
}

// offer opens a completion on a token with plain candidates.

func TestAnEmptyOfferIsADismissal(t *testing.T) {
	// A popup with nothing in it is a popup in the way.
	var c Completion
	offer(&c, "one")
	c.Offer(Token{}, nil)
	if c.Open() {
		t.Fatal("an offer of nothing left the completion open")
	}
	if c.Measure(20) != 0 {
		t.Fatal("a closed completion asked for room")
	}
}

func TestAClosedCompletionAnswersNothing(t *testing.T) {
	// It would be a completion with opinions about text it is not offering anything for.
	var c Completion
	c.Accept = func(Candidate, Token) { t.Fatal("a closed completion accepted") }
	for _, ev := range []input.Event{
		input.Key{Code: input.Down},
		input.Key{Code: input.Tab},
		input.Key{Code: input.Esc},
		input.Key{Code: input.Enter},
	} {
		if c.Handle(ev) {
			t.Errorf("a closed completion consumed %v", ev)
		}
	}
}

func TestAcceptingReportsTheCandidateAndTheTokenItReplaces(t *testing.T) {
	var c Completion
	var got Candidate
	var at Token
	c.Accept = func(candidate Candidate, token Token) { got, at = candidate, token }
	token := offer(&c, "src/one.go", "src/two.go")

	c.Handle(input.Key{Code: input.Down})
	if !c.Handle(input.Key{Code: input.Tab}) {
		t.Fatal("accepting was not handled")
	}
	if got.Text != "src/two.go" {
		t.Fatalf("accepted %q, want the one under the cursor", got.Text)
	}
	if at != token {
		t.Fatalf("token = %+v, want %+v", at, token)
	}
	if c.Open() {
		t.Fatal("the completion stayed open after accepting")
	}
}

func TestAcceptingClosesBeforeTheCallbackRuns(t *testing.T) {
	// The callback is about to change the text the token came from, and that change
	// must not read as another query for a completion that is still open.
	var c Completion
	openDuring := true
	c.Accept = func(Candidate, Token) { openDuring = c.Open() }
	offer(&c, "one")
	c.Handle(input.Key{Code: input.Tab})
	if openDuring {
		t.Fatal("the completion was still open while the text was being changed")
	}
}

func TestWithNowhereToSendItAcceptingIsNotConsumed(t *testing.T) {
	// A key swallowed by a widget that then does nothing with it is a key the user
	// pressed for no reason.
	var c Completion
	offer(&c, "one")
	if c.Handle(input.Key{Code: input.Tab}) {
		t.Fatal("accepting was consumed with nothing to accept into")
	}
	if !c.Open() {
		t.Fatal("the completion closed anyway")
	}
}

func TestDismissing(t *testing.T) {
	var c Completion
	offer(&c, "one", "two")
	if !c.Handle(input.Key{Code: input.Esc}) {
		t.Fatal("escape was not handled")
	}
	if c.Open() {
		t.Fatal("escape left the completion open")
	}
}

func TestMovingThroughTheCandidates(t *testing.T) {
	var c Completion
	offer(&c, "one", "two", "three")
	if got, _ := c.Current(); got.Text != "one" {
		t.Fatalf("opens on %q, want the first", got.Text)
	}
	c.Handle(input.Key{Code: input.Down})
	c.Handle(input.Key{Code: input.Down})
	if got, _ := c.Current(); got.Text != "three" {
		t.Fatalf("after two moves down = %q", got.Text)
	}
	// Not past the end: the list does not wrap, because in a long one wrapping loses
	// the reader's place.
	c.Handle(input.Key{Code: input.Down})
	if got, _ := c.Current(); got.Text != "three" {
		t.Fatalf("moved past the last candidate to %q", got.Text)
	}
}

func TestANewOfferStartsAtTheFirstCandidate(t *testing.T) {
	// The query changed, so which candidate was under the cursor answers a question
	// nobody asked.
	var c Completion
	offer(&c, "one", "two")
	c.Handle(input.Key{Code: input.Down})
	offer(&c, "alpha", "beta")
	if got, _ := c.Current(); got.Text != "alpha" {
		t.Fatalf("a new offer opens on %q, want the first", got.Text)
	}
}

func TestTheMeasuredHeightIsARowPerCandidateUpToTheCap(t *testing.T) {
	var c Completion
	c.MaxRows = 3
	offer(&c, "one", "two")
	if got := c.Measure(20); got != 2 {
		t.Fatalf("height = %d, want a row each", got)
	}
	offer(&c, "one", "two", "three", "four", "five")
	if got := c.Measure(20); got != 3 {
		t.Fatalf("height = %d, want the cap", got)
	}
}

func TestTheWidthFitsTheWidestRow(t *testing.T) {
	var c Completion
	c.Offer(Token{}, []Candidate{
		{Text: "short"},
		{Text: "much longer one", Detail: "dir"},
	})
	if got, want := c.Width(), len("much longer one")+detailGap+len("dir"); got != want {
		t.Fatalf("width = %d, want %d", got, want)
	}
}

func TestARowShowsItsLabelAndDetail(t *testing.T) {
	var c Completion
	c.Offer(Token{}, []Candidate{{Text: "src/main.go", Label: "main.go", Detail: "src"}})
	rows := paint(20, 1, c.Draw)
	if !strings.Contains(rows[0], "main.go") {
		t.Fatalf("row = %q, want the label rather than the text", rows[0])
	}
	if !strings.HasSuffix(strings.TrimRight(rows[0], "."), "src") {
		t.Fatalf("row = %q, want the detail on the right", rows[0])
	}
}

func TestARowWithNoLabelShowsWhatItWouldInsert(t *testing.T) {
	var c Completion
	c.Offer(Token{}, []Candidate{{Text: "insert-me"}})
	if rows := paint(20, 1, c.Draw); !strings.Contains(rows[0], "insert-me") {
		t.Fatalf("row = %q", rows[0])
	}
}

func TestTheMatchedCharactersArePickedOut(t *testing.T) {
	var c Completion
	c.MatchStyle = grid.Style{Attr: grid.Bold}
	c.Offer(Token{}, []Candidate{{Text: "status", Matched: []int{0, 1}}})

	s := grid.NewSurface(20, 1)
	c.Draw(s.View())
	for x, want := range []bool{true, true, false, false, false, false} {
		if got := s.CellAt(x, 0).Style.Attr.Has(grid.Bold); got != want {
			t.Errorf("column %d emphasised = %v, want %v", x, got, want)
		}
	}
}

func TestAMatchInsideAClusterEmphasisesTheWholeCluster(t *testing.T) {
	// A pattern character can match a combining mark, whose offset is inside the
	// cluster rather than at its start. Testing for equality there would leave the
	// character the reader sees unhighlighted.
	var c Completion
	c.MatchStyle = grid.Style{Attr: grid.Bold}
	// "e" plus a combining acute: one cluster, two runes, the mark at offset 1.
	c.Offer(Token{}, []Candidate{{Text: "éx", Matched: []int{1}}})

	s := grid.NewSurface(10, 1)
	c.Draw(s.View())
	if !s.CellAt(0, 0).Style.Attr.Has(grid.Bold) {
		t.Fatal("the matched cluster was not emphasised")
	}
	if s.CellAt(1, 0).Style.Attr.Has(grid.Bold) {
		t.Fatal("the cluster after the match was emphasised")
	}
}

func TestTheSelectedRowIsTheOneUnderTheCursor(t *testing.T) {
	var c Completion
	c.SelectedStyle = grid.Style{Attr: grid.Reverse}
	offer(&c, "one", "two")
	c.Handle(input.Key{Code: input.Down})

	s := grid.NewSurface(20, 2)
	c.Draw(s.View())
	if s.CellAt(0, 0).Style.Attr.Has(grid.Reverse) {
		t.Error("the row that is not selected is drawn as selected")
	}
	if !s.CellAt(0, 1).Style.Attr.Has(grid.Reverse) {
		t.Error("the selected row is not drawn as selected")
	}
}

func TestADetailWithNoRoomIsDropped(t *testing.T) {
	// Half a description reads as a broken label. None of this may overflow the row.
	var c Completion
	c.Offer(Token{}, []Candidate{{Text: "a-fairly-long-label", Detail: "and a description"}})
	rows := paint(12, 1, c.Draw)
	if len(rows[0]) != 12 {
		t.Fatalf("row = %q, want twelve columns", rows[0])
	}
}

func TestACompletionWithNoRoomDrawsNothing(t *testing.T) {
	var c Completion
	offer(&c, "one", "two")
	for _, size := range [][2]int{{0, 0}, {1, 1}, {4, 1}} {
		paint(size[0], size[1], c.Draw)
	}
}

func TestAcceptingACompletionIsOneUndoStep(t *testing.T) {
	// Which is the reason Editor.Replace exists: taking back one thing the user did
	// should not take two.
	e := NewEditor()
	e.SetText("look at @src")
	token, ok := TokenAt(e.Text(), len("look at @src"), file)
	if !ok {
		t.Fatal("no token")
	}
	e.Replace(token.Start, token.End, "src/main.go")
	if got := e.Text(); got != "look at @src/main.go" {
		t.Fatalf("after accepting = %q", got)
	}
	if _, col := e.Cursor(); col != len("look at @src/main.go") {
		t.Fatalf("cursor at %d, want after what was put in", col)
	}
	e.Undo()
	if got := e.Text(); got != "look at @src" {
		t.Fatalf("after one undo = %q, want what was there before", got)
	}
}

func TestReplaceIsClampedToTheLine(t *testing.T) {
	e := NewEditor()
	e.SetText("abc")
	e.Replace(-3, 99, "x")
	if got := e.Text(); got != "x" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorTyping(t *testing.T) {
	e := NewEditor()
	typeText(e, "hello")
	if got := e.Text(); got != "hello" {
		t.Fatalf("text = %q", got)
	}
	_, col := e.Cursor()
	if col != 5 {
		t.Fatalf("cursor at %d, want the end", col)
	}
}

func TestEditorRefusesControlCharacters(t *testing.T) {
	// A control character has no width and would be dropped at the cell, leaving a
	// cursor position with nothing under it.
	e := NewEditor()
	e.InsertRune('\x1b')
	e.InsertRune('\r')
	if !e.Empty() {
		t.Fatalf("text = %q, want nothing", e.Text())
	}
	e.InsertRune('\t')
	if got := e.Text(); got != "\t" {
		t.Fatalf("text = %q, want the tab kept", got)
	}
}

func TestEditorMovesByClusterNotByByteOrRune(t *testing.T) {
	e := NewEditor()
	e.Insert("a中b")
	e.MoveLineStart()
	e.MoveRight()
	if _, col := e.Cursor(); col != 1 {
		t.Fatalf("after one right the cursor is at %d, want past the letter", col)
	}
	e.MoveRight()
	// The wide character is three bytes and one cluster: one press crosses it whole.
	if _, col := e.Cursor(); col != 4 {
		t.Fatalf("after two rights the cursor is at %d, want past the wide character", col)
	}
	e.MoveLeft()
	if _, col := e.Cursor(); col != 1 {
		t.Fatalf("after moving back the cursor is at %d", col)
	}
}

func TestEditorCursorSitsOnTheColumnItsCharacterOccupies(t *testing.T) {
	e := NewEditor()
	e.Insert("中文x")
	_, column := cursorAt(e, 20, 3)
	// Two wide characters are four columns, so the cursor after the letter is at five.
	if column != 5 {
		t.Fatalf("cursor column = %d, want 5", column)
	}
}

func TestEditorNewlineSplitsTheLine(t *testing.T) {
	e := NewEditor()
	e.Insert("hello world")
	e.MoveLineStart()
	for range 5 {
		e.MoveRight()
	}
	e.Newline()
	if got := e.Text(); got != "hello\n world" {
		t.Fatalf("text = %q", got)
	}
	line, col := e.Cursor()
	if line != 1 || col != 0 {
		t.Fatalf("cursor at line %d column %d, want the start of the new line", line, col)
	}
}

func TestEditorPasteArrivesAsText(t *testing.T) {
	// The point of a bracketed paste: what was pasted goes in as it was, newlines and
	// all, rather than being interpreted a keystroke at a time.
	e := NewEditor()
	e.Handle(input.Paste{Text: "func main() {\n\tprintln(1)\n}"})
	if got := e.Text(); got != "func main() {\n\tprintln(1)\n}" {
		t.Fatalf("text = %q", got)
	}
	line, col := e.Cursor()
	if line != 2 || col != 1 {
		t.Fatalf("cursor at line %d column %d, want the end of the paste", line, col)
	}
}

func TestEditorBackspaceJoinsLines(t *testing.T) {
	e := NewEditor()
	e.Insert("one\ntwo")
	e.MoveLineStart()
	e.DeleteBack()
	if got := e.Text(); got != "onetwo" {
		t.Fatalf("text = %q", got)
	}
	if _, col := e.Cursor(); col != 3 {
		t.Fatalf("cursor at %d, want where the join happened", col)
	}
}

func TestEditorBackspaceAtTheVeryStartDoesNothing(t *testing.T) {
	e := NewEditor()
	e.Insert("x")
	e.MoveLineStart()
	e.DeleteBack()
	if got := e.Text(); got != "x" {
		t.Fatalf("text = %q, want it untouched", got)
	}
}

func TestEditorDeleteForwardJoinsTheLineBelow(t *testing.T) {
	e := NewEditor()
	e.Insert("one\ntwo")
	e.MoveLineStart()
	e.MoveUp()
	e.MoveLineEnd()
	e.DeleteForward()
	if got := e.Text(); got != "onetwo" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorWordMotions(t *testing.T) {
	e := NewEditor()
	e.Insert("alpha beta_two  gamma")
	e.MoveLineEnd()

	e.MoveWordLeft()
	if _, col := e.Cursor(); col != len("alpha beta_two  ") {
		t.Fatalf("cursor at %d, want the start of the last word", col)
	}
	e.MoveWordLeft()
	// The underscore is part of a word, so a word motion in code stops where a reader
	// expects rather than in the middle of an identifier.
	if _, col := e.Cursor(); col != len("alpha ") {
		t.Fatalf("cursor at %d, want the start of the identifier", col)
	}
	e.MoveWordRight()
	if _, col := e.Cursor(); col != len("alpha beta_two") {
		t.Fatalf("cursor at %d, want past the identifier", col)
	}
}

func TestEditorDeleteWordBack(t *testing.T) {
	e := NewEditor()
	e.Insert("one two three")
	e.Handle(ctrlKey('w'))
	if got := e.Text(); got != "one two " {
		t.Fatalf("text = %q", got)
	}
	// What it removed is kept, so it can be put back.
	e.Handle(ctrlKey('y'))
	if got := e.Text(); got != "one two three" {
		t.Fatalf("text after putting it back = %q", got)
	}
}

func TestEditorKillToEndAndBack(t *testing.T) {
	e := NewEditor()
	e.Insert("keep this away")
	e.MoveLineStart()
	for range len("keep this ") {
		e.MoveRight()
	}
	e.Handle(ctrlKey('k'))
	if got := e.Text(); got != "keep this " {
		t.Fatalf("text = %q", got)
	}
	e.Handle(ctrlKey('y'))
	if got := e.Text(); got != "keep this away" {
		t.Fatalf("text after putting it back = %q", got)
	}
}

func TestEditorKillToEndSwallowsTheLineBreakWhenThereIsNothingElse(t *testing.T) {
	// What makes repeated presses take a paragraph rather than stop at the first line.
	e := NewEditor()
	e.Insert("one\ntwo")
	e.MoveLineStart()
	e.MoveUp()
	e.MoveLineEnd()
	e.Handle(ctrlKey('k'))
	if got := e.Text(); got != "onetwo" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorKillToStart(t *testing.T) {
	e := NewEditor()
	e.Insert("drop this keep")
	e.MoveLineStart()
	for range len("drop this ") {
		e.MoveRight()
	}
	e.Handle(ctrlKey('u'))
	if got := e.Text(); got != "keep" {
		t.Fatalf("text = %q", got)
	}
	if _, col := e.Cursor(); col != 0 {
		t.Fatalf("cursor at %d, want the start", col)
	}
}

func TestEditorUndoStepsOverAPhraseNotALetter(t *testing.T) {
	// One undo per keystroke would make undo useless in a composer.
	e := NewEditor()
	typeText(e, "hello world")
	e.Undo()
	if !e.Empty() {
		t.Fatalf("text after one undo = %q, want the whole phrase gone", e.Text())
	}
}

func TestEditorUndoAndRedo(t *testing.T) {
	e := NewEditor()
	typeText(e, "first")
	e.Newline()
	typeText(e, "second")

	e.Undo()
	if got := e.Text(); got != "first\n" {
		t.Fatalf("after one undo = %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "first" {
		t.Fatalf("after two undos = %q", got)
	}
	e.Redo()
	if got := e.Text(); got != "first\n" {
		t.Fatalf("after redo = %q", got)
	}
	// A change after an undo abandons the redo history, which is what every editor
	// does and what stops a redo from resurrecting text the user has moved past.
	typeText(e, "third")
	e.Redo()
	if got := e.Text(); got != "first\nthird" {
		t.Fatalf("redo after a new change = %q, want it to have done nothing", got)
	}
}

func TestEditorUndoHistoryIsBounded(t *testing.T) {
	// An unbounded history in a long-lived process is a leak with a friendly name.
	e := NewEditor()
	for i := range maxUndo + 50 {
		e.Insert("x")
		e.MoveLeft()
		_ = i
	}
	if len(e.undo) > maxUndo {
		t.Fatalf("history holds %d steps, want at most %d", len(e.undo), maxUndo)
	}
}

func TestEditorVerticalMovementFollowsTheScreenNotTheParagraph(t *testing.T) {
	// A field that wraps has to move down the screen. Jumping to the next paragraph
	// would move the cursor somewhere the user cannot see the reason for.
	e := NewEditor()
	e.Insert("aaa bbb ccc ddd")
	if got := e.Measure(8); got != 2 {
		t.Fatalf("height at width 8 = %d, want the text wrapped onto 2 rows", got)
	}
	cursorAt(e, 8, 4)
	e.MoveLineStart()
	cursorAt(e, 8, 4)

	e.MoveDown()
	row, _ := cursorAt(e, 8, 4)
	if row != 1 {
		t.Fatalf("after moving down the cursor is on row %d, want the second row", row)
	}
	line, _ := e.Cursor()
	if line != 0 {
		t.Fatalf("cursor moved to logical line %d, want it still inside the one paragraph", line)
	}
}

func TestEditorVerticalMovementKeepsItsColumnThroughShortLines(t *testing.T) {
	// Travelling down through a short line and out the other side has to come back to
	// the column it went in at, or the cursor drifts left and stays there.
	e := NewEditor()
	e.Insert("aaaaaaaa\nbb\ncccccccc")
	e.MoveLineStart()
	e.line, e.col = 0, 6
	cursorAt(e, 20, 5)

	e.MoveDown()
	if _, col := e.Cursor(); col != 2 {
		t.Fatalf("on the short line the cursor is at %d, want its end", col)
	}
	e.MoveDown()
	if _, col := e.Cursor(); col != 6 {
		t.Fatalf("back on a long line the cursor is at %d, want the column it started from", col)
	}
}

func TestEditorVerticalMovementStopsAtTheEnds(t *testing.T) {
	e := NewEditor()
	e.Insert("one\ntwo")
	cursorAt(e, 20, 5)
	e.MoveUp()
	e.MoveUp()
	if line, _ := e.Cursor(); line != 0 {
		t.Fatalf("cursor at line %d, want the first", line)
	}
	e.MoveDown()
	e.MoveDown()
	e.MoveDown()
	if line, _ := e.Cursor(); line != 1 {
		t.Fatalf("cursor at line %d, want the last", line)
	}
}

func TestEditorMeasuresTheWidthAndItsCap(t *testing.T) {
	e := NewEditor()
	e.Insert("one two three four five six seven")
	wide := e.Measure(40)
	narrow := e.Measure(10)
	if narrow <= wide {
		t.Fatalf("height at 10 = %d, at 40 = %d, want narrower to be taller", narrow, wide)
	}
	e.MaxRows = 2
	if got := e.Measure(10); got != 2 {
		t.Fatalf("height with a cap of 2 = %d", got)
	}
}

func TestEditorScrollsToKeepTheCursorVisible(t *testing.T) {
	// A field that jumped to the end would lose the line the user is typing on.
	e := NewEditor()
	e.Insert("l1\nl2\nl3\nl4\nl5\nl6")
	row, _ := cursorAt(e, 20, 3)
	if row < 0 || row > 2 {
		t.Fatalf("cursor is on visible row %d of a 3-row field", row)
	}
	e.line, e.col = 0, 0
	row, _ = cursorAt(e, 20, 3)
	if row != 0 {
		t.Fatalf("after moving to the top the cursor is on row %d", row)
	}
}

func TestEditorPlaceholderIsNotText(t *testing.T) {
	e := NewEditor()
	e.Placeholder = "Ask anything"
	rows := paint(20, 1, func(v grid.View) { e.Draw(v) })
	if !strings.Contains(rows[0], "Ask anything") {
		t.Fatalf("row = %q, want the placeholder", rows[0])
	}
	if !e.Empty() || e.Text() != "" {
		t.Fatalf("text = %q, want the placeholder to be no part of it", e.Text())
	}
	typeText(e, "hi")
	rows = paint(20, 1, func(v grid.View) { e.Draw(v) })
	if strings.Contains(rows[0], "Ask") {
		t.Fatalf("row = %q, want the placeholder gone once there is text", rows[0])
	}
}

func TestEditorLeavesEnterToItsContainer(t *testing.T) {
	// Whether Enter sends or breaks the line is the container's decision. An editor
	// that swallowed it would take that decision away from every container.
	e := NewEditor()
	if e.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the editor consumed Enter")
	}
	if !e.Empty() {
		t.Fatalf("text = %q, want Enter to have done nothing", e.Text())
	}
	// Alt+Enter is the editor's own way to break a line, which leaves plain Enter free.
	if !e.Handle(input.Key{Code: input.Enter, Mods: input.Alt}) {
		t.Fatal("the editor ignored its newline binding")
	}
	if got := e.Text(); got != "\n" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorLeavesChordsItHasNoUseForAlone(t *testing.T) {
	e := NewEditor()
	for _, ev := range []input.Event{
		ctrlKey('g'),
		ctrlKey('c'),
		altKey('x'),
		input.Key{Code: input.F5},
		input.Key{Code: input.Character, Rune: 'a', Transition: input.Release},
	} {
		if e.Handle(ev) {
			t.Fatalf("the editor consumed %+v", ev)
		}
	}
	if !e.Empty() {
		t.Fatalf("text = %q, want nothing typed", e.Text())
	}
}

func TestEditorAcceptsShiftedCharacters(t *testing.T) {
	e := NewEditor()
	e.Handle(input.Key{Code: input.Character, Rune: 'A', Mods: input.Shift})
	if got := e.Text(); got != "A" {
		t.Fatalf("text = %q", got)
	}
}

func TestEditorPrefersTheTextTheTerminalReported(t *testing.T) {
	// The key's own code is the unshifted key on the physical keyboard. On a layout
	// where the key beside "1" produces "@", inserting the code would type "2", so
	// what the terminal says the key produced has to win.
	e := NewEditor()
	e.Handle(input.Key{Code: input.Character, Rune: '2', Text: "@"})
	if got := e.Text(); got != "@" {
		t.Fatalf("text = %q, want what the terminal reported", got)
	}
	e.Clear()
	e.Handle(input.Key{Code: input.Character, Text: "中文"})
	if got := e.Text(); got != "中文" {
		t.Fatalf("text = %q, want the reported text", got)
	}
}

func TestTheZeroEditorIsUsable(t *testing.T) {
	var e Editor
	if !e.Empty() {
		t.Fatal("the zero editor is not empty")
	}
	e.Insert("x")
	if got := e.Text(); got != "x" {
		t.Fatalf("text = %q", got)
	}
	// None of this may panic on a field nobody configured.
	e.MoveUp()
	e.MoveDown()
	e.DeleteBack()
	e.Handle(input.Key{Code: input.Left})
	paint(10, 2, func(v grid.View) { e.Draw(v) })
}

func TestEditorTextAndDrawnRowsAgree(t *testing.T) {
	// The invariant the cursor rests on: what the layout says is what is drawn.
	e := NewEditor()
	e.Insert("alpha beta gamma delta")
	const width = 12
	rows := paint(width, e.Measure(width), func(v grid.View) { e.Draw(v) })
	joined := strings.Join(rows, "")
	for _, word := range []string{"alpha", "beta", "gamma", "delta"} {
		if !strings.Contains(joined, word) {
			t.Fatalf("drawn rows %v lost %q", rows, word)
		}
	}
}

func TestAFreshScrollShowsTheStart(t *testing.T) {
	// Which is what a list of items wants. Following is asked for, not assumed.
	var s Scroll
	s.Layout(10, 5)
	if s.AtBottom() || s.Offset() != 0 {
		t.Fatalf("offset = %d, following = %v, want the start", s.Offset(), s.AtBottom())
	}
}

func TestAFollowingScrollStaysAtTheEndAsContentArrives(t *testing.T) {
	var s Scroll
	s.Layout(10, 5)
	s.ToBottom()
	if got := s.Offset(); got != 5 {
		t.Fatalf("offset = %d, want the last five rows shown", got)
	}
	s.Layout(20, 5)
	if got := s.Offset(); got != 15 {
		t.Fatalf("offset = %d, want to still be showing the end", got)
	}
}

func TestScrollingUpKeepsThePlaceAsContentArrives(t *testing.T) {
	var s Scroll
	s.Layout(100, 10)
	s.ToBottom()
	s.By(-20)
	before := s.Offset()
	// Ten more rows arrive while the reader is looking at something further up.
	s.Layout(110, 10)
	if got := s.Offset(); got != before {
		t.Fatalf("offset moved from %d to %d as content arrived", before, got)
	}
	if s.AtBottom() {
		t.Fatal("scrolled up but still claims to be following the end")
	}
}

func TestScrollClampsToTheContent(t *testing.T) {
	var s Scroll
	s.Layout(10, 5)
	s.ToBottom()
	s.By(-1000)
	if got := s.Offset(); got != 0 {
		t.Fatalf("offset = %d, want the start", got)
	}
	s.By(1000)
	if !s.AtBottom() || s.Offset() != 5 {
		t.Fatalf("offset = %d, want the end", s.Offset())
	}
	// Content that shrank under a scrolled window must not leave it out of bounds.
	s.By(-3)
	s.Layout(6, 5)
	if got := s.Offset(); got > 1 {
		t.Fatalf("offset = %d, want it clamped to the smaller content", got)
	}
}

func TestScrollEverythingFitsMeansNoOffset(t *testing.T) {
	var s Scroll
	s.Layout(3, 10)
	if got := s.Offset(); got != 0 {
		t.Fatalf("offset = %d, want nothing hidden", got)
	}
	s.By(-5)
	if got := s.Offset(); got != 0 {
		t.Fatalf("offset = %d after scrolling content that fits", got)
	}
}

func TestScrollPagesKeepOneRowOfOverlap(t *testing.T) {
	var s Scroll
	s.Layout(100, 10)
	s.ToTop()
	s.Pages(1)
	// Nine rows, not ten: the reader needs one row they recognise on the other side
	// of the jump.
	if got := s.Offset(); got != 9 {
		t.Fatalf("offset after a page = %d, want 9", got)
	}
}

func TestScrollHandlesKeysAndTheWheel(t *testing.T) {
	keys := DefaultScrollKeys()
	var s Scroll
	s.Layout(100, 10)
	s.ToTop()

	if !s.Handle(key(input.Down), keys) || s.Offset() != 1 {
		t.Fatalf("offset after one down = %d", s.Offset())
	}
	if !s.Handle(wheel(input.WheelDown), keys) || s.Offset() != 1+wheelRows {
		t.Fatalf("offset after a wheel notch = %d", s.Offset())
	}
	if !s.Handle(key(input.End), keys) || !s.AtBottom() {
		t.Fatal("End did not go to the end")
	}
	if !s.Handle(key(input.Home), keys) || s.Offset() != 0 {
		t.Fatal("Home did not go to the start")
	}
	// An event it has no use for carries on to whoever else might want it.
	if s.Handle(key(input.Enter), keys) {
		t.Fatal("the scroll swallowed a key it does nothing with")
	}
	if s.Handle(input.Mouse{Action: input.MouseDown}, keys) {
		t.Fatal("the scroll swallowed a click")
	}
}

func TestScrollRowsDrawsOnlyTheVisibleSlice(t *testing.T) {
	var s Scroll
	s.ToBottom()
	rows := paint(4, 3, func(v grid.View) {
		s.Rows(v, 10, func(row grid.View, index int) {
			row.Text(0, 0, strconv.Itoa(index), grid.Style{})
		})
	})
	// The window follows the end of the content, so the last three rows are shown.
	equalRows(t, rows, []string{"7...", "8...", "9..."})
}

func TestScrollRowsStopsAtTheEndOfShortContent(t *testing.T) {
	var s Scroll
	rows := paint(4, 4, func(v grid.View) {
		s.Rows(v, 2, func(row grid.View, index int) {
			row.Text(0, 0, strconv.Itoa(index), grid.Style{})
		})
	})
	equalRows(t, rows, []string{"0...", "1...", "....", "...."})
}

// items builds a list of numbered strings.

func TestListSelectionMoves(t *testing.T) {
	l := newList(5)
	if got := l.Selected(); got != 0 {
		t.Fatalf("selection starts at %d, want the first item", got)
	}
	l.Handle(key(input.Down))
	l.Handle(key(input.Down))
	if got := l.Selected(); got != 2 {
		t.Fatalf("selection = %d, want 2", got)
	}
	item, ok := l.Current()
	if !ok || item != "item2" {
		t.Fatalf("current = %q, %v", item, ok)
	}
	l.Handle(key(input.End))
	if got := l.Selected(); got != 4 {
		t.Fatalf("selection after End = %d, want the last item", got)
	}
	// Without wrapping the selection stops at the end, because in a long list
	// wrapping loses the user's place.
	l.Handle(key(input.Down))
	if got := l.Selected(); got != 4 {
		t.Fatalf("selection = %d, want it to have stayed at the end", got)
	}
}

func TestListWrapsOnlyWhenAskedTo(t *testing.T) {
	l := newList(3)
	l.Wrap = true
	l.Handle(key(input.Up))
	if got := l.Selected(); got != 2 {
		t.Fatalf("selection = %d, want it wrapped to the last item", got)
	}
	l.Handle(key(input.Down))
	if got := l.Selected(); got != 0 {
		t.Fatalf("selection = %d, want it wrapped to the first", got)
	}
}

func TestListScrollsToKeepTheSelectionVisible(t *testing.T) {
	// A selection the user cannot see is one they will act on by mistake.
	l := newList(20)
	rows := paint(10, 4, func(v grid.View) { l.Draw(v) })
	if !strings.Contains(rows[0], ">item0") {
		t.Fatalf("first frame = %v, want the selection at the top", rows)
	}
	for range 6 {
		l.Handle(key(input.Down))
	}
	rows = paint(10, 4, func(v grid.View) { l.Draw(v) })
	if !strings.Contains(rows[3], ">item6") {
		t.Fatalf("frame = %v, want the selection scrolled into the last row", rows)
	}
	l.Handle(key(input.Home))
	rows = paint(10, 4, func(v grid.View) { l.Draw(v) })
	if !strings.Contains(rows[0], ">item0") {
		t.Fatalf("frame = %v, want the view back at the top", rows)
	}
}

func TestListPageMovesByAWindow(t *testing.T) {
	l := newList(50)
	paint(10, 8, func(v grid.View) { l.Draw(v) })
	l.Handle(key(input.PageDown))
	if got := l.Selected(); got != 7 {
		t.Fatalf("selection after a page = %d, want a window's worth less one", got)
	}
}

func TestListKeepsItsPlaceWhenTheItemsAreRefreshed(t *testing.T) {
	// A list refreshed while the user is reading it must not jump.
	l := newList(10)
	l.Select(5)
	l.SetItems(items(10))
	if got := l.Selected(); got != 5 {
		t.Fatalf("selection = %d, want it kept", got)
	}
	// And when the list shrank past it, the selection lands on what is there.
	l.SetItems(items(3))
	if got := l.Selected(); got != 2 {
		t.Fatalf("selection = %d, want the last item of the shorter list", got)
	}
}

func TestListWithNothingInIt(t *testing.T) {
	l := &List[string]{Keys: DefaultListKeys()}
	if got := l.Selected(); got != -1 {
		t.Fatalf("selection = %d, want none", got)
	}
	if _, ok := l.Current(); ok {
		t.Fatal("an empty list handed out an item")
	}
	// None of this may panic.
	l.Handle(key(input.Down))
	l.Handle(key(input.End))
	paint(10, 3, func(v grid.View) { l.Draw(v) })
}

func TestListIgnoresKeysItHasNoUseFor(t *testing.T) {
	l := newList(3)
	if l.Handle(key(input.Enter)) {
		t.Fatal("the list swallowed Enter, which is its container's to interpret")
	}
}

func TestAZeroListAnswersTheKeysItDocuments(t *testing.T) {
	// A list is a struct a caller fills in, so its zero value has to work. One that
	// quietly ignored the arrow keys would look finished and not be — and nothing would
	// say so, because an unconsumed key simply carries on to whatever is around it.
	l := List[string]{Items: []string{"a", "b", "c"}}
	if !l.Handle(input.Key{Code: input.Down}) {
		t.Fatal("a zero list ignored the down key")
	}
	if l.Selected() != 1 {
		t.Fatalf("selected %d, want the second item", l.Selected())
	}
	if !l.Handle(input.Key{Code: input.End}) || l.Selected() != 2 {
		t.Fatalf("end went to %d", l.Selected())
	}
}

func TestAZeroEditorAnswersTheKeysItDocuments(t *testing.T) {
	// Same for the editor, whose ensure is already the seam that makes its zero value
	// usable: it took text but answered no navigation, which is the worse kind of
	// broken because it looks like it works.
	var e Editor
	e.Insert("abc")
	if !e.Handle(input.Key{Code: input.Left}) {
		t.Fatal("a zero editor ignored the left key")
	}
	if _, col := e.Cursor(); col != 2 {
		t.Fatalf("cursor at %d, want it to have moved left", col)
	}
}

func TestPointerTracksWhereItIs(t *testing.T) {
	var p Pointer
	if _, inside := p.Position(); inside {
		// A pointer that has never been reported is nowhere, not at the origin.
		t.Fatal("a fresh pointer claims to be somewhere")
	}
	if !p.Handle(press(3, 4, input.MouseMove, input.ButtonNone)) {
		t.Fatal("a mouse event was not taken")
	}
	at, inside := p.Position()
	if !inside || at != image.Pt(3, 4) {
		t.Fatalf("position = %v, %v", at, inside)
	}
	if p.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the pointer consumed a keystroke")
	}
}

func TestPointerHover(t *testing.T) {
	var p Pointer
	box := grid.Rect(2, 2, 4, 2)
	p.Handle(press(3, 3, input.MouseMove, input.ButtonNone))
	if !p.Over(box) {
		t.Fatal("the pointer is inside the box but does not say so")
	}
	p.Handle(press(9, 9, input.MouseMove, input.ButtonNone))
	if p.Over(box) {
		t.Fatal("the pointer left the box and still says it is over it")
	}
}

func TestAClickCommitsOnReleaseOverTheTargetThatTookThePress(t *testing.T) {
	// A control that fired on the way down fires when the user was aiming at it and
	// changed their mind.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)

	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the click fired on the way down")
	}
	if !p.Pressing(box) {
		t.Fatal("the control does not know it is being pushed")
	}
	p.Handle(press(1, 0, input.MouseUp, input.ButtonLeft))
	if !p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the click never fired")
	}
}

func TestAPressDraggedAwayAndBackIsStillHeld(t *testing.T) {
	// It follows the press, not the pointer: the press was never released.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)

	p.Handle(press(9, 9, input.MouseDrag, input.ButtonLeft))
	if !p.Pressing(box) {
		t.Fatal("dragging away released a press that was still held")
	}
	p.Handle(press(1, 0, input.MouseDrag, input.ButtonLeft))
	p.Handle(press(1, 0, input.MouseUp, input.ButtonLeft))
	if !p.Clicked(box, input.ButtonLeft) {
		t.Fatal("coming back and releasing did not click")
	}
}

func TestAReleaseSomewhereElseIsNotAClick(t *testing.T) {
	// Which is how a user takes back a press they did not mean.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)
	p.Handle(press(9, 9, input.MouseUp, input.ButtonLeft))
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("releasing away from the target still clicked it")
	}
	if p.Clicked(grid.Rect(8, 9, 4, 1), input.ButtonLeft) {
		t.Fatal("releasing over something else clicked that instead")
	}
}

func TestAPressBelongsToOneTarget(t *testing.T) {
	// Two overlapping regions must not both answer the same press.
	var p Pointer
	outer := grid.Rect(0, 0, 10, 4)
	inner := grid.Rect(1, 1, 4, 1)
	p.Handle(press(2, 1, input.MouseDown, input.ButtonLeft))
	p.Claim(inner)
	p.Claim(outer)
	p.Handle(press(2, 1, input.MouseUp, input.ButtonLeft))

	if p.Clicked(outer, input.ButtonLeft) {
		t.Fatal("the region that did not take the press answered the click")
	}
	if !p.Clicked(inner, input.ButtonLeft) {
		t.Fatal("the region that took the press did not answer the click")
	}
}

func TestAClickIsAnsweredOnce(t *testing.T) {
	// A widget asking twice in one frame, or two widgets asking in turn, must not both
	// act on the same click.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)
	p.Handle(press(1, 0, input.MouseUp, input.ButtonLeft))

	if !p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the click never fired")
	}
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("the same click fired twice")
	}
}

func TestAClickIsTheButtonThatWasPressed(t *testing.T) {
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonRight))
	p.Claim(box)
	p.Handle(press(1, 0, input.MouseUp, input.ButtonRight))
	if p.Clicked(box, input.ButtonLeft) {
		t.Fatal("a right press answered a left click")
	}
	if !p.Clicked(box, input.ButtonRight) {
		t.Fatal("the right click never fired")
	}
}

func TestLeavingTheInterfaceEndsHoverAndAnyPress(t *testing.T) {
	// A hover left highlighted under an unfocused window looks like the interface is
	// still live.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	p.Claim(box)

	p.Left()
	if p.Over(box) {
		t.Fatal("still hovering after the pointer left")
	}
	if p.Pressing(box) {
		t.Fatal("still holding a press after the pointer left")
	}
}

func TestAnUnclaimedPressPushesWhateverIsUnderIt(t *testing.T) {
	// Nothing has been drawn since the press, so the first frame after it is where a
	// control finds out it was pushed.
	var p Pointer
	box := grid.Rect(0, 0, 4, 1)
	p.Handle(press(1, 0, input.MouseDown, input.ButtonLeft))
	if !p.Pressing(box) {
		t.Fatal("a control under an unclaimed press does not draw as pushed")
	}
	if p.Pressing(grid.Rect(8, 8, 2, 1)) {
		t.Fatal("a control nowhere near the press draws as pushed")
	}
}
