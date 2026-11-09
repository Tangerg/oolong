package layout_test

import (
	"errors"
	"image"
	"math"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/core/layout"
)

func TestLayoutEnumValidationCoversTheWholeDomain(t *testing.T) {
	for _, tc := range []struct {
		name         string
		last         uint8
		validate     func(uint8) error
		invalidError error
	}{
		{"axis", uint8(layout.Across), func(value uint8) error { return layout.Axis(value).Validate() }, layout.ErrInvalidAxis},
		{"align", uint8(layout.End), func(value uint8) error { return layout.Align(value).Validate() }, layout.ErrInvalidAlign},
		{"anchor", uint8(layout.BottomRight), func(value uint8) error { return layout.Anchor(value).Validate() }, layout.ErrInvalidAnchor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for value := 0; value <= math.MaxUint8; value++ {
				err := tc.validate(uint8(value))
				if value <= int(tc.last) {
					if err != nil {
						t.Errorf("legal value %d: %v", value, err)
					}
				} else if !errors.Is(err, tc.invalidError) {
					t.Errorf("unknown value %d: %v", value, err)
				}
			}
		})
	}
}

func expectLayoutBoundaryPanic(t *testing.T, want error, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, want) {
			t.Errorf("panic = %v, want %v", err, want)
		}
	}()
	run()
}

func TestFlowRejectsInvalidCandidatesBeforeMeasuring(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(layout.Flow, int, []layout.Slot)
	}{
		{"Rects", func(flow layout.Flow, extent int, slots []layout.Slot) { flow.Rects(image.Pt(extent, extent), slots) }},
		{"Divide", func(flow layout.Flow, extent int, slots []layout.Slot) { flow.Divide(extent, extent, slots) }},
		{"Wanted", func(flow layout.Flow, extent int, slots []layout.Slot) { flow.Wanted(extent, slots) }},
		{"CrossWanted", func(flow layout.Flow, extent int, slots []layout.Slot) { flow.CrossWanted(extent, slots) }},
	} {
		for _, extent := range []int{0, 12} {
			for _, tc := range []struct {
				name  string
				flow  layout.Flow
				cross layout.Cross
				empty bool
				want  error
			}{
				{name: "empty invalid axis", flow: layout.Flow{Axis: layout.Axis(math.MaxUint8)}, empty: true, want: layout.ErrInvalidAxis},
				{name: "measured invalid axis", flow: layout.Flow{Axis: layout.Axis(math.MaxUint8)}, want: layout.ErrInvalidAxis},
				{name: "invalid fill alignment", cross: layout.Cross{Align: layout.Align(math.MaxUint8)}, want: layout.ErrInvalidAlign},
				{name: "invalid bounded alignment", cross: layout.Cross{Size: 1, Align: layout.Align(math.MaxUint8)}, want: layout.ErrInvalidAlign},
				{name: "invalid oversized alignment", cross: layout.Cross{Size: math.MaxInt, Align: layout.Align(math.MaxUint8)}, want: layout.ErrInvalidAlign},
			} {
				t.Run(operation.name+"/"+strconv.Itoa(extent)+"/"+tc.name, func(t *testing.T) {
					calls := 0
					probe := layout.MeasureFunc(func(layout.Axis, int) int {
						calls++
						return 2
					})
					slots := []layout.Slot{
						{Size: layout.Measured(0, 0), Of: probe},
						{Size: layout.Fixed(1), Cross: tc.cross},
					}
					if tc.empty {
						slots = nil
					}
					expectLayoutBoundaryPanic(t, tc.want, func() { operation.run(tc.flow, extent, slots) })
					if calls != 0 {
						t.Fatalf("invalid candidate called the measurer %d times", calls)
					}
				})
			}
		}
	}
}

func TestMeasureFuncRejectsAnInvalidAxisBeforeTheCallback(t *testing.T) {
	calls := 0
	measure := layout.MeasureFunc(func(layout.Axis, int) int {
		calls++
		return 1
	})
	expectLayoutBoundaryPanic(t, layout.ErrInvalidAxis, func() { measure.Measure(layout.Axis(math.MaxUint8), 0) })
	if calls != 0 {
		t.Fatalf("invalid axis called the measurer %d times", calls)
	}
}

func TestEmptyGeometryStillRejectsInvalidAlignmentAndAnchor(t *testing.T) {
	for _, extent := range []int{-1, 0, 10} {
		t.Run(strconv.Itoa(extent), func(t *testing.T) {
			expectLayoutBoundaryPanic(t, layout.ErrInvalidAlign, func() {
				layout.Align(math.MaxUint8).Offset(extent, extent)
			})
			expectLayoutBoundaryPanic(t, layout.ErrInvalidAnchor, func() {
				(layout.Placement{Anchor: layout.Anchor(math.MaxUint8)}).In(image.Pt(extent, extent))
			})
		})
	}
}
