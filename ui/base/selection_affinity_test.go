package base_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestSelectionRejectsUnknownAffinityBeforeChangingGesture(t *testing.T) {
	first := base.TextPoint{Content: 1, Revision: 1, Position: text.Position{Byte: 1}}
	last := base.TextPoint{Content: 1, Revision: 1, Position: text.Position{Byte: 4, Affinity: text.Backward}}
	invalid := first
	invalid.Position.Affinity = text.Affinity(math.MaxUint8)
	for _, state := range []struct {
		name  string
		start func(*base.Selection)
	}{
		{"zero", func(*base.Selection) {}},
		{"empty range", func(s *base.Selection) { s.Set(base.TextRange{Start: first, End: first}) }},
		{"settled", func(s *base.Selection) { s.Set(base.TextRange{Start: last, End: first}) }},
		{"dragging", func(s *base.Selection) { s.Begin(last); s.Extend(first) }},
	} {
		for _, operation := range []struct {
			name  string
			apply func(*base.Selection)
		}{
			{"begin", func(s *base.Selection) { s.Begin(invalid) }},
			{"set start", func(s *base.Selection) { s.Set(base.TextRange{Start: invalid, End: last}) }},
			{"set end", func(s *base.Selection) { s.Set(base.TextRange{Start: first, End: invalid}) }},
			{"extend", func(s *base.Selection) { s.Extend(invalid) }},
		} {
			t.Run(state.name+"/"+operation.name, func(t *testing.T) {
				var selection base.Selection
				state.start(&selection)
				anchor := selection.Anchor()
				start, end := selection.Range()
				active, dragging := selection.Active(), selection.Dragging()
				assertInvalidAffinity(t, func() { operation.apply(&selection) })
				gotStart, gotEnd := selection.Range()
				if selection.Anchor() != anchor || gotStart != start || gotEnd != end ||
					selection.Active() != active || selection.Dragging() != dragging {
					t.Fatalf("rejected affinity changed selection: anchor=%+v range=(%+v, %+v) active=%t dragging=%t",
						selection.Anchor(), gotStart, gotEnd, selection.Active(), selection.Dragging())
				}
			})
		}
	}
}

func TestTranscriptCaretRejectsUnknownAffinityBeforeProjection(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, transcriptBody("one two"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 1, 1)
	projection := model.Layout()
	span, ok := projection.Hit(base.Point{Row: 0, Col: 1})
	if !ok {
		t.Fatal("committed transcript declined a text hit")
	}
	stale := span.Start
	stale.Revision++
	for _, test := range []struct {
		name   string
		layout base.TranscriptLayout
		point  base.TextPoint
		valid  bool
	}{
		{"current", projection, span.Start, true},
		{"stale", projection, stale, false},
		{"empty", base.TranscriptLayout{}, base.TextPoint{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := test.layout.Caret(test.point); ok != test.valid {
				t.Fatalf("fixture caret availability = %t, want %t", ok, test.valid)
			}
			point := test.point
			point.Position.Affinity = text.Affinity(math.MaxUint8)
			assertInvalidAffinity(t, func() { test.layout.Caret(point) })
		})
	}
}

func assertInvalidAffinity(t *testing.T, action func()) {
	t.Helper()
	var failure any
	func() {
		defer func() { failure = recover() }()
		action()
	}()
	err, _ := failure.(error)
	if !errors.Is(err, text.ErrInvalidAffinity) {
		t.Errorf("invalid affinity panic = %v, want ErrInvalidAffinity", failure)
	}
}
