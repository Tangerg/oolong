package layout_test

import (
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/core/layout"
)

// wants is a measurer that asks for a fixed amount, remembering what it was asked.
type wants struct {
	give   int
	across int
	asked  bool
}

func (w *wants) Measure(across int) int {
	w.across, w.asked = across, true
	return w.give
}

func sizes(rects []image.Rectangle, vertical bool) []int {
	out := make([]int, len(rects))
	for i, r := range rects {
		if vertical {
			out[i] = r.Dy()
		} else {
			out[i] = r.Dx()
		}
	}
	return out
}

func equal(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFixedSlotsAreHonouredBeforeAnythingElse(t *testing.T) {
	views := layout.Down.Rects(image.Pt(10, 10),
		layout.Slot{Size: layout.Fixed(2)},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(3)},
	)
	equal(t, sizes(views, true), []int{2, 5, 3})
}

func TestFlexSlotsSplitWhatIsLeftInProportion(t *testing.T) {
	views := layout.Down.Rects(image.Pt(4, 9),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Flex(2)},
	)
	equal(t, sizes(views, true), []int{3, 6})
}

func TestTheRoundingRemainderIsNotLost(t *testing.T) {
	// A unit lost to rounding makes the allocation smaller than its input.
	views := layout.Down.Rects(image.Pt(4, 10),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Flex(2)},
	)
	total := 0
	for _, n := range sizes(views, true) {
		total += n
	}
	if total != 10 {
		t.Fatalf("slots add up to %d units, want all 10", total)
	}
}

func TestAMeasuredSlotIsAskedAcrossTheOtherAxis(t *testing.T) {
	w := &wants{give: 3}
	views := layout.Down.Rects(image.Pt(12, 10),
		layout.Slot{Size: layout.Measured(0, 0), Of: w},
		layout.Slot{Size: layout.Flex(1)},
	)
	if !w.asked {
		t.Fatal("the measurer was never asked")
	}
	if w.across != 12 {
		t.Fatalf("measured across %d, want the width of 12", w.across)
	}
	equal(t, sizes(views, true), []int{3, 7})
}

func TestMeasuringWorksAcrossBothAxes(t *testing.T) {
	// The reason Measure takes the axis it is not deciding: a slot divided Across is
	// asked for a width at a height, and one method means the same thing in both.
	w := &wants{give: 4}
	views := layout.Across.Rects(image.Pt(30, 5),
		layout.Slot{Size: layout.Measured(0, 0), Of: w},
		layout.Slot{Size: layout.Flex(1)},
	)
	if w.across != 5 {
		t.Fatalf("measured across %d, want the height of 5", w.across)
	}
	equal(t, sizes(views, false), []int{4, 26})
}

func TestAMeasuredSlotIsHeldBetweenItsFloorAndItsCap(t *testing.T) {
	for _, tc := range []struct {
		name       string
		give       int
		min, max   int
		wantHeight int
	}{
		{"below the floor", 1, 3, 0, 3},
		{"above the cap", 9, 0, 4, 4},
		{"between the two", 5, 3, 8, 5},
		{"no cap at all", 9, 0, 0, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			views := layout.Down.Rects(image.Pt(10, 20),
				layout.Slot{Size: layout.Measured(tc.min, tc.max), Of: &wants{give: tc.give}},
				layout.Slot{Size: layout.Flex(1)},
			)
			if got := sizes(views, true)[0]; got != tc.wantHeight {
				t.Fatalf("measured slot = %d units, want %d", got, tc.wantHeight)
			}
		})
	}
}

func TestAMeasuredSlotWithNothingToAskGetsItsFloor(t *testing.T) {
	views := layout.Down.Rects(image.Pt(10, 20),
		layout.Slot{Size: layout.Measured(2, 0)},
		layout.Slot{Size: layout.Flex(1)},
	)
	equal(t, sizes(views, true), []int{2, 18})
}

func TestNeverHandsOutMoreThanThereIs(t *testing.T) {
	// Two floors that do not both fit. Honouring them both would make the reported
	// allocation larger than the space it divides.
	views := layout.Down.Rects(image.Pt(20, 10),
		layout.Slot{Size: layout.Flex(1).AtLeast(8)},
		layout.Slot{Size: layout.Flex(1).AtLeast(8)},
	)
	total := 0
	for _, r := range views {
		total += r.Dy()
	}
	if total != 10 {
		t.Fatalf("slots add up to %d units, want the 10 there are", total)
	}
}

func TestSizingConstructorsDoNotExpressNegativeExtents(t *testing.T) {
	got := layout.Divide(12, 1, []layout.Slot{
		{Size: layout.Fixed(-1)},
		{Size: layout.Part(-1, 2)},
		{Size: layout.Flex(-1)},
		{Size: layout.Measured(-1, -1)},
	})
	equal(t, got, []int{0, 0, 0, 0})
}

func TestExtentSumsStayNonnegativeAtIntegerLimits(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	if got := layout.Sum(-1, 2, maxInt, 1); got != maxInt {
		t.Fatalf("sum = %d, want saturation at %d", got, maxInt)
	}
	if got := layout.Sum(-1, -2); got != 0 {
		t.Fatalf("negative extents summed to %d, want zero", got)
	}
	if got := layout.Remaining(minInt, maxInt); got != 0 {
		t.Fatalf("remaining negative room = %d, want zero", got)
	}
	if got := layout.Remaining(maxInt, maxInt, maxInt); got != 0 {
		t.Fatalf("overdrawn room = %d, want zero", got)
	}
}

func TestCoordinateTranslationKeepsDirectionAtIntegerLimits(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	for _, tc := range []struct {
		at, delta, want int
	}{
		{maxInt, 1, maxInt},
		{minInt, -1, minInt},
		{maxInt, -1, maxInt - 1},
		{minInt, 1, minInt + 1},
		{-3, 5, 2},
	} {
		if got := layout.Translate(tc.at, tc.delta); got != tc.want {
			t.Errorf("Translate(%d, %d) = %d, want %d", tc.at, tc.delta, got, tc.want)
		}
	}
}

func TestRelativeCoordinateKeepsDirectionAtIntegerLimits(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	for _, tc := range []struct {
		at, origin, want int
	}{
		{minInt, 1, minInt},
		{maxInt, -1, maxInt},
		{maxInt, maxInt, 0},
		{minInt, minInt, 0},
		{2, -3, 5},
	} {
		if got := layout.Relative(tc.at, tc.origin); got != tc.want {
			t.Errorf("Relative(%d, %d) = %d, want %d", tc.at, tc.origin, got, tc.want)
		}
	}
}

func TestSizingRejectsContradictoryConstraints(t *testing.T) {
	for name, makeSizing := range map[string]func() layout.Sizing{
		"measured bounds": func() layout.Sizing { return layout.Measured(3, 2) },
		"measured floor":  func() layout.Sizing { return layout.Measured(0, 2).AtLeast(3) },
		"fixed floor":     func() layout.Sizing { return layout.Fixed(2).AtLeast(1) },
		"zero floor":      func() layout.Sizing { return (layout.Sizing{}).AtLeast(1) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("contradictory sizing did not panic")
				}
			}()
			makeSizing()
		})
	}
}

func TestASlotSqueezedToNothingStillGetsARectangle(t *testing.T) {
	rects := layout.Down.Rects(image.Pt(10, 1),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(5)},
	)
	if len(rects) != 2 {
		t.Fatalf("got %d rectangles, want one per slot", len(rects))
	}
	if h := rects[1].Dy(); h != 0 {
		t.Fatalf("the squeezed slot got %d units, want none", h)
	}
}

func TestDivideAnswersWithoutRectangles(t *testing.T) {
	// Related geometry can use the allocation without constructing rectangles.
	got := layout.Divide(10, 4, []layout.Slot{{Size: layout.Fixed(3)}, {Size: layout.Flex(1)}})
	equal(t, got, []int{3, 7})
}

func TestAlignPlacesContentInASpaceWiderThanItself(t *testing.T) {
	for _, tc := range []struct {
		align layout.Align
		want  int
	}{
		{layout.Start, 0},
		{layout.Center, 3},
		{layout.End, 6},
	} {
		if got := tc.align.Offset(10, 4); got != tc.want {
			t.Errorf("align %d: offset = %d, want %d", tc.align, got, tc.want)
		}
	}
}

func TestAlignNeverPlacesContentOutsideTheSpace(t *testing.T) {
	for _, align := range []layout.Align{layout.Start, layout.Center, layout.End} {
		if got := align.Offset(4, 10); got != 0 {
			t.Errorf("align %d: content wider than its space starts at %d, want 0", align, got)
		}
	}
}

func TestAlignTreatsNegativeExtentsAsEmpty(t *testing.T) {
	for _, align := range []layout.Align{layout.Start, layout.Center, layout.End} {
		if got := align.Offset(-4, -2); got != 0 {
			t.Errorf("align %d: offset = %d, want 0", align, got)
		}
	}
}

func TestInsetReportsWhatItTakesAndWhatItLeaves(t *testing.T) {
	in := layout.Symmetric(1, 2)
	if got := in.Size(); got != image.Pt(4, 2) {
		t.Fatalf("size = %+v, want horizontal 4 and vertical 2", got)
	}
	left := in.Apply(image.Rect(0, 0, 10, 6))
	if left != image.Rect(2, 1, 8, 5) {
		t.Fatalf("inner = %v, want the region less the inset", left)
	}
}

func TestAnInsetBiggerThanItsRegionLeavesNothing(t *testing.T) {
	if left := layout.Uniform(5).Apply(image.Rect(0, 0, 4, 4)); !left.Empty() {
		t.Fatalf("inner = %v, want nothing left", left)
	}
}

func TestNegativeInsetsDoNotExpandARegion(t *testing.T) {
	inset := layout.Inset{Top: -1, Right: -2, Bottom: -3, Left: -4}
	region := image.Rect(2, 3, 8, 9)
	if got := inset.Apply(region); got != region {
		t.Fatalf("inner = %v, want the original region", got)
	}
	if got := inset.Size(); got != (image.Point{}) {
		t.Fatalf("size = %v, want zero", got)
	}
}

func TestPlacementAnchors(t *testing.T) {
	space := image.Pt(10, 6)
	for _, tc := range []struct {
		anchor layout.Anchor
		wantX  int
		wantY  int
	}{
		{layout.TopLeft, 0, 0},
		{layout.Top, 3, 0},
		{layout.TopRight, 6, 0},
		{layout.Left, 0, 2},
		{layout.Middle, 3, 2},
		{layout.Right, 6, 2},
		{layout.BottomLeft, 0, 4},
		{layout.Bottom, 3, 4},
		{layout.BottomRight, 6, 4},
	} {
		got := layout.Placement{Anchor: tc.anchor, Width: 4, Height: 2}.In(space)
		if got.Min.X != tc.wantX || got.Min.Y != tc.wantY {
			t.Errorf("anchor %d: at (%d,%d), want (%d,%d)",
				tc.anchor, got.Min.X, got.Min.Y, tc.wantX, tc.wantY)
		}
	}
}

func TestPlacementIsClampedToItsSpace(t *testing.T) {
	got := layout.Placement{Anchor: layout.Middle, Width: 100, Height: 100}.In(image.Pt(10, 6))
	if got != image.Rect(0, 0, 10, 6) {
		t.Fatalf("area = %v, want it clamped to the space", got)
	}
}

func TestPlacementWithNoSizeFillsWhatTheMarginLeaves(t *testing.T) {
	got := layout.Placement{Margin: 1}.In(image.Pt(10, 6))
	if got != image.Rect(1, 1, 9, 5) {
		t.Fatalf("area = %v, want the space less the margin", got)
	}
}

func TestPlacementWithNowhereToGo(t *testing.T) {
	if got := (layout.Placement{Margin: 10}).In(image.Pt(4, 4)); !got.Empty() {
		t.Fatalf("area = %v, want nothing", got)
	}
}

func TestPlacementNormalizesNegativeSpaceAndMargin(t *testing.T) {
	if got := (layout.Placement{Margin: -4, Width: 2, Height: 2}).In(image.Pt(5, 5)); got != image.Rect(1, 1, 3, 3) {
		t.Fatalf("negative margin placed at %v, want the zero-margin placement", got)
	}
	if got := (layout.Placement{}).In(image.Pt(-5, -5)); !got.Empty() {
		t.Fatalf("negative space produced %v, want an empty rectangle", got)
	}
}

func TestARowOfSlotsCanHaveRoomBetweenThem(t *testing.T) {
	// Written once for the whole division rather than as padding on every slot but
	// the last — which is the version where the last one is wrong.
	flow := layout.Flow{Axis: layout.Across, Gap: 2}
	rects := flow.Rects(image.Pt(13, 1), []layout.Slot{
		{Size: layout.Flex(1)}, {Size: layout.Flex(1)}, {Size: layout.Flex(1)},
	})
	if len(rects) != 3 {
		t.Fatalf("divided into %d", len(rects))
	}
	for i, want := range []int{0, 5, 10} {
		if rects[i].Min.X != want || rects[i].Dx() != 3 {
			t.Fatalf("slot %d is %v, want width 3 at %d", i, rects[i], want)
		}
	}

	// And what is asked for altogether includes them, so something made of slots can
	// answer for itself when it is in a slot.
	slots := []layout.Slot{{Size: layout.Fixed(3)}, {Size: layout.Fixed(3)}}
	if got := flow.Wanted(1, slots); got != 8 {
		t.Fatalf("two slots of three with two between them want %d", got)
	}
}

func TestTheRoomBetweenSlotsDoesNotDependOnWhatIsInThem(t *testing.T) {
	// A gap that appeared and disappeared with its neighbour's extent would move
	// every following slot whenever an item happened to be empty.
	flow := layout.Flow{Axis: layout.Across, Gap: 1}
	full := flow.Rects(image.Pt(12, 1), []layout.Slot{
		{Size: layout.Fixed(4)}, {Size: layout.Fixed(3)}, {Size: layout.Fixed(2)},
	})
	empty := flow.Rects(image.Pt(12, 1), []layout.Slot{
		{Size: layout.Fixed(4)}, {Size: layout.Fixed(0)}, {Size: layout.Fixed(2)},
	})
	if full[2].Min.X != 9 {
		t.Fatalf("the third slot starts at %d", full[2].Min.X)
	}
	if empty[2].Min.X != 6 {
		t.Fatalf("with the middle slot empty the third starts at %d", empty[2].Min.X)
	}
}

func TestFlowNormalizesNegativeAndOversizedGaps(t *testing.T) {
	slots := []layout.Slot{{Size: layout.Flex(1)}, {Size: layout.Flex(1)}}
	without := (layout.Flow{Axis: layout.Across}).Rects(image.Pt(10, 1), slots)
	negative := (layout.Flow{Axis: layout.Across, Gap: -3}).Rects(image.Pt(10, 1), slots)
	if negative[0] != without[0] || negative[1] != without[1] {
		t.Fatalf("negative gap produced %v, want %v", negative, without)
	}

	bounded := (layout.Flow{Axis: layout.Across, Gap: 10}).Rects(image.Pt(3, 1), slots)
	for i, rect := range bounded {
		if rect.Min.X < 0 || rect.Max.X > 3 || rect.Min.Y < 0 || rect.Max.Y > 1 {
			t.Errorf("slot %d escaped its space: %v", i, rect)
		}
	}
	empty := layout.Down.Rects(image.Pt(-3, -2), slots...)
	for i, rect := range empty {
		if !rect.Empty() {
			t.Errorf("slot %d in negative space is %v, want empty", i, rect)
		}
	}

	maxInt := int(^uint(0) >> 1)
	huge := layout.Flow{Axis: layout.Across, Gap: maxInt}
	for i, rect := range huge.Rects(image.Pt(3, 1), append(slots, layout.Slot{})) {
		if rect.Min.X < 0 || rect.Max.X > 3 {
			t.Errorf("slot %d escaped with a huge gap: %v", i, rect)
		}
	}
	if got := huge.Wanted(1, []layout.Slot{{Size: layout.Fixed(1)}, {Size: layout.Fixed(1)}, {Size: layout.Fixed(1)}}); got != maxInt {
		t.Errorf("wanted = %d, want saturation at %d", got, maxInt)
	}
}

func TestASlotSaysWhereContentNarrowerThanItSits(t *testing.T) {
	rects := layout.Down.Rects(image.Pt(10, 3), []layout.Slot{
		{Size: layout.Fixed(1), Cross: layout.Cross{Size: 4, Align: layout.Center}},
		{Size: layout.Fixed(1), Cross: layout.Cross{Size: 4, Align: layout.End}},
		{Size: layout.Fixed(1)},
	}...)
	if got := rects[0]; got.Min.X != 3 || got.Dx() != 4 {
		t.Fatalf("centred content is at %v", got)
	}
	if got := rects[1]; got.Min.X != 6 || got.Dx() != 4 {
		t.Fatalf("content against the end is at %v", got)
	}
	// The zero value fills the region, which is what every slot did before there was
	// a way to say otherwise.
	if got := rects[2]; got.Min.X != 0 || got.Dx() != 10 {
		t.Fatalf("a slot that said nothing about the cross axis is %v", got)
	}

	// The same question the other way round: dividing width, a slot says how tall.
	across := layout.Across.Rects(image.Pt(3, 8), []layout.Slot{
		{Size: layout.Fixed(1), Cross: layout.Cross{Size: 2, Align: layout.Center}},
	}...)
	if got := across[0]; got.Min.Y != 3 || got.Dy() != 2 {
		t.Fatalf("centred content is at %v", got)
	}
}

func TestAShareOfTheWholeIsNotAShareOfWhatIsLeft(t *testing.T) {
	// The sentence a caller means literally: half of this, whatever else happens. A
	// share of the remainder cannot say it, because the remainder moves when anything
	// beside it does.
	half := layout.Slot{Size: layout.Part(1, 2)}
	rest := layout.Slot{Size: layout.Flex(1)}
	equal(t, layout.Divide(20, 1, []layout.Slot{half, rest}), []int{10, 10})

	// Another fixed slot appears before it, and the half is still a half.
	fixed := layout.Slot{Size: layout.Fixed(4)}
	equal(t, layout.Divide(20, 1, []layout.Slot{fixed, half, rest}), []int{4, 10, 6})

	// Where a share of what is left would have been six and then four.
	third := layout.Slot{Size: layout.Flex(1)}
	equal(t, layout.Divide(20, 1, []layout.Slot{fixed, third, rest}), []int{4, 8, 8})
}

func TestArithmeticSaturatesInsteadOfWrapping(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1

	if got := layout.Wanted(1, []layout.Slot{{Size: layout.Fixed(maxInt)}, {Size: layout.Fixed(1)}}); got != maxInt {
		t.Fatalf("wanted = %d, want saturation at %d", got, maxInt)
	}
	if got := (layout.Inset{Left: maxInt, Right: maxInt}).Size().X; got != maxInt {
		t.Fatalf("inset width = %d, want saturation at %d", got, maxInt)
	}
	extreme := image.Rectangle{Min: image.Pt(minInt, minInt), Max: image.Pt(maxInt, maxInt)}
	if got := (layout.Inset{Top: 1, Right: 1, Bottom: 1, Left: 1}).Apply(extreme); got != (image.Rectangle{Min: image.Pt(minInt+1, minInt+1), Max: image.Pt(maxInt-1, maxInt-1)}) {
		t.Fatalf("inset extreme rectangle = %v", got)
	}
	if got := (layout.Placement{Margin: maxInt}).In(image.Pt(maxInt, maxInt)); got != (image.Rectangle{}) {
		t.Fatalf("huge margin produced %v, want an empty rectangle", got)
	}

	nearWhole := layout.Divide(maxInt, 1, []layout.Slot{{Size: layout.Part(maxInt-1, maxInt)}})
	if nearWhole[0] != maxInt-1 {
		t.Fatalf("near-whole fraction = %d, want %d", nearWhole[0], maxInt-1)
	}
	// Three weights past the cap all saturate to the same value, so the ratio between
	// them is still one to one to one and the rounding remainder still goes to the
	// last of them. That is what saturating buys: weights nobody can tell apart
	// behave like the smallest weights that say the same thing.
	weighted := layout.Divide(10, 1, []layout.Slot{
		{Size: layout.Flex(maxInt)},
		{Size: layout.Flex(maxInt)},
		{Size: layout.Flex(maxInt)},
	})
	equal(t, weighted, []int{3, 3, 4})
	if plain := layout.Divide(10, 1, []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Flex(1)},
	}); !slices.Equal(weighted, plain) {
		t.Fatalf("saturated weights divided %v, want the same as %v", weighted, plain)
	}
}

func TestScaleIsOverflowSafeAndBounded(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		name               string
		total, part, whole int
		want               int
	}{
		{name: "half", total: 10, part: 1, whole: 2, want: 5},
		{name: "rounds down", total: 10, part: 2, whole: 3, want: 6},
		{name: "negative total", total: -1, part: 1, whole: 2},
		{name: "negative part", total: 10, part: -1, whole: 2},
		{name: "empty whole", total: 10, part: 1},
		{name: "past end", total: 10, part: 3, whole: 2, want: 10},
		{name: "near maximum", total: maxInt, part: maxInt - 1, whole: maxInt, want: maxInt - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := layout.Scale(tc.total, tc.part, tc.whole); got != tc.want {
				t.Fatalf("Scale(%d, %d, %d) = %d, want %d", tc.total, tc.part, tc.whole, got, tc.want)
			}
		})
	}
}

func BenchmarkDivide(b *testing.B) {
	for _, tc := range []struct {
		name  string
		slots []layout.Slot
	}{
		{
			name: "ordinary weights",
			slots: []layout.Slot{
				{Size: layout.Fixed(4)},
				{Size: layout.Flex(1)},
				{Size: layout.Flex(2)},
				{Size: layout.Measured(2, 20), Of: layout.MeasureFunc(func(int) int { return 8 })},
			},
		},
		{
			name: "saturated weights",
			slots: []layout.Slot{
				{Size: layout.Flex(int(^uint(0) >> 1))},
				{Size: layout.Flex(int(^uint(0) >> 1))},
				{Size: layout.Flex(int(^uint(0) >> 1))},
			},
		},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				layout.Divide(160, 40, tc.slots)
			}
		})
	}
}
