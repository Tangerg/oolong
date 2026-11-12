package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func FuzzSharedEditorStateTransitions(f *testing.F) {
	f.Add("first\nsecond", []byte{0, 1, 128, 5, 134, 7, 136, 9})
	f.Add("a👩‍💻e\u0301中", []byte{2, 131, 4, 133, 6, 135, 8, 137})
	f.Fuzz(func(t *testing.T, initial string, operations []byte) {
		const limit = 256
		initial = initial[:min(len(initial), limit)]
		operations = operations[:min(len(operations), limit)]
		session := base.NewEditSession(base.EditSessionConfig{Initial: initial})
		views := [...]*base.Editor{
			base.NewEditor(base.EditorConfig{Session: session}),
			base.NewEditor(base.EditorConfig{Session: session}),
		}
		fragments := [...]string{"a", "\u0301", "👩‍💻", "中", "\n", "\t", "\u200d", "🇦"}
		for _, operation := range operations {
			view := views[operation>>7]
			fragment := fragments[int(operation>>4)%len(fragments)]
			switch operation % 16 {
			case 0:
				view.Insert(fragment)
			case 1:
				view.InsertRune([]rune(fragment)[0])
			case 2:
				view.DeleteBack()
			case 3:
				view.DeleteForward()
			case 4:
				view.SetCursor(int(operation)%5-1, int(operation)%17)
			case 5:
				view.Anchor()
				view.MoveRight()
			case 6:
				view.SelectAll()
			case 7:
				view.Undo()
			case 8:
				view.Redo()
			case 9:
				view.SetSingleLine(operation&16 != 0)
			case 10:
				mask := ""
				if operation&16 != 0 {
					mask = "*"
				}
				view.SetMask(mask)
			case 11:
				view.InsertElement(1, fragment)
			case 12:
				session.Reset(fragment)
			case 13:
				view.Replace(int(operation)%3, 0, int(operation)%7, fragment)
			case 14:
				view.ForgetHistory()
			case 15:
				view.SelectNone()
			}
			for _, current := range views {
				assertEditorStorage(t, current)
			}
		}
	})
}
