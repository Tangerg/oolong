package base_test

import (
	"context"
	"errors"
	"image"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

func transcriptModel(tb testing.TB, cfg base.TranscriptConfig) *base.Transcript {
	tb.Helper()
	m, err := base.NewTranscript(cfg)
	if err != nil {
		tb.Fatal(err)
	}
	return m
}

func transcriptBody(source string) grid.Drawable {
	lines := strings.Split(source, "\n")
	styled := make([]text.Line, len(lines))
	for i, line := range lines {
		styled[i] = text.Of(line, grid.Style{})
	}
	return text.NewBlock(text.BlockConfig{Lines: styled, Wrap: true})
}

func transcriptAdd(tb testing.TB, m *base.Transcript, body grid.Drawable) *base.TranscriptItem {
	tb.Helper()
	item, err := m.Add(body)
	if err != nil {
		tb.Fatal(err)
	}
	return item
}

func transcriptScene(tb testing.TB, m *base.Transcript) *scene.Root {
	tb.Helper()
	r, err := scene.NewRoot(scene.RootConfig{Element: m.Element()})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(r.Close)
	return r
}

func transcriptCommit(tb testing.TB, r *scene.Root, width, height int, sequence uint64) *grid.Surface {
	tb.Helper()
	surface := grid.NewSurface(width, height)
	candidate, err := r.Prepare(surface.View())
	if err != nil {
		tb.Fatal(err)
	}
	candidate.Commit(sequence)
	r.Wrote(sequence)
	return surface
}

func transcriptDocument(tb testing.TB) *content.StreamDocument {
	tb.Helper()
	d, err := content.NewStreamDocument(content.StreamConfig{Format: "text"})
	if err != nil {
		tb.Fatal(err)
	}
	return d
}

func transcriptPrepare(tb testing.TB, d *content.StreamDocument) {
	tb.Helper()
	if err := d.Accept(d.Snapshot().Prepare(context.Background(), func(_ context.Context, source string) (grid.Drawable, error) { return transcriptBody(source), nil })); err != nil {
		tb.Fatal(err)
	}
}

func TestTranscriptFrameAbortPreservesGeometryWithoutRollingBackContent(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	first := transcriptAdd(t, m, transcriptBody("one\ntwo"))
	r := transcriptScene(t, m)
	transcriptCommit(t, r, 20, 4, 1)
	if m.Height() != 2 {
		t.Fatal(m.Height())
	}
	second := transcriptAdd(t, m, transcriptBody("three\nfour"))
	candidate, err := r.Prepare(grid.NewSurface(8, 4).View())
	if err != nil {
		t.Fatal(err)
	}
	if m.Height() != 2 || m.Width() != 20 {
		t.Fatal("candidate advanced geometry")
	}
	candidate.Abort()
	if m.Len() != 2 || m.Block(second.ID()) == nil {
		t.Fatal("abort rolled back model facts")
	}
	if m.Height() != 2 || m.Width() != 20 {
		t.Fatal("abort changed committed geometry")
	}
	transcriptCommit(t, r, 8, 4, 2)
	if m.Height() != 4 || m.Width() != 8 {
		t.Fatal("complete frame did not settle geometry")
	}
	if top, rows, ok := m.Extent(first.ID()); !ok || top != 0 || rows != 2 {
		t.Fatalf("extent=%d,%d,%v", top, rows, ok)
	}
}

func TestTranscriptOwnsOneCoordinateProjectionAcrossRowsAndDrawing(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{MaxItems: 200})
	for i := range 100 {
		transcriptAdd(t, m, transcriptBody(strings.Repeat("x\n", i%4)+"last"))
	}
	r := transcriptScene(t, m)
	transcriptCommit(t, r, 20, 10, 1)
	projection := m.Layout()
	for row := projection.StartRow(); row < projection.EndRow(); row++ {
		id, offset, ok := projection.At(row)
		if !ok {
			t.Fatalf("missing row %d", row)
		}
		top, height, exists := projection.Extent(id)
		if !exists || offset != row-top || offset >= height {
			t.Fatal("coordinates have competing owners")
		}
		canvas := grid.NewSurface(20, 1)
		projection.Draw(canvas.View(), row)
		meaningful := projection.Rows(row, 1)
		if len(meaningful) != 1 {
			t.Fatal("row projection lost physical count")
		}
		var got strings.Builder
		for x := range 20 {
			cell, _ := canvas.CellAt(x, 0)
			got.WriteString(cell.Content())
		}
		if strings.TrimRight(got.String(), " ") != meaningful[0].Text {
			t.Fatalf("draw and copy differ at %d", row)
		}
	}
}

func TestTranscriptAutomaticallySubscribesFinalizesAndRetiresAfterAdmission(t *testing.T) {
	p, err := publication.New(publication.Config{Width: 20, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	m := transcriptModel(t, base.TranscriptConfig{Publisher: p, Retention: base.PublishCompleted})
	d := transcriptDocument(t)
	item, err := m.AddDocument(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append([]byte("answer")); err != nil {
		t.Fatal(err)
	}
	transcriptPrepare(t, d)
	if m.Len() != 1 || item.State() != base.ContentLive {
		t.Fatal("provisional document retired")
	}
	if err := d.End(nil); err != nil {
		t.Fatal(err)
	}
	if m.Len() != 1 {
		t.Fatal("EOF without final parse retired content")
	}
	transcriptPrepare(t, d)
	if m.Len() != 0 || item.State() != base.ContentRetired || item.Receipt().Snapshot().State != publication.Accepted {
		t.Fatal("accepted final content did not retire")
	}
	next := transcriptAdd(t, m, transcriptBody("next"))
	if m.Len() != 1 || next.State() == base.ContentRetired {
		t.Fatal("full publisher lost caller content")
	}
	p.Commit(p.Prepare(), 1)
	p.Written(1)
	if m.Len() != 1 {
		t.Fatal("writer settlement called subscriber in the commit phase")
	}
	p.FlushNotifications()
	if m.Len() != 0 || next.State() != base.ContentRetired {
		t.Fatal("capacity notification did not resume composition")
	}
	m.Close()
}

func TestTranscriptRetainsPartialUntilPolicyExplicitlyPermitsIt(t *testing.T) {
	for _, publishPartial := range []bool{false, true} {
		p, err := publication.New(publication.Config{Width: 20})
		if err != nil {
			t.Fatal(err)
		}
		m := transcriptModel(t, base.TranscriptConfig{Publisher: p, Retention: base.PublishCompleted, PublishPartial: publishPartial})
		d := transcriptDocument(t)
		item, err := m.AddDocument(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Append([]byte("useful partial")); err != nil {
			t.Fatal(err)
		}
		cause := errors.New("source failed")
		if err := d.End(cause); err != nil {
			t.Fatal(err)
		}
		transcriptPrepare(t, d)
		if (item.State() == base.ContentRetired) != publishPartial || !errors.Is(item.Err(), cause) {
			t.Fatalf("partial state=%v error=%v", item.State(), item.Err())
		}
		m.Close()
	}
}

type transcriptHuge struct{ rows int }

func (b transcriptHuge) HeightForWidth(int) int { return b.rows }
func (transcriptHuge) Draw(grid.View)           {}
func TestTranscriptOverflowCannotPublishPartialGeometry(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, m, transcriptHuge{rows: 1})
	r := transcriptScene(t, m)
	transcriptCommit(t, r, 20, 1, 1)
	transcriptAdd(t, m, transcriptHuge{rows: math.MaxInt})
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("overflow was accepted")
			}
		}()
		_, _ = r.Prepare(grid.NewSurface(20, 1).View())
	}()
	if m.Height() != 1 || m.Len() != 2 {
		t.Fatal("failed layout changed committed coordinates or source")
	}
}

func TestTranscriptRejectsMutationBeforeChangingStateDuringPaint(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	if err := m.Element().SetPaint(func(scene.Frame) { _, _ = m.Add(transcriptBody("illegal")) }); err != nil {
		t.Fatal(err)
	}
	r := transcriptScene(t, m)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("mutation during paint was allowed")
			}
		}()
		_, _ = r.Prepare(grid.NewSurface(10, 1).View())
	}()
	if m.Len() != 0 {
		t.Fatal("purity rejection happened after mutation")
	}
}

func TestLogicalSelectionSurvivesWrappingAndCopiesSafeText(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, m, transcriptBody("alpha 中👩‍💻 bravo charlie"))
	r := transcriptScene(t, m)
	transcriptCommit(t, r, 40, 4, 1)
	before := m.Layout()
	selection := m.Selection()
	if !selection.SelectWord(before, base.Point{Row: 0, Col: 14}) {
		t.Fatal("word not found")
	}
	want, err := selection.Text(before)
	if err != nil {
		t.Fatal(err)
	}
	transcriptCommit(t, r, 8, 8, 2)
	got, err := selection.Text(m.Layout())
	if err != nil || got != want {
		t.Fatalf("after resize text=%q err=%v, want %q", got, err, want)
	}
	rects, err := selection.Rects(m.Layout())
	if err != nil || len(rects) == 0 {
		t.Fatal("logical range lost its visual projection")
	}
	for _, rect := range rects {
		if rect.Min.X < 0 || rect.Max.X > 8 {
			t.Fatalf("unclipped range=%v", rect)
		}
	}
}

func TestLogicalSelectionDirectionAndRevisionBelongToOneOwner(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	d := transcriptDocument(t)
	_, err := m.AddDocument(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append([]byte("one two three")); err != nil {
		t.Fatal(err)
	}
	transcriptPrepare(t, d)
	r := transcriptScene(t, m)
	transcriptCommit(t, r, 30, 1, 1)
	a, ok := m.Layout().Hit(base.Point{Row: 0, Col: 8})
	if !ok {
		t.Fatal("hit")
	}
	b, ok := m.Layout().Hit(base.Point{Row: 0, Col: 4})
	if !ok {
		t.Fatal("hit")
	}
	selection := m.Selection()
	selection.Begin(a.End)
	selection.Extend(b.Start)
	first, last := selection.Range()
	if !reflect.DeepEqual(selection.Anchor(), a.End) || first.Position.Byte != 4 || last.Position.Byte != 9 {
		t.Fatal("range sorting moved anchor")
	}
	if err := d.Append([]byte(" changed")); err != nil {
		t.Fatal(err)
	}
	transcriptPrepare(t, d)
	if selection.Active() {
		t.Fatal("semantic replacement retained old anchors")
	}
}

func TestTranscriptPointerUsesArrivalFrameLocalProjection(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, m, transcriptBody("first\nsecond\nthird\nfourth"))
	m.Scroll().ToTop()
	r := transcriptScene(t, m)
	transcriptCommit(t, r, 20, 2, 1)
	captured := r.CaptureInput(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)})
	m.Scroll().By(1)
	candidate, err := r.Prepare(grid.NewSurface(20, 2).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Commit(2)
	if !r.HandleInput(captured) {
		t.Fatal("old written click was refused")
	}
	got, err := m.Selection().Text(m.Layout())
	if err != nil || got != "f" {
		t.Fatalf("old click selected %q: %v", got, err)
	}
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonRight})
	if !m.Selection().Dragging() {
		t.Fatal("foreign release ended the gesture")
	}
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(999, 999), At: time.Now()})
	if m.Selection().Dragging() {
		t.Fatal("capture did not settle outside viewport")
	}
}

func TestTranscriptSelectionUsesItsConfiguredClickInterval(t *testing.T) {
	for _, test := range []struct {
		name     string
		interval time.Duration
		want     string
	}{
		{name: "default", want: "alpha"},
		{name: "short", interval: 10 * time.Millisecond, want: "l"},
		{name: "negative uses default", interval: -time.Second, want: "alpha"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := transcriptModel(t, base.TranscriptConfig{})
			transcriptAdd(t, model, transcriptBody("alpha beta"))
			started := time.Unix(100, 0)
			root, err := scene.NewRoot(scene.RootConfig{Element: model.Element(), Now: func() time.Time { return started }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			transcriptCommit(t, root, 20, 1, 1)
			model.Selection().SetClickInterval(test.interval)
			for _, elapsed := range []time.Duration{0, 50 * time.Millisecond} {
				press := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 0), At: started.Add(elapsed)}
				if !root.Handle(press) {
					t.Fatal("presented text refused the click")
				}
				press.Action = input.MouseUp
				press.At = press.At.Add(time.Millisecond)
				root.Handle(press)
			}
			got, err := model.Selection().Text(model.Layout())
			if err != nil || got != test.want {
				t.Fatalf("selected %q, want %q: %v", got, test.want, err)
			}
		})
	}
}

func TestTranscriptSearchCopiesOnlyVisibleMatches(t *testing.T) {
	m := transcriptModel(t, base.TranscriptConfig{})
	matches := make([]base.Match, 10000)
	for i := range matches {
		matches[i] = base.Match{Row: i, Spans: []base.Span{{Width: 1}}}
	}
	m.SetMatches(matches)
	first, visible := m.MatchesIn(9000, 9010)
	if first != 9000 || len(visible) != 10 {
		t.Fatalf("visible=%d,%d", first, len(visible))
	}
	visible[0].Spans[0].Width = 999
	_, again := m.MatchesIn(9000, 9001)
	if again[0].Spans[0].Width != 1 {
		t.Fatal("read projection advanced match owner")
	}
}

func TestTranscriptRowsPreserveSlotsForNontextContent(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, transcriptBody("before"))
	transcriptAdd(t, model, &plain{lines: 2})
	transcriptAdd(t, model, transcriptBody("after"))
	root := transcriptScene(t, model)
	surface := transcriptCommit(t, root, 20, 4, 1)
	if got, want := rowTexts(model.Rows(0, 4)), []string{"before", "", "", "after"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("text projection=%q want=%q", got, want)
	}
	if rowText(surface.View(), 3) != "after" {
		t.Fatal("nontext content changed following row coordinates")
	}
}

func TestTranscriptKeepsRecentCompletedContentAndResumesOlderPrefix(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	model := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted, KeepCompleted: 2, MaxItems: 5})
	t.Cleanup(model.Close)
	first := transcriptAdd(t, model, transcriptBody("first"))
	second := transcriptAdd(t, model, transcriptBody("second"))
	if model.Len() != 2 || first.State() != base.ContentRetained || second.State() != base.ContentRetained {
		t.Fatal("recent completed content left the interactive surface")
	}
	third := transcriptAdd(t, model, transcriptBody("third"))
	if model.Len() != 2 || first.State() != base.ContentRetired || second.State() != base.ContentRetained {
		t.Fatal("only the oldest completed item should transfer")
	}
	fourth := transcriptAdd(t, model, transcriptBody("fourth"))
	if model.Len() != 3 || second.State() != base.ContentRetained || fourth.State() != base.ContentRetained {
		t.Fatal("publication backpressure discarded the remaining prefix")
	}
	publisher.Commit(publisher.Prepare(), 1)
	publisher.Written(1)
	publisher.FlushNotifications()
	if model.Len() != 2 || second.State() != base.ContentRetired || third.State() != base.ContentRetained || fourth.State() != base.ContentRetained {
		t.Fatal("capacity notification did not restore the recent-item bound")
	}
}

func TestTranscriptRecentRetentionNeverPublishesPastAnIncompletePrefix(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20})
	if err != nil {
		t.Fatal(err)
	}
	model := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted, KeepCompleted: 1})
	t.Cleanup(model.Close)
	document := transcriptDocument(t)
	first, err := model.AddDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	second := transcriptAdd(t, model, transcriptBody("second"))
	third := transcriptAdd(t, model, transcriptBody("third"))
	if items, _ := publisher.Pending(); items != 0 {
		t.Fatal("complete suffix overtook an incomplete source")
	}
	if err := document.Append([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := document.End(nil); err != nil {
		t.Fatal(err)
	}
	transcriptPrepare(t, document)
	if model.Len() != 1 || first.State() != base.ContentRetired || second.State() != base.ContentRetired || third.State() != base.ContentRetained {
		t.Fatal("final preparation did not retire the eligible leading run")
	}
}

func TestTranscriptRecentRetentionLeavesRoomForNewSources(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []base.TranscriptConfig{
		{KeepCompleted: -1},
		{KeepCompleted: 1},
		{Publisher: publisher, Retention: base.PublishCompleted, MaxItems: 2, KeepCompleted: 2},
	} {
		if _, err := base.NewTranscript(cfg); err == nil {
			t.Fatalf("accepted impossible retention policy: %+v", cfg)
		}
	}
}
