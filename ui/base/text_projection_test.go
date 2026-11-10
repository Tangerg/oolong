package base_test

import (
	"image"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestButtonAndTabsDrawTheirMeasuredTextProjection(t *testing.T) {
	for _, component := range []struct {
		name string
		new  func(string) scene.Element
	}{
		{name: "button", new: func(label string) scene.Element {
			return base.NewButton(base.ButtonConfig{Label: label}).Element()
		}},
		{name: "tabs", new: func(label string) scene.Element {
			return base.NewTabs(base.TabsConfig{Items: []base.Tab{{Title: label}}}).Element()
		}},
	} {
		t.Run(component.name, func(t *testing.T) {
			for _, test := range []struct {
				name, source, shown string
				width               int
			}{
				{name: "tab", source: "a\tb", shown: "a       b", width: 9},
				{name: "separated variation selector", source: "❤\n\ufe0fX", shown: "❤X", width: 2},
				{name: "separated combining mark", source: "a\x00\u0301X", shown: "aX", width: 2},
				{name: "malformed byte", source: "a\xffX", shown: "aX", width: 2},
				{name: "wide cluster", source: "👩🏽‍💻X", shown: "👩🏽‍💻X", width: 3},
				{name: "leading combining mark", source: "\u0301X", shown: "X", width: 1},
			} {
				t.Run(test.name, func(t *testing.T) {
					element := component.new(test.source)
					if got := element.Measure(layout.Across, 1); got != test.width {
						t.Fatalf("measured width = %d, want %d", got, test.width)
					}
					root, err := scene.NewRoot(scene.RootConfig{Element: element})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(root.Close)
					for _, width := range []int{test.width, max(test.width-1, 1)} {
						for _, offset := range []int{0, 3} {
							actual := grid.NewSurface(width+offset, 1)
							expected := grid.NewSurface(width+offset, 1)
							if offset > 0 {
								actual.View().Text(offset-1, 0, "Z", grid.Style{})
								expected.View().Text(offset-1, 0, "Z", grid.Style{})
							}
							area := grid.Area(offset, 0, width, 1)
							candidate, err := root.Prepare(actual.View().Sub(area))
							if err != nil {
								t.Fatal(err)
							}
							candidate.Abort()
							expected.View().Sub(area).Text(0, 0, test.shown, grid.Style{})
							if got, want := actual.Rows(), expected.Rows(); !reflect.DeepEqual(got, want) {
								t.Fatalf("width %d, offset %d: drawn %q, want %q", width, offset, got, want)
							}
						}
					}
				})
			}
		})
	}
}

func TestChoiceMarksShareTheirReservedColumns(t *testing.T) {
	for _, test := range []struct {
		name        string
		look        base.Look
		first, last string
	}{
		{name: "tab in taken", look: base.Look{Taken: "a\tb", Free: "-"}, first: "a       b one", last: "-         two"},
		{name: "tab in free", look: base.Look{Taken: "+", Free: "a\tb"}, first: "+         one", last: "a       b two"},
		{name: "filtered mark", look: base.Look{Taken: "❤\n\ufe0fX", Free: "-"}, first: "❤X one", last: "-  two"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scenes := scenetest.New()
			t.Cleanup(scenes.Close)
			field := base.NewSelect(base.SelectConfig[int]{
				Same: base.Equal[int], Initial: 1,
				Options: []base.Option[int]{{Label: "one", Value: 1}, {Label: "two", Value: 2}},
			})
			if err := field.Element().SetPaint(func(frame scene.Frame) { field.DrawWith(frame, test.look) }); err != nil {
				t.Fatal(err)
			}
			surface := grid.NewSurface(20, 2)
			scenes.Mount(field).Draw(surface.View().Sub(grid.Area(3, 0, 17, 2)))
			if got, want := surface.Rows(), []string{"   " + test.first, "   " + test.last}; !reflect.DeepEqual(got, want) {
				t.Fatalf("choice rows = %q, want %q", got, want)
			}
		})
	}
}

func TestConfirmTextProjectionOwnsItsPointerSplit(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	field := base.NewConfirm(base.ConfirmConfig{Initial: true, Yes: "a\tb", No: "N"})
	look := base.Look{Taken: "❤\n\ufe0fX", Free: "-"}
	if err := field.Element().SetPaint(func(frame scene.Frame) { field.DrawWith(frame, look) }); err != nil {
		t.Fatal(err)
	}
	root := scenes.Mount(field)
	surface := grid.NewSurface(24, 1)
	root.Draw(surface.View())
	if got, want := surface.Rows()[0], "❤X a       b  -  N"; got != want {
		t.Fatalf("answers = %q, want %q", got, want)
	}
	if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(13, 0)}) || !field.Answer() {
		t.Fatal("last column before the second answer did not belong to yes")
	}
	if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(14, 0)}) || field.Answer() {
		t.Fatal("first column of the second answer did not belong to no")
	}
}

func TestCompletionRightAlignsItsTruncatedDetailProjection(t *testing.T) {
	surface := grid.NewSurface(6, 1)
	base.DefaultCompletionRenderer{}.DrawRow(surface.View(), base.Candidate{Text: "C", Detail: "界界"}, false, base.Look{})
	if got, want := surface.Rows()[0], "C   界"; got != want {
		t.Fatalf("completion = %q, want %q", got, want)
	}
	if got, ok := surface.CellAt(5, 0); !ok || got.Width() != 0 {
		t.Fatalf("rightmost column = %+v, want the detail's continuation", got)
	}
}
