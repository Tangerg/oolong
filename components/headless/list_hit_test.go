package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func assertRowHit(t *testing.T, hit func(image.Point) (int, bool), point image.Point, want int) {
	t.Helper()
	got, ok := hit(point)
	if ok != (want >= 0) || ok && got != want {
		t.Fatalf("Hit(%v) = (%d, %v), want index %d", point, got, ok, want)
	}
}

type hitFrameProbe struct {
	child   headless.Widget
	inspect func()
}

func (p *hitFrameProbe) Draw(frame headless.Frame) {
	p.child.Draw(frame)
	if p.inspect != nil {
		p.inspect()
	}
}

func TestListHitUsesBothVisibleAxesAndRejectsBlankRows(t *testing.T) {
	var list headless.List[int]
	assertRowHit(t, list.Hit, image.Pt(0, 0), -1)
	list.SetItems([]int{0, 1, 2})
	assertRowHit(t, list.Hit, image.Pt(0, 0), -1)
	headless.NewRoot(&list).Draw(grid.NewSurface(4, 4).View())
	for _, test := range []struct {
		point image.Point
		want  int
	}{
		{point: image.Pt(0, 0), want: 0},
		{point: image.Pt(3, 2), want: 2},
		{point: image.Pt(-1, 1), want: -1},
		{point: image.Pt(4, 1), want: -1},
		{point: image.Pt(1, -1), want: -1},
		{point: image.Pt(1, 4), want: -1},
		{point: image.Pt(1, 3), want: -1},
	} {
		assertRowHit(t, list.Hit, test.point, test.want)
	}
	if list.Selected() != 0 || list.Scroll().Offset() != 0 || list.Scroll().FollowingEnd() {
		t.Fatal("hit queries changed list selection or scrolling")
	}
	list.SetItems(nil)
	headless.NewRoot(&list).Draw(grid.NewSurface(4, 4).View())
	assertRowHit(t, list.Hit, image.Pt(0, 0), -1)
}

func TestListHitKeepsTheLogicalRowOriginWhenClipped(t *testing.T) {
	var list headless.List[int]
	list.SetItems([]int{0, 1, 2, 3, 4})
	view := grid.NewSurface(4, 2).View().Sub(image.Rect(-2, -1, 6, 4))
	headless.NewRoot(&list).Draw(view)
	for _, test := range []struct {
		point image.Point
		want  int
	}{
		{point: image.Pt(2, 1), want: 1},
		{point: image.Pt(5, 2), want: 2},
		{point: image.Pt(1, 2), want: -1},
		{point: image.Pt(6, 2), want: -1},
		{point: image.Pt(3, 0), want: -1},
		{point: image.Pt(3, 3), want: -1},
	} {
		assertRowHit(t, list.Hit, test.point, test.want)
	}
}

func TestListHitWaitsForTheWholeFrameToCommit(t *testing.T) {
	var list headless.List[int]
	list.SetItems([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	probe := &hitFrameProbe{child: &list}
	root := headless.NewRoot(probe)
	root.Draw(grid.NewSurface(4, 3).View())
	list.Scroll().By(4)
	assertRowHit(t, list.Hit, image.Pt(3, 2), 2)
	if list.Selected() != 0 || list.Scroll().Offset() != 4 {
		t.Fatal("hit query changed selection or pending scroll position")
	}
	probe.inspect = func() {
		assertRowHit(t, list.Hit, image.Pt(3, 2), 2)
		assertRowHit(t, list.Hit, image.Pt(1, 1), 1)
	}
	root.Draw(grid.NewSurface(2, 2).View())
	assertRowHit(t, list.Hit, image.Pt(3, 2), -1)
	assertRowHit(t, list.Hit, image.Pt(1, 1), 5)
	list.Select(9)
	assertRowHit(t, list.Hit, image.Pt(1, 1), 5)
	if list.Selected() != 9 || list.Scroll().Offset() != 8 {
		t.Fatal("hit query changed pending selection navigation")
	}
	probe.inspect = nil
	root.Draw(grid.NewSurface(2, 2).View())
	assertRowHit(t, list.Hit, image.Pt(1, 1), 9)
}

func TestListHitRejectsAReplacementUntilItsFrameCommits(t *testing.T) {
	var list headless.List[string]
	list.SetItems([]string{"old zero", "old one"})
	probe := &hitFrameProbe{child: &list}
	root := headless.NewRoot(probe)
	view := grid.NewSurface(4, 3).View()
	root.Draw(view)
	list.Select(1)
	list.SetItems([]string{"new zero", "new one"})
	assertRowHit(t, list.Hit, image.Pt(1, 1), -1)
	probe.inspect = func() {
		assertRowHit(t, list.Hit, image.Pt(1, 1), -1)
	}
	root.Draw(view)
	assertRowHit(t, list.Hit, image.Pt(1, 1), 1)
	if list.Selected() != 1 {
		t.Fatal("hit query changed the selection retained by index")
	}
	if item, _ := list.At(1); item != "new one" {
		t.Fatalf("hit index identifies %q instead of the replacement", item)
	}
	list.SetItems([]string{"only"})
	assertRowHit(t, list.Hit, image.Pt(1, 0), -1)
	probe.inspect = nil
	root.Draw(view)
	assertRowHit(t, list.Hit, image.Pt(1, 0), 0)
	assertRowHit(t, list.Hit, image.Pt(1, 1), -1)
}

func TestListHitPreservesTheCommittedFrameWhenDrawingFails(t *testing.T) {
	for _, invalidFrame := range []bool{false, true} {
		var list headless.List[int]
		list.SetItems([]int{0, 1, 2, 3, 4, 5, 6, 7})
		probe := &hitFrameProbe{child: &list}
		root := headless.NewRoot(probe)
		root.Draw(grid.NewSurface(4, 3).View())
		list.Scroll().By(4)
		probe.inspect = func() { panic("aborted hit frame") }
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid draw did not panic")
				}
			}()
			if invalidFrame {
				list.Draw(headless.Frame{})
				return
			}
			root.Draw(grid.NewSurface(2, 2).View())
		}()
		assertRowHit(t, list.Hit, image.Pt(3, 2), 2)
		if list.Scroll().Offset() != 4 {
			t.Fatal("hit query changed scrolling after an aborted draw")
		}
		probe.inspect = nil
		root.Draw(grid.NewSurface(2, 2).View())
		assertRowHit(t, list.Hit, image.Pt(3, 2), -1)
		assertRowHit(t, list.Hit, image.Pt(1, 1), 5)
	}
}

func TestListHitClearsItsRegionWithAnEmptyCommittedFrame(t *testing.T) {
	for _, empty := range []grid.View{
		{},
		grid.NewSurface(0, 3).View(),
		grid.NewSurface(4, 0).View(),
		grid.NewSurface(4, 3).View().Sub(image.Rect(4, 3, 8, 6)),
	} {
		var list headless.List[int]
		list.SetItems([]int{0, 1, 2})
		root := headless.NewRoot(&list)
		view := grid.NewSurface(4, 3).View()
		root.Draw(view)
		assertRowHit(t, list.Hit, image.Pt(1, 1), 1)
		root.Draw(empty)
		assertRowHit(t, list.Hit, image.Pt(1, 1), -1)
		root.Draw(view)
		assertRowHit(t, list.Hit, image.Pt(1, 1), 1)
	}
}

func TestListPointerSelectionRejectsInvisiblePoints(t *testing.T) {
	for _, action := range []input.MouseAction{input.MouseDown, input.MouseDrag} {
		for _, test := range []struct {
			name  string
			point image.Point
		}{
			{name: "left", point: image.Pt(1, 2)},
			{name: "right", point: image.Pt(6, 2)},
			{name: "above", point: image.Pt(3, 0)},
			{name: "below", point: image.Pt(3, 3)},
		} {
			t.Run(test.name, func(t *testing.T) {
				var list headless.List[int]
				list.SetItems([]int{0, 1, 2, 3, 4})
				view := grid.NewSurface(4, 2).View().Sub(image.Rect(-2, -1, 6, 4))
				headless.NewRoot(&list).Draw(view)
				if list.Handle(input.Mouse{Action: action, Button: input.ButtonLeft, Pos: test.point}) {
					t.Fatalf("pointer action %v selected an invisible point %v", action, test.point)
				}
				if list.Selected() != 0 {
					t.Fatalf("an invisible pointer action moved selection to %d", list.Selected())
				}
			})
		}
	}
}
