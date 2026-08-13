package headless_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/text"
)

// FuzzEditorStateTransitions exercises the public editor as a state machine. The
// individual operations already have example-shaped tests; this keeps their shared
// storage contract true across sequences no one would think to enumerate by hand.
func FuzzEditorStateTransitions(f *testing.F) {
	for _, seed := range [][]byte{
		{},
		[]byte("plain text"),
		[]byte("first\r\nsecond\x00\x1b"),
		[]byte("中文\ttext"),
		{0xff, 0xfe, 0x00, '\n'},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		const limit = 512
		if len(data) > limit {
			data = data[:limit]
		}

		clipboard := &clip{}
		editor := headless.Editor{Clipboard: clipboard}
		editor.SetText(string(data))
		for i, b := range data {
			before := editor.Revision()
			fragment := string(data[i:min(i+3, len(data))])
			switch b % 36 {
			case 0:
				editor.Insert(fragment)
			case 1:
				editor.InsertRune(rune(b))
			case 2:
				editor.Replace(int(b)%5-2, int(b)%9, fragment)
			case 3:
				editor.DeleteBack()
			case 4:
				editor.DeleteForward()
			case 5:
				editor.DeleteWordBack()
			case 6:
				editor.SetCursor(int(b)%7-3, int(b)%13-4)
			case 7:
				editor.MoveLeft()
			case 8:
				editor.MoveRight()
			case 9:
				editor.MoveUp()
			case 10:
				editor.MoveDown()
			case 11:
				editor.Anchor()
			case 12:
				editor.SelectAll()
			case 13:
				editor.InsertElement(headless.ElementKind(b), fragment)
			case 14:
				elements := editor.Elements()
				if len(elements) > 0 {
					editor.RemoveElement(elements[int(b)%len(elements)].ID)
				}
			case 15:
				editor.Undo()
			case 16:
				editor.Redo()
			case 17:
				editor.SetSingleLine(b&1 != 0)
				masks := [...]string{"", "*", "••"}
				editor.SetMask(masks[int(b)%len(masks)])
			case 18:
				editor.MoveWordLeft()
			case 19:
				editor.MoveWordRight()
			case 20:
				editor.MoveLineStart()
			case 21:
				editor.MoveLineEnd()
			case 22:
				editor.Newline()
			case 23:
				editor.SetText(fragment)
			case 24:
				editor.Clear()
			case 25:
				editor.KillToStart()
			case 26:
				editor.KillToEnd()
			case 27:
				editor.Yank()
			case 28:
				editor.YankPop()
			case 29:
				editor.DeleteSelection()
			case 30:
				editor.Cut()
			case 31:
				editor.Handle(input.Paste{Text: fragment})
			case 32:
				editor.SelectNone()
			case 33:
				editor.Focus(b&1 != 0)
			case 34:
				editor.Copy()
			case 35:
				editor.DeleteForward()
			}
			if editor.Revision() < before {
				t.Fatalf("revision moved backwards from %d to %d", before, editor.Revision())
			}
			assertEditorStorage(t, &editor)
		}

		width := 1
		if len(data) > 0 {
			width += int(data[0] % 12)
		}
		paintWidget(width, max(editor.Measure(width), 1), &editor)
	})
}

func assertEditorStorage(t *testing.T, editor *headless.Editor) {
	t.Helper()
	document := editor.Text()
	if !utf8.ValidString(document) {
		t.Fatalf("editor retained invalid UTF-8: %q", document)
	}
	for _, r := range document {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			t.Fatalf("editor retained control character %U in %q", r, document)
		}
	}
	lines := strings.Split(document, "\n")
	if (editor.SingleLine() || editor.Mask() != "") && len(lines) != 1 {
		t.Fatalf("one-line editor retained %d logical lines in %q", len(lines), document)
	}
	line, col := editor.Cursor()
	if line < 0 || line >= len(lines) || col < 0 || col > len(lines[line]) {
		t.Fatalf("cursor (%d,%d) is outside %q", line, col, document)
	}
	if !clusterBoundary(lines[line], col) {
		t.Fatalf("cursor (%d,%d) splits a grapheme in %q", line, col, document)
	}
	if start, end, ok := editor.Selection(); ok {
		if start.Line < 0 || start.Line >= len(lines) || end.Line < 0 || end.Line >= len(lines) ||
			!clusterBoundary(lines[start.Line], start.Col) || !clusterBoundary(lines[end.Line], end.Col) {
			t.Fatalf("selection [%+v,%+v) is outside cluster boundaries in %q", start, end, document)
		}
	}
	elements := editor.Elements()
	for _, element := range elements {
		body := element.Text(editor)
		if element.Line < 0 || element.Line >= len(lines) || element.Start < 0 ||
			element.End > len(lines[element.Line]) || element.Start >= element.End ||
			body == "" || strings.Contains(body, "\n") {
			t.Fatalf("invalid element %+v over %q", element, document)
		}
		if line == element.Line && col > element.Start && col < element.End {
			t.Fatalf("cursor (%d,%d) is inside atomic element %+v", line, col, element)
		}
	}
	_ = editor.Selected() // Selection endpoints must remain safe after every mutation.
}

func clusterBoundary(line string, at int) bool {
	if at == 0 || at == len(line) {
		return true
	}
	return text.NextCluster(line, text.PrevCluster(line, at)) == at
}
