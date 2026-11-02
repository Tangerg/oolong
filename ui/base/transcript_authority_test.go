package base_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/text"
)

type recordingProjector struct{ calls int }

func (*recordingProjector) HeightForWidth(int) int { return 1 }
func (*recordingProjector) Draw(grid.View)         {}
func (b *recordingProjector) Rows(int) []text.Row {
	b.calls++
	return []text.Row{{Text: "he\x1bllo"}, {Text: "outside the measured block"}}
}

func TestTranscriptQueriesReadOnlyCommittedRows(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	block := new(recordingProjector)
	transcriptAdd(t, m, block)
	transcriptAdd(t, m, transcriptBody("end"))
	r := transcriptScene(t, m)
	transcriptCommit(t, r, 20, 2, 1)
	projection := m.Layout()
	if got := projection.Rows(0, 2); len(got) != 2 || got[0].Text != "hello" || block.calls != 1 {
		t.Errorf("committed rows = %v; projection calls = %d", got, block.calls)
	}
	first, ok := projection.Hit(base.Point{})
	if !ok {
		t.Fatal("missing first row")
	}
	last, ok := projection.Hit(base.Point{Row: 1, Col: 2})
	if !ok {
		t.Fatal("missing last row")
	}
	m.Selection().Set(base.TextRange{Start: first.Start, End: last.End})
	if got, err := m.Selection().Text(projection); err != nil || got != "hello\nend" {
		t.Fatalf("copied %q (%v)", got, err)
	}
}

func TestTranscriptPointsCannotResolveInAnotherTranscript(t *testing.T) {
	layout := func(mid string) (*base.Transcript, base.TranscriptLayout) {
		m := transcriptModel(t, base.TranscriptConfig{})
		for _, source := range []string{"begin", mid, "end"} {
			transcriptAdd(t, m, transcriptBody(source))
		}
		transcriptCommit(t, transcriptScene(t, m), 20, 3, 1)
		return m, m.Layout()
	}
	a, one := layout("public")
	_, other := layout("different")
	start, _ := one.Hit(base.Point{})
	end, _ := one.Hit(base.Point{Row: 2, Col: 2})
	a.Selection().Set(base.TextRange{Start: start.Start, End: end.End})
	if got, err := a.Selection().Text(other); !errors.Is(err, base.ErrStaleSelection) {
		t.Errorf("foreign transcript accepted: %q %v", got, err)
	}
	if _, ok := other.Caret(start.Start); ok {
		t.Error("foreign caret resolved")
	}
}

func TestSelectionOwnsTheTranscriptPointerDrag(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, m, transcriptBody("word"))
	transcriptCommit(t, transcriptScene(t, m), 10, 1, 1)
	if !m.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}) || !m.Selection().Dragging() {
		t.Fatal("press did not begin selection")
	}
	m.Selection().Done()
	if m.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft}) || m.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft}) {
		t.Fatal("transcript consumed an already ended selection drag")
	}
}

func TestDocumentRegistrationPublishesOnlyTheSubscribedReference(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	d := transcriptDocument(t)
	armed := false
	if err := m.Element().SetFocusable(func() bool {
		if armed {
			armed = false
			panic("model observed")
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	r := transcriptScene(t, m)
	armed = true
	func() {
		defer func() {
			if recover() != "model observed" {
				t.Fatal("focus predicate did not run")
			}
		}()
		_, _ = m.AddDocument(d)
	}()
	if err := d.Append([]byte("new")); err != nil {
		t.Fatal(err)
	}
	transcriptPrepare(t, d)
	transcriptCommit(t, r, 10, 1, 1)
	if got := m.Rows(0, 1); len(got) != 1 || got[0].Text != "new" {
		t.Fatalf("retained reference was never subscribed: %v", got)
	}
}

func TestTranscriptObservesTheDocumentThatHasAlreadySettled(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	d := transcriptDocument(t)
	var item *base.TranscriptItem
	var states []base.ContentState
	stop := d.Subscribe(func(change content.DocumentChange) {
		if change.State == content.Final {
			states = append(states, item.State())
			if m.Block(item.ID()) != change.Body {
				t.Error("document and item disagree on the current body")
			}
		}
	})
	defer stop()
	var err error
	item, err = m.AddDocument(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.End(nil); err != nil {
		t.Fatal(err)
	}
	transcriptPrepare(t, d)
	if !slices.Equal(states, []base.ContentState{base.ContentRetained}) {
		t.Fatalf("earlier subscriber saw stale item state: %v", states)
	}
}

func TestTranscriptCaretSurvivesConsumedWrapWhitespace(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, m, transcriptBody("ab  cd"))
	root := transcriptScene(t, m)
	transcriptCommit(t, root, 12, 3, 1)
	wide := m.Layout()
	boundary, ok := wide.Hit(base.Point{Col: 2})
	if !ok {
		t.Fatal("gap has no source position")
	}
	space, _ := wide.Hit(base.Point{Col: 3})
	transcriptCommit(t, root, 3, 3, 2)
	narrow := m.Layout()
	for _, tc := range []struct {
		point base.TextPoint
		want  base.Point
	}{
		{boundary.Start, base.Point{Col: 2}},
		{boundary.End, base.Point{Col: 2}},
		{space.Start, base.Point{Row: 1}},
	} {
		if got, ok := narrow.Caret(tc.point); !ok || got != tc.want {
			t.Fatalf("caret for %+v = %+v,%v want %+v", tc.point.Position, got, ok, tc.want)
		}
	}
}
