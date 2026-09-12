package headless_test

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

func TestPresentedClickFocusesTheSameChildAfterKeyedReorder(t *testing.T) {
	a, b := &field{name: "a", takes: true}, &field{name: "b", takes: true}
	first := headless.Item{Key: "a", Size: layout.Fixed(1), Of: a}
	second := headless.Item{Key: "b", Size: layout.Fixed(1), Of: b}
	c := headless.NewContainer(layout.Down, first, second)
	drawn(c, 2)
	c.Set(second, first)
	c.FocusIndex(0)
	if !c.Handle(pressAt(0, 0)) || c.Focused() != headless.Widget(a) {
		t.Fatal("visible a and keyboard focus diverged after reorder")
	}
	replacement := &field{name: "replacement", takes: true}
	c.Set(second, headless.Item{Key: "a", Size: layout.Fixed(1), Of: replacement})
	if c.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDrag}) {
		t.Fatal("replacement inherited capture")
	}
	if c.Handle(pressAt(0, 0)) {
		t.Fatal("old pixels targeted a replacement that has not drawn")
	}
	drawn(c, 2)
	if !c.Handle(pressAt(0, 1)) || c.Focused() != headless.Widget(replacement) {
		t.Fatal("new frame did not activate replacement")
	}
}

func TestListNavigationDoesNotOverrideLaterManualScroll(t *testing.T) {
	var list headless.List[int]
	list.SetItems(make([]int, 30))
	root := headless.NewRoot(&list)
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
	bound := language("en")
	selectField := headless.Select[language]{Same: headless.Equal[language], Value: headless.Bind(&bound)}
	selectField.SetOptions([]headless.Option[language]{{Label: "中文", Value: "zh"}, {Label: "English", Value: "en"}})
	if option, ok := selectField.Chosen(); !ok || option.Value != "en" {
		t.Fatal("label determined identity")
	}
	selectField.SetOptions(headless.Options(language("en"), language("zh")))
	if option, _ := selectField.Chosen(); option.Value != "en" {
		t.Fatal("defined string type lost identity")
	}
	values := &countedAccessor[[]string]{value: []string{"missing", "b", "b", "a"}}
	multi := headless.MultiSelect[string]{Same: headless.Equal[string], Value: values}
	multi.SetOptions(headless.Options("a", "b"))
	_ = multi.Ask()
	if !slices.Equal(multi.Taken(), []string{"a", "b"}) || values.writes != 0 {
		t.Fatal("reading canonical choices wrote Value")
	}
	multi.Sync()
	if values.writes != 1 || !slices.Equal(values.value, []string{"a", "b"}) {
		t.Fatal("Sync did not settle exactly once")
	}
	multi.Sync()
	if values.writes != 1 {
		t.Fatal("canonical Sync repeated the write")
	}
}

type intrinsicWidget struct{ width, height int }

func (*intrinsicWidget) Draw(headless.Frame)      {}
func (w *intrinsicWidget) HeightForWidth(int) int { return w.height }
func (w *intrinsicWidget) WidthForHeight(int) int { return w.width }

func TestMeasuredContainersKeepAxisUnitsDistinct(t *testing.T) {
	child := &intrinsicWidget{width: 7, height: 2}
	row := headless.NewContainer(layout.Across, headless.Item{Size: layout.Measured(0, 0), Of: child})
	if row.WidthForHeight(10) != 7 || row.HeightForWidth(20) != 2 {
		t.Fatal("horizontal container confused width and height")
	}
	column := headless.NewContainer(layout.Down, headless.Item{Size: layout.Measured(0, 0), Of: row})
	if column.HeightForWidth(20) != 2 {
		t.Fatal("nested horizontal container returned width as height")
	}
}

type alternateCompletion struct{}

func (alternateCompletion) Width(headless.Candidate) int { return 9 }
func (alternateCompletion) DrawRow(v grid.View, c headless.Candidate, selected bool, look headless.Look) {
	mark := "-"
	if selected {
		mark = "+"
	}
	v.Text(0, 0, mark+c.Text, look.Text)
}

type themedExternalField struct {
	observedField
	look headless.Look
}

func (f *themedExternalField) DrawWith(v headless.Frame, look headless.Look) {
	f.look = look
	v.Text(0, 0, "external", look.Accent)
}

func TestCompletionAndExternalFieldsCanReplaceTheirAppearance(t *testing.T) {
	completion := headless.Completion{Renderer: alternateCompletion{}}
	completion.Offer(headless.Token{}, []headless.Candidate{{Text: "one", Matched: []int{0}}, {Text: "two"}})
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
	form := headless.NewForm(external)
	form.Look = headless.Look{Accent: grid.Style{Attr: grid.Bold}}
	paintWidget(10, 1, form)
	if external.look != form.Look {
		t.Fatal("external field could not receive form theme")
	}
	selected := headless.Select[string]{Same: headless.Equal[string], Row: func(v grid.View, o headless.Option[string], _, _ bool, _ headless.Look) {
		v.Text(0, 0, "/"+o.Value, grid.Style{})
	}}
	selected.SetOptions(headless.Options("one"))
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
		token, ok := headless.TokenAt(tc.line, len(tc.line), headless.Trigger{Prefix: tc.prefix, AtStart: tc.atStart})
		if !ok || token.Start != len(tc.prefix) || token.Query != tc.query {
			t.Fatalf("TokenAt(%q)=%+v,%v", tc.line, token, ok)
		}
	}
}

func TestControlledFieldsCommitAmbiguousActionsAtTheirOwnerBoundary(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		t.Run(map[bool]string{false: "declined continuation", true: "deferred resolution"}[delayed], func(t *testing.T) {
			for _, kind := range []string{"text", "choice"} {
				t.Run(kind, func(t *testing.T) {
					var resolve func()
					keys := &keymap.Map{}
					if delayed {
						keys.Resolve = func(_ time.Duration, fn func()) func() { resolve = fn; return func() {} }
					}
					g := input.Chord{Code: input.Character, Rune: 'g'}
					value := &countedAccessor[string]{value: "abc"}
					var field headless.Field
					if kind == "text" {
						field = &headless.Text{Value: value, Keys: keys}
						keys.Bind(headless.DeleteBack, g)
						keys.Bind(headless.Undo, g, g)
					} else {
						value.value = "a"
						choice := &headless.Select[string]{Same: headless.Equal[string], Value: value, Keys: keys}
						choice.SetOptions(headless.Options("a", "b", "c"))
						field = choice
						keys.Bind(headless.SelectNext, g)
						keys.Bind(headless.SelectLast, g, g)
					}
					if !field.Handle(input.Key{Code: input.Character, Rune: 'g'}) {
						t.Fatal("prefix rejected")
					}
					if value.writes != 0 {
						t.Fatal("prefix ran early")
					}
					if delayed {
						resolve()
					} else if field.Handle(input.Key{Code: input.Esc}) {
						t.Fatal("unbound continuation was swallowed")
					}
					want := "ab"
					if kind == "choice" {
						want = "b"
					}
					if value.value != want || value.writes != 1 {
						t.Fatalf("accepted value=%q writes=%d, want %q once", value.value, value.writes, want)
					}
				})
			}
		})
	}
}

func TestRequiredPresentationOwnersRejectNilReceivers(t *testing.T) {
	for name, call := range map[string]func(){
		"Snapshot.Value":   func() { var s *headless.Snapshot[int]; s.Value() },
		"Snapshot.Stage":   func() { var s *headless.Snapshot[int]; s.Stage(headless.Frame{}, 1) },
		"List.Len":         func() { var l *headless.List[int]; l.Len() },
		"List.At":          func() { var l *headless.List[int]; l.At(0) },
		"List.At negative": func() { var l *headless.List[int]; l.At(-1) },
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
	candidates := []headless.Candidate{{Text: "one"}, {Text: "two"}, {Text: "three"}, {Text: "four"}, {Text: "five"}}
	var c headless.Completion
	c.Offer(headless.Token{}, candidates)
	root := headless.NewRoot(&c)
	surface := grid.NewSurface(9, 2)
	root.Draw(surface.View())
	c.Handle(input.Mouse{Action: input.WheelDown})
	root.Draw(surface.View())
	c.Offer(headless.Token{Query: "new"}, candidates)
	equalRows(t, paintWidget(9, 2, &c), []string{"one......", "two......"})
}
