package text_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"

	"github.com/Tangerg/oolong/ui/core/text"
)

func expectAffinityPanic(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, text.ErrInvalidAffinity) {
			t.Errorf("invalid affinity = %v", err)
		}
	}()
	run()
}

func TestLayoutRejectsUnknownAffinityBeforeProjection(t *testing.T) {
	invalid := text.Position{Byte: 1, Affinity: text.Affinity(math.MaxUint8)}
	for _, fixture := range []struct {
		name   string
		layout *text.Layout
	}{
		{name: "nil"},
		{name: "empty", layout: text.NewLayout(text.LayoutConfig{})},
		{name: "grapheme", layout: text.NewLayout(text.LayoutConfig{Lines: []text.Line{text.Of("e\u0301x", grid.Style{})}, Width: 1})},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			for _, operation := range []struct {
				name string
				run  func()
			}{
				{"Normalize", func() { fixture.layout.Normalize(invalid) }},
				{"Caret", func() { fixture.layout.Caret(invalid) }},
				{"RectsStart", func() { fixture.layout.Rects(text.Range{Start: invalid}) }},
				{"RectsEnd", func() { fixture.layout.Rects(text.Range{End: invalid}) }},
				{"TextStart", func() { fixture.layout.Text(text.Range{Start: invalid}) }},
				{"TextEnd", func() { fixture.layout.Text(text.Range{End: invalid}) }},
			} {
				t.Run(operation.name, func(t *testing.T) {
					expectAffinityPanic(t, operation.run)
				})
			}
		})
	}
	layout := text.NewLayout(text.LayoutConfig{Lines: []text.Line{text.Of("content", grid.Style{})}})
	for _, line := range []int{-1, 1} {
		invalid.Line = line
		expectAffinityPanic(t, func() { layout.Normalize(invalid) })
	}
}
