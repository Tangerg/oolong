package headless

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type routingTarget struct{ events int }

func (*routingTarget) Draw(Frame) {}

func (t *routingTarget) Handle(input.Event) bool {
	t.events++
	return true
}

type routingProbe struct {
	root  *Root
	point image.Point
	armed bool
}

func (p *routingProbe) Draw(Frame) {
	if p.armed {
		p.armed = false
		p.root.Handle(input.Mouse{Pos: p.point, Action: input.MouseMove})
	}
}

func TestRootPublishesNestedRoutingGeometryAtomically(t *testing.T) {
	a, b, c, d := &routingTarget{}, &routingTarget{}, &routingTarget{}, &routingTarget{}
	item := func(key string, target *routingTarget) Item { return Item{Key: key, Size: layout.Fixed(1), Of: target} }
	inner := NewContainer(layout.Down, item("a", a), item("b", b), item("c", c), item("d", d))
	probe := &routingProbe{point: image.Pt(0, 2)}
	outer := NewContainer(layout.Down, Item{Key: "inner", Size: layout.Fixed(4), Of: inner}, Item{Key: "probe", Of: probe})
	root := NewRoot(outer)
	probe.root = root
	root.Draw(grid.NewSurface(4, 5).View())
	// Old/old reaches c; new/new reaches a. Either mixed geometry reaches b or d.
	inner.Set(item("b", b), item("a", a), item("d", d), item("c", c))
	outer.Set(Item{Size: layout.Fixed(1)}, Item{Key: "inner", Size: layout.Fixed(4), Of: inner}, Item{Key: "probe", Of: probe})
	probe.armed = true
	root.Draw(grid.NewSurface(4, 5).View())
	if c.events != 1 || a.events != 0 || b.events != 0 || d.events != 0 {
		t.Fatal("input during Draw observed mixed geometry")
	}
	root.Handle(input.Mouse{Pos: probe.point, Action: input.MouseMove})
	if a.events != 1 || b.events != 0 || d.events != 0 {
		t.Fatal("committed frame did not publish both levels")
	}
}

type rootTarget struct {
	events int
	panics bool
}

func (t *rootTarget) Draw(Frame) {
	if t.panics {
		panic("aborted root replacement")
	}
}

func (t *rootTarget) Handle(input.Event) bool {
	t.events++
	return true
}

func TestRootPublishesItsOwnTargetWithTheCompleteFrame(t *testing.T) {
	old := &rootTarget{}
	next := &rootTarget{}
	root := NewRoot(old)
	view := grid.NewSurface(1, 1).View()

	if root.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("a root handled input before it presented a target")
	}
	root.Draw(view)
	root.SetContent(next)
	if root.Content() != next {
		t.Fatal("root did not expose the replacement selected for its next frame")
	}
	root.Handle(input.Key{Code: input.Enter})
	if old.events != 1 || next.events != 0 {
		t.Fatalf("unpublished replacement received input: old=%d next=%d", old.events, next.events)
	}

	root.Draw(view)
	root.Handle(input.Key{Code: input.Enter})
	if next.events != 1 {
		t.Fatalf("published replacement received %d events, want one", next.events)
	}
}

func TestAnAbortedRootReplacementKeepsThePresentedTarget(t *testing.T) {
	old := &rootTarget{}
	next := &rootTarget{panics: true}
	root := NewRoot(old)
	view := grid.NewSurface(1, 1).View()
	root.Draw(view)
	root.SetContent(next)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Draw did not propagate the replacement panic")
			}
		}()
		root.Draw(view)
	}()

	root.Handle(input.Key{Code: input.Enter})
	if old.events != 1 || next.events != 0 {
		t.Fatalf("aborted replacement received input: old=%d next=%d", old.events, next.events)
	}
}

func TestRootDoesNotTransferAPointerGestureToItsReplacement(t *testing.T) {
	old := &rootTarget{}
	next := &rootTarget{}
	root := NewRoot(old)
	view := grid.NewSurface(1, 1).View()
	root.Draw(view)

	root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	root.SetContent(next)
	root.Draw(view)
	root.Handle(input.Mouse{Action: input.MouseDrag, Pos: image.Pt(2, 2)})
	root.Handle(input.Mouse{Action: input.MouseUp, Pos: image.Pt(2, 2)})
	if old.events != 3 || next.events != 0 {
		t.Fatalf("gesture was transferred: old=%d next=%d", old.events, next.events)
	}

	root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	if next.events != 1 {
		t.Fatalf("replacement received %d events after release, want one", next.events)
	}
}

type snapshotFixture struct {
	value *int
	panic bool
	state Snapshot[*int]
}

func (f *snapshotFixture) Draw(frame Frame) {
	f.state.Stage(frame, f.value)
	if f.panic {
		panic("aborted frame")
	}
}

func TestRootAbortsPendingPresentationStateOnPanic(t *testing.T) {
	old, pending, next := 1, 2, 3
	fixture := &snapshotFixture{value: &old}
	root := NewRoot(fixture)
	view := grid.NewSurface(1, 1).View()
	root.Draw(view)

	fixture.value, fixture.panic = &pending, true
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Draw did not propagate the application panic")
			}
		}()
		root.Draw(view)
	}()

	if fixture.state.Value() != &old {
		t.Fatal("an aborted frame replaced the committed snapshot")
	}
	if fixture.state.pending != nil || fixture.state.staged != nil {
		t.Fatal("an aborted frame retained pending presentation state")
	}

	fixture.value, fixture.panic = &next, false
	root.Draw(view)
	if fixture.state.Value() != &next {
		t.Fatal("the root could not commit a frame after an abort")
	}
}

type duplicateSnapshotFixture struct{ state Snapshot[int] }

func (f *duplicateSnapshotFixture) Draw(frame Frame) {
	f.state.Stage(frame, 1)
	f.state.Stage(frame, 2)
}

func TestOnePresentationStateCanBeStagedOnlyOncePerFrame(t *testing.T) {
	fixture := &duplicateSnapshotFixture{}
	root := NewRoot(fixture)
	defer func() {
		if got := recover(); got != "headless: presentation state staged twice in one frame" {
			t.Fatalf("duplicate stage panic = %v", got)
		}
		if fixture.state.Value() != 0 || fixture.state.pending != 0 || fixture.state.staged != nil {
			t.Fatalf("duplicate stage survived abort: value=%d pending=%d staged=%p",
				fixture.state.Value(), fixture.state.pending, fixture.state.staged)
		}
	}()
	root.Draw(grid.NewSurface(1, 1).View())
}

type retainedFrameFixture struct {
	old   Frame
	reuse bool
	state Snapshot[int]
}

func (f *retainedFrameFixture) Draw(frame Frame) {
	if !f.reuse {
		f.old = frame
		return
	}
	f.state.Stage(f.old, 1)
}

func TestAFrameCannotBecomeValidAgainDuringALaterDraw(t *testing.T) {
	fixture := &retainedFrameFixture{}
	root := NewRoot(fixture)
	view := grid.NewSurface(1, 1).View()
	root.Draw(view)
	fixture.reuse = true
	defer func() {
		if got := recover(); got != "headless: presentation state staged outside its Root.Draw frame" {
			t.Fatalf("stale frame panic = %v", got)
		}
	}()
	root.Draw(view)
}

type resizeScrollFixture struct {
	scroll *Scroll
	before int
	after  int
}

func (f *resizeScrollFixture) Draw(frame Frame) {
	layout := f.scroll.Stage(frame, 20, 10)
	layout.Reveal(12, 12)
	f.before = layout.Offset()
	layout.Resize(5)
	f.after = layout.Offset()
}

func TestOneScrollLayoutCanBeRefinedWithinItsFrame(t *testing.T) {
	var scroll Scroll
	fixture := &resizeScrollFixture{scroll: &scroll}
	NewRoot(fixture).Draw(grid.NewSurface(1, 10).View())
	if fixture.before != 3 || fixture.after != 8 || scroll.Offset() != 8 {
		t.Fatalf("offset before resize=%d after=%d committed=%d; want 3, 8, 8 to retain row 12 in the smaller window",
			fixture.before, fixture.after, scroll.Offset())
	}
}

type scrollFixture struct {
	scroll     *Scroll
	total      int
	seenDuring int
}

func (f *scrollFixture) Draw(frame Frame) {
	f.scroll.Stage(frame, f.total, 5)
	f.seenDuring = f.scroll.Offset()
}

func TestScrollPublishesDerivedBoundsWithTheRootFrame(t *testing.T) {
	var scroll Scroll
	stageScrollForTest(&scroll, 10, 5)
	scroll.ToBottom()
	fixture := &scrollFixture{scroll: &scroll, total: 20}

	NewRoot(fixture).Draw(grid.NewSurface(1, 5).View())
	if fixture.seenDuring != 5 {
		t.Fatalf("Draw observed pending offset %d, want committed 5", fixture.seenDuring)
	}
	if got := scroll.Offset(); got != 15 {
		t.Fatalf("committed offset = %d, want new end 15", got)
	}
}

type wrappingBlock struct{}

func (wrappingBlock) Draw(grid.View) {}

func (wrappingBlock) HeightForWidth(width int) int {
	if width < 10 {
		return 2
	}
	return 1
}

type transcriptFixture struct {
	transcript  *Transcript
	width       int
	seenWidth   int
	seenHeight  int
	drawnHeight int
}

func (f *transcriptFixture) Draw(frame Frame) {
	layout := f.transcript.Stage(frame, f.width)
	f.seenWidth = f.transcript.Width()
	f.seenHeight = f.transcript.Height()
	f.drawnHeight = layout.Height()
}

func TestTranscriptReflowCommitsWithTheRootFrame(t *testing.T) {
	var transcript Transcript
	transcript.Append(wrappingBlock{})
	stageTranscriptForTest(&transcript, 10)
	fixture := &transcriptFixture{transcript: &transcript, width: 5}

	NewRoot(fixture).Draw(grid.NewSurface(5, 2).View())
	if fixture.seenWidth != 10 || fixture.seenHeight != 1 {
		t.Fatalf("Draw observed pending transcript %dx%d, want committed 10x1",
			fixture.seenWidth, fixture.seenHeight)
	}
	if fixture.drawnHeight != 2 {
		t.Fatalf("pending layout height = %d, want 2", fixture.drawnHeight)
	}
	if transcript.Width() != 5 || transcript.Height() != 2 {
		t.Fatalf("committed transcript = %dx%d, want 5x2", transcript.Width(), transcript.Height())
	}
}

type extremeBlock struct{ height int }

func (extremeBlock) Draw(grid.View) {}

func (b extremeBlock) HeightForWidth(int) int { return b.height }

func TestTranscriptPendingRowsCannotWrap(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	var transcript Transcript
	transcript.Append(extremeBlock{height: maxInt})
	transcript.Append(extremeBlock{height: 1})
	fixture := &transcriptFixture{transcript: &transcript, width: 1}

	NewRoot(fixture).Draw(grid.NewSurface(1, 1).View())
	if fixture.drawnHeight != maxInt || transcript.Height() != maxInt {
		t.Fatalf("pending height = %d, committed height = %d, want saturation at %d",
			fixture.drawnHeight, transcript.Height(), maxInt)
	}
}

func TestScrollRefinementKeepsTheOriginBeforeProvisionalClamping(t *testing.T) {
	var scroll Scroll
	stageScrollForTest(&scroll, 20, 5)
	scroll.ToBottom()
	fixture := &resizeScrollFixture{scroll: &scroll}
	NewRoot(fixture).Draw(grid.NewSurface(1, 10).View())
	if fixture.before != 10 || fixture.after != 12 || scroll.Offset() != 12 {
		t.Fatalf("refinement lost original offset 15: before=%d after=%d committed=%d",
			fixture.before, fixture.after, scroll.Offset())
	}
}
