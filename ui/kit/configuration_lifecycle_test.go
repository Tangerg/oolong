package kit_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestAppearanceChangesRespectTheSceneTransaction(t *testing.T) {
	panel := kit.NewPanel(kit.PanelConfig{})
	dialog := kit.NewDialog(kit.DialogConfig{})
	composer := kit.NewComposer(kit.ComposerConfig{})
	form := kit.NewForm(kit.FormConfig{})
	settings := kit.NewSettings(kit.SettingsConfig[int]{Items: []int{1}})
	slider := kit.NewSlider(kit.SliderConfig{Behavior: base.SliderConfig{Minimum: 0, Maximum: 10}})
	tree := kit.NewTree(kit.TreeConfig[int]{})
	transcript, err := kit.NewTranscript(kit.TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		element scene.Element
		change  func()
		state   func() any
	}{
		{"panel box", panel.Element(), func() { panel.SetBox(kit.Box{Title: "changed"}) }, func() any { return panel.Box() }},
		{"dialog placement", dialog.Element(), func() { dialog.SetWhere(layout.Placement{Anchor: layout.BottomRight}) }, func() any { return dialog.Where() }},
		{"composer prompt", composer.Element(), func() { composer.SetPrompt("ask") }, func() any { return composer.Prompt() }},
		{"composer hints", composer.Element(), func() { composer.SetHints([]keymap.Action{base.Yank}) }, func() any { return composer.Hints() }},
		{"form glyphs", form.Element(), func() { form.SetGlyphs(kit.ASCII()) }, func() any { return form.Glyphs() }},
		{"settings width", settings.Element(), func() { settings.SetValueWidth(4) }, func() any { return settings.ValueWidth() }},
		{"slider glyphs", slider.Element(), func() { slider.SetGlyphs(kit.ASCII()) }, func() any { return slider.Glyphs() }},
		{"tree indent", tree.Element(), func() { tree.SetIndent(4) }, func() any { return tree.Indent() }},
		{"transcript glyphs", transcript.Element(), func() { transcript.SetGlyphs(kit.ASCII()) }, func() any { return transcript.Glyphs() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := scene.NewRoot(scene.RootConfig{Element: tc.element})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			before, revision := tc.state(), root.Progress().ModelRevision
			tc.element.ReadOnly(func() { rejectAppearanceChange(t, tc.change) })
			if !reflect.DeepEqual(tc.state(), before) || root.Progress().ModelRevision != revision {
				t.Fatal("read-only rejection changed the appearance or revision")
			}
			frame, err := root.Prepare(grid.NewSurface(12, 4).View())
			if err != nil {
				t.Fatal(err)
			}
			rejectAppearanceChange(t, tc.change)
			frame.Abort()
			if !reflect.DeepEqual(tc.state(), before) || root.Progress().ModelRevision != revision {
				t.Fatal("pending-frame rejection changed the appearance or revision")
			}
			changed := root.Changes()
			tc.change()
			if reflect.DeepEqual(tc.state(), before) || root.Progress().ModelRevision <= revision {
				t.Fatal("accepted appearance change was not published")
			}
			select {
			case <-changed:
			default:
				t.Fatal("accepted appearance change did not wake the scene")
			}
		})
	}
}

func rejectAppearanceChange(t *testing.T, change func()) {
	t.Helper()
	defer func() {
		failure, ok := recover().(error)
		if !ok || !errors.Is(failure, scene.ErrBusy) {
			t.Errorf("rejected change = %v, want ErrBusy", failure)
		}
	}()
	change()
}

func TestAppearanceSnapshotsDoNotExposeOwnedSlices(t *testing.T) {
	glyphs := kit.Unicode()
	panel := kit.NewPanel(kit.PanelConfig{Box: kit.Box{Glyphs: glyphs}})
	form := kit.NewForm(kit.FormConfig{Glyphs: glyphs})
	before := form.Glyphs()
	glyphs.Spinner[0] = "caller changed"
	glyphs.BarSteps[0] = kit.CellGlyph{}
	returned := form.Glyphs()
	returned.Spinner[0] = "getter changed"
	returned.Sparkline[0] = kit.CellGlyph{}
	box := panel.Box()
	box.Glyphs.Spinner[0] = "box getter changed"
	if !reflect.DeepEqual(form.Glyphs(), before) || !reflect.DeepEqual(panel.Box().Glyphs, before) {
		t.Fatal("caller mutated an owned glyph snapshot")
	}
	hints := []keymap.Action{base.Yank}
	composer := kit.NewComposer(kit.ComposerConfig{Hints: hints})
	hints[0] = base.Cancel
	composer.Hints()[0] = base.Cancel
	if got := composer.Hints(); len(got) != 1 || got[0] != base.Yank {
		t.Fatalf("caller mutated owned hints: %v", got)
	}
}
