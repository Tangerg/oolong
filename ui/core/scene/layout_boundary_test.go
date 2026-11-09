package scene_test

import (
	"errors"
	"image"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func expectAxisPanic(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, layout.ErrInvalidAxis) {
			t.Errorf("panic = %v, want %v", err, layout.ErrInvalidAxis)
		}
	}()
	run()
}

func TestMeasureRejectsInvalidAxesBeforeApplicationCode(t *testing.T) {
	calls := 0
	for _, element := range []scene.Element{
		{},
		scene.NewElement(scene.ElementConfig{}),
		scene.NewElement(scene.ElementConfig{Measure: func(layout.Axis, int) int {
			calls++
			return 1
		}}),
	} {
		expectAxisPanic(t, func() { element.Measure(layout.Axis(math.MaxUint8), 0) })
	}
	if calls != 0 {
		t.Fatalf("invalid axis reached application code %d times", calls)
	}
}

func TestInvalidGroupAxesCannotClaimChildren(t *testing.T) {
	for _, tc := range []struct {
		name    string
		overlay bool
		custom  bool
	}{
		{name: "flow"},
		{name: "overlay", overlay: true},
		{name: "custom", custom: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := scene.NewElement(scene.ElementConfig{})
			config := scene.GroupConfig{
				Axis: layout.Axis(math.MaxUint8), Overlay: tc.overlay,
				Items: []scene.Item{{Element: child}},
			}
			if tc.custom {
				config.Layout = func(space image.Point, _ []scene.Item) []image.Rectangle {
					return []image.Rectangle{{Max: space}}
				}
			}
			expectAxisPanic(t, func() { scene.NewGroup(config) })
			group := scene.NewGroup(scene.GroupConfig{})
			if err := group.Set(scene.Item{Element: child}); err != nil {
				t.Fatalf("rejected group retained its child: %v", err)
			}
		})
	}
}
