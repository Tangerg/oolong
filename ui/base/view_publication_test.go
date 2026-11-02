package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestViewOnlyChangesPublishTheirOwningScene(t *testing.T) {
	for _, name := range []string{"cursor", "selection", "scroll", "transcript selection", "sticky"} {
		t.Run(name, func(t *testing.T) {
			var element scene.Element
			var mutate func()
			var changed func() bool
			switch name {
			case "cursor", "selection", "scroll":
				editor := base.NewEditor(base.EditorConfig{Initial: "zero\none\ntwo\nthree\nfour\nfive"})
				editor.SetCursor(0, 0)
				element = editor.Element()
				switch name {
				case "cursor":
					mutate = editor.MoveRight
					changed = func() bool { line, column := editor.Cursor(); return line == 0 && column == 1 }
				case "selection":
					mutate = editor.SelectAll
					changed = func() bool { return editor.Selected() == editor.Text() }
				case "scroll":
					mutate = func() { editor.Scroll().By(1) }
					changed = func() bool { return editor.Scroll().Offset() == 1 }
				}
			default:
				model := transcriptModel(t, base.TranscriptConfig{})
				item := transcriptAdd(t, model, transcriptBody("text"))
				element = model.Element()
				if name == "sticky" {
					mutate = func() { model.Sticky().Add(item.ID()) }
					changed = func() bool { return model.Sticky().Len() == 1 }
				} else {
					mutate = func() {
						selected, ok := model.Layout().Hit(base.Point{})
						if !ok {
							t.Fatal("missing text point")
						}
						model.Selection().Set(selected)
					}
					changed = func() bool { return model.Selection().Active() }
				}
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			frame, err := root.Prepare(grid.NewSurface(12, 4).View())
			if err != nil {
				t.Fatal(err)
			}
			frame.Commit(1)
			root.Wrote(1)
			before, wake := root.Progress().ModelRevision, root.Changes()
			mutate()
			if !changed() || root.Progress().ModelRevision <= before {
				t.Fatal("completed view change did not advance the scene")
			}
			select {
			case <-wake:
			default:
				t.Fatal("completed view change did not wake the scene observer")
			}
		})
	}
}

func TestUnchangedViewOperationsDoNotPublish(t *testing.T) {
	editor := base.NewEditor(base.EditorConfig{Initial: "text", SingleLine: true})
	editor.SetCursor(0, 0)
	root, err := scene.NewRoot(scene.RootConfig{Element: editor.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	before := root.Progress().ModelRevision
	editor.SetCursor(0, 0)
	editor.MoveLeft()
	editor.SelectNone()
	if root.Progress().ModelRevision != before {
		t.Fatal("unchanged cursor and selection requested another frame")
	}

	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, transcriptBody("one\ntwo\nthree\nfour"))
	model.Scroll().ToTop()
	other := transcriptScene(t, model)
	transcriptCommit(t, other, 12, 2, 1)
	before = other.Progress().ModelRevision
	model.Scroll().By(0)
	model.Scroll().ToTop()
	model.Selection().Clear()
	model.Sticky().Add()
	model.Sticky().SetBlocks(nil)
	if other.Progress().ModelRevision != before {
		t.Fatal("unchanged reading state requested another frame")
	}
}

func TestEditorPublishesCompletedCommandsAndSharedEdits(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{Initial: "abc"})
	first := base.NewEditor(base.EditorConfig{Session: session})
	second := base.NewEditor(base.EditorConfig{Session: session})
	first.SetCursor(0, 0)
	second.SetCursor(0, 2)
	observe := func() {}
	group := scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Enabled: func() bool { observe(); return true }},
		Items:   []scene.Item{{Element: first.Element()}, {Element: second.Element()}},
	})
	root, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	before := session.Revision()
	observed := 0
	observe = func() {
		observed++
		_, column := second.Cursor()
		if session.Text() != "Xabc" || session.Revision() != before+1 || column != 3 {
			t.Error("scene observed text before all shared view anchors and revision committed")
		}
	}
	first.Insert("X")
	if observed == 0 {
		t.Fatal("edit did not notify the scene")
	}
	observe = func() {}
	first.SelectAll()
	observe = func() {
		if first.Selected() != "" {
			t.Error("navigation published before clearing the prior selection")
		}
	}
	first.Do(base.MoveLeft)
	observe = func() {}
}

func TestEditorNavigationPreservesTheCommittedScrollViewport(t *testing.T) {
	for _, action := range []string{"bottom", "page"} {
		t.Run(action, func(t *testing.T) {
			editor := base.NewEditor(base.EditorConfig{Initial: "0\n1\n2\n3\n4\n5\n6\n7\n8\n9"})
			editor.SetCursor(0, 0)
			root := newTestRoot(editor)
			t.Cleanup(root.root.Close)
			root.Draw(grid.NewSurface(12, 4).View())
			editor.SetCursor(0, 0)
			want := 6
			if action == "bottom" {
				editor.Scroll().ToBottom()
			} else {
				editor.Scroll().Pages(1)
				want = 3
			}
			if got := editor.Scroll().Offset(); got != want {
				t.Fatalf("%s offset after navigation = %d, want %d", action, got, want)
			}
		})
	}
}
