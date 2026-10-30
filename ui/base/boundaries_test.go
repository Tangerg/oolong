package base_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
)

func TestListNavigationDoesNotOverrideLaterManualScroll(t *testing.T) {
	var list base.List[int]
	list.SetItems(make([]int, 30))
	root := newTestRoot(&list)
	surface := grid.NewSurface(12, 5)
	root.Draw(surface.View())
	list.Scroll().By(3)
	root.Draw(surface.View())
	root.Draw(surface.View())
	if list.Scroll().Offset() != 3 {
		t.Fatal("redrawing replayed selection navigation")
	}
	list.Select(12)
	root.Draw(surface.View())
	if list.Scroll().Offset() != 8 {
		t.Fatal("selection change was not revealed")
	}
	list.Select(20)
	list.Scroll().ToTop()
	root.Draw(surface.View())
	if list.Scroll().Offset() != 0 {
		t.Fatal("a pending selection overrode later manual scrolling")
	}
	list.Select(25)
	fail := true
	list.Row = func(grid.View, int, int, bool) {
		if fail {
			panic("aborted frame")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("test frame did not abort")
			}
		}()
		root.Draw(surface.View())
	}()
	fail = false
	root.Draw(surface.View())
	if list.Scroll().Offset() != 21 {
		t.Fatal("aborted frame consumed navigation")
	}
	list.Scroll().By(-3)
	root.Draw(surface.View())
	if list.Scroll().Offset() != 18 {
		t.Fatal("committed navigation repeated")
	}
	list.Select(28)
	list.SetItems(nil)
	list.SetItems(make([]int, 30))
	root.Draw(surface.View())
	if list.Scroll().Offset() != 0 {
		t.Fatal("empty list retained stale navigation")
	}
}

func TestOptionIdentityAndGettersUseValuesWithoutWrites(t *testing.T) {
	type language string
	field := base.NewSelect(base.SelectConfig[language]{Same: base.Equal[language], Initial: "en", Options: []base.Option[language]{{Label: "中文", Value: "zh"}, {Label: "English", Value: "en"}}})
	if choice, ok := field.Chosen(); !ok || choice.Value != "en" {
		t.Fatal("label determined identity")
	}
	field.SetOptions(base.Options(language("en"), language("zh")))
	if choice, _ := field.Chosen(); choice.Value != "en" {
		t.Fatal("reordering changed value identity")
	}
	multi := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: base.Equal[string], Options: base.Options("a", "b"), Initial: []string{"missing", "b", "b", "a"}})
	revision := multi.Revision()
	_ = multi.Ask()
	if !slices.Equal(multi.Taken(), []string{"a", "b"}) || multi.Revision() != revision {
		t.Fatal("read changed canonical set")
	}
}

func TestMeasuredGroupsKeepAxisUnitsDistinct(t *testing.T) {
	child := scene.NewElement(scene.ElementConfig{Measure: func(axis layout.Axis, _ int) int {
		if axis == layout.Across {
			return 7
		}
		return 2
	}})
	row := scene.NewGroup(scene.GroupConfig{Axis: layout.Across, Items: []scene.Item{{Size: layout.Measured(0, 0), Element: child}}})
	if row.Element().Measure(layout.Across, 10) != 7 || row.Element().Measure(layout.Down, 20) != 2 {
		t.Fatal("horizontal group confused width and height")
	}
	column := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{{Size: layout.Measured(0, 0), Element: row.Element()}}})
	if column.Element().Measure(layout.Down, 20) != 2 {
		t.Fatal("nested horizontal group returned width as height")
	}
}

type alternateCompletion struct{}

func (alternateCompletion) Width(base.Candidate) int { return 9 }
func (alternateCompletion) DrawRow(v grid.View, c base.Candidate, selected bool, look base.Look) {
	mark := "-"
	if selected {
		mark = "+"
	}
	v.Text(0, 0, mark+c.Text, look.Text)
}

type themedExternalField struct {
	observedField
	look base.Look
}

func (f *themedExternalField) DrawWith(v scene.Frame, look base.Look) {
	f.look = look
	v.Text(0, 0, "external", look.Accent)
}

func TestCompletionAndExternalFieldsCanReplaceTheirAppearance(t *testing.T) {
	completion := base.Completion{Renderer: alternateCompletion{}}
	completion.Offer(base.Token{}, []base.Candidate{{Text: "one", Matched: []int{0}}, {Text: "two"}})
	if completion.Width() != 9 {
		t.Fatal("custom row width ignored")
	}
	equalRows(t, paintWidget(9, 1, &completion), []string{"+one....."})
	candidate, _ := completion.Current()
	candidate.Matched[0] = 2
	again, _ := completion.Current()
	if again.Matched[0] != 0 {
		t.Fatal("candidate snapshot exposed mutable storage")
	}
	external := &themedExternalField{}
	form := base.NewForm(base.FormConfig{Fields: []base.Field{external}})
	look := base.Look{Accent: grid.Style{Attr: grid.Bold}}
	styleFields(form, look)
	paintWidget(10, 1, form)
	if external.look != look {
		t.Fatal("external field could not receive form theme")
	}
	selected := base.Select[string]{Same: base.Equal[string], Row: func(v grid.View, o base.Option[string], _, _ bool, _ base.Look) {
		v.Text(0, 0, "/"+o.Value, grid.Style{})
	}}
	selected.SetOptions(base.Options("one"))
	equalRows(t, paintWidget(8, 1, &selected), []string{"/one...."})
}

func TestTokenLookupSkipsInvalidRepeatedTriggers(t *testing.T) {
	for _, tc := range []struct {
		line, prefix, query string
		atStart             bool
	}{
		{"/foo/bar", "/", "foo/bar", true},
		{"@dir/user@example.com", "@", "dir/user@example.com", false},
		{"@目录/user@example.com", "@", "目录/user@example.com", false},
	} {
		token, ok := base.TokenAt(tc.line, len(tc.line), base.Trigger{Prefix: tc.prefix, AtStart: tc.atStart})
		if !ok || token.Start != len(tc.prefix) || token.Query != tc.query {
			t.Fatalf("TokenAt(%q)=%+v,%v", tc.line, token, ok)
		}
	}
}

func TestFieldsCommitAmbiguousActionsAtTheirOwnerBoundary(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		for _, kind := range []string{"text", "choice"} {
			t.Run(fmt.Sprintf("%s/%t", kind, delayed), func(t *testing.T) {
				var resolve func()
				keys := &keymap.Map{}
				if delayed {
					keys.Resolve = func(_ time.Duration, fn func()) func() { resolve = fn; return func() {} }
				}
				g := input.Chord{Rune: 'g'}
				var field interface {
					Handle(event input.Event) bool
					Revision() uint64
				}
				var value func() string
				if kind == "text" {
					control := base.NewText(base.TextConfig{Initial: "abc", Keys: keys})
					field = control
					value = control.Text
					keys.Bind(base.DeleteBack, g)
					keys.Bind(base.Undo, g, g)
				} else {
					control := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Options: base.Options("a", "b", "c"), Keys: keys})
					field = control
					value = func() string { return chosenValue(control) }
					keys.Bind(base.SelectNext, g)
					keys.Bind(base.SelectLast, g, g)
				}
				before := field.Revision()
				if !field.Handle(input.Key{Rune: 'g'}) || field.Revision() != before {
					t.Fatal("ambiguous prefix changed owner early")
				}
				if delayed {
					resolve()
				} else if field.Handle(input.Key{Code: input.Esc}) {
					t.Fatal("unbound continuation swallowed")
				}
				want := "ab"
				if kind == "choice" {
					want = "b"
				}
				if value() != want || field.Revision() != before+1 {
					t.Fatalf("resolved=%q revision=%d, want %q once", value(), field.Revision(), want)
				}
			})
		}
	}
}

func TestRequiredPresentationOwnersRejectNilReceivers(t *testing.T) {
	for name, call := range map[string]func(){
		"Snapshot.Stage":   func() { var s *scene.Snapshot[int]; s.Stage(scene.Frame{}, 1) },
		"List.Len":         func() { var l *base.List[int]; l.Len() },
		"List.At":          func() { var l *base.List[int]; l.At(0) },
		"List.At negative": func() { var l *base.List[int]; l.At(-1) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("nil owner silently accepted operation")
				}
			}()
			call()
		})
	}
}

func TestNewCompletionOfferStartsAtItsFirstRowAfterManualScroll(t *testing.T) {
	candidates := []base.Candidate{{Text: "one"}, {Text: "two"}, {Text: "three"}, {Text: "four"}, {Text: "five"}}
	var c base.Completion
	c.Offer(base.Token{}, candidates)
	root := newTestRoot(&c)
	surface := grid.NewSurface(9, 2)
	root.Draw(surface.View())
	c.Handle(input.Mouse{Action: input.WheelDown})
	root.Draw(surface.View())
	c.Offer(base.Token{Query: "new"}, candidates)
	equalRows(t, paintWidget(9, 2, &c), []string{"one......", "two......"})
}
