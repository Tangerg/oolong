package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestStickyOverlayKeepsTheFullScrollViewport(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	first := transcriptAdd(t, model, transcriptBody("a0\na1\na2"))
	transcriptAdd(t, model, transcriptBody("tail0\ntail1\ntail2\ntail3"))
	second := transcriptAdd(t, model, transcriptBody("b0\nb1\nb2"))
	transcriptAdd(t, model, transcriptBody("end0\nend1"))
	model.Sticky().SetBlocks([]base.BlockID{first.ID(), second.ID()})
	var bodyFrom int
	var pinned base.Pinned
	if err := model.Element().SetPaint(func(frame scene.Frame) {
		window := model.StageWindow(frame)
		bodyFrom, pinned = window.BodyFrom, window.Pinned
		window.Content.Draw(frame.Sub(window.Header).View, window.HeaderFrom)
		window.Content.Draw(window.Body, window.BodyFrom)
	}); err != nil {
		t.Fatal(err)
	}
	root := transcriptScene(t, model)
	surface := transcriptCommit(t, root, 12, 6, 1)
	if model.Scroll().Offset() != 6 || bodyFrom != 7 || pinned.Block != first.ID() || pinned.Rows != 1 {
		t.Fatalf("viewport offset=%d body=%d pinned=%+v", model.Scroll().Offset(), bodyFrom, pinned)
	}
	if rowText(surface.View(), 0) != "a2" || rowText(surface.View(), 1) != "b0" || rowText(surface.View(), 5) != "end1" {
		t.Fatal("overlay did not preserve the next header position and visible end")
	}
	if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 1)}) {
		t.Fatal("body click was not routed")
	}
	if got, err := model.Selection().Text(model.Layout()); err != nil || got != "b" {
		t.Fatalf("body selection=%q, %v", got, err)
	}
	model.Selection().Clear()
	model.Scroll().Pages(-1)
	if got := model.Scroll().Offset(); got != 1 {
		t.Fatalf("page used the shortened body: got %d, want 1", got)
	}
	transcriptCommit(t, root, 12, 6, 2)
	anchor, ok := model.ReadingAnchor()
	point, valid := model.Layout().Caret(anchor.Point)
	if !ok || !valid || point.Row != bodyFrom || anchor.ViewportOffset != pinned.Rows {
		t.Fatalf("reading anchor did not identify the visible body: %+v, %+v", anchor, point)
	}
	model.Scroll().ToBottom()
	surface = transcriptCommit(t, root, 12, 4, 3)
	if model.Scroll().Offset() != 8 || !model.Scroll().FollowingEnd() || rowText(surface.View(), 3) != "end1" {
		t.Fatal("resize changed following intent or hid the final row")
	}
	model.Scroll().ToTop()
	model.Scroll().By(7)
	surface = transcriptCommit(t, root, 12, 4, 4)
	if pinned.Rows != 0 || bodyFrom != 7 || rowText(surface.View(), 0) != "b0" {
		t.Fatal("the new header did not replace the overlay at the viewport boundary")
	}
}

func TestStickyReadingAnchorSurvivesReflowInViewportCoordinates(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	header := transcriptAdd(t, model, transcriptBody("a"))
	transcriptAdd(t, model, transcriptBody("abcdefghij\nklmnopqrst\nuvwxyz0123\n456789abcd\nefghijklmn\nopqrstuvwx"))
	model.Sticky().Add(header.ID())
	model.Sticky().SetGap(1)
	model.Scroll().ToTop()
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 10, 4, 1)
	model.Scroll().By(2)
	transcriptCommit(t, root, 10, 4, 2)
	before, ok := model.ReadingAnchor()
	if !ok || before.ViewportOffset != 2 {
		t.Fatalf("reading anchor before reflow=%+v", before)
	}
	transcriptCommit(t, root, 5, 4, 3)
	after, ok := model.ReadingAnchor()
	if !ok || after.Point != before.Point || after.ViewportOffset != before.ViewportOffset {
		t.Fatalf("reflow changed visible reading position: before=%+v after=%+v", before, after)
	}
	if got := model.Scroll().Offset(); got != 5 {
		t.Fatalf("reflow viewport offset=%d, want 5", got)
	}
}

func TestStickyReadingAnchorTracksReflowedHeaderGeometry(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		width, nextWidth, offset, nextOffset int
		pinnedRows, nextPinnedRows           int
		pushOff                              bool
	}{
		{name: "growing header", width: 6, nextWidth: 3, offset: 3, nextOffset: 7, pinnedRows: 2, nextPinnedRows: 3},
		{name: "shrinking header", width: 3, nextWidth: 6, offset: 7, nextOffset: 3, pinnedRows: 3, nextPinnedRows: 2},
		{name: "growing header during push off", width: 6, nextWidth: 3, offset: 3, nextOffset: 7, pinnedRows: 1, nextPinnedRows: 1, pushOff: true},
		{name: "shrinking header during push off", width: 3, nextWidth: 6, offset: 7, nextOffset: 3, pinnedRows: 1, nextPinnedRows: 1, pushOff: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := transcriptModel(t, base.TranscriptConfig{})
			header := transcriptAdd(t, model, transcriptBody("abcdef"))
			model.Sticky().Add(header.ID())
			if tc.pushOff {
				transcriptAdd(t, model, transcriptBody("ghijkl\nmnopqr\nstuvwx"))
				next := transcriptAdd(t, model, transcriptBody("UVWXYZ"))
				model.Sticky().Add(next.ID())
			}
			transcriptAdd(t, model, transcriptBody("ghijkl\nmnopqr\nstuvwx\nyz0123\n456789\nABCDEF\nGHIJKL\nMNOPQR"))
			model.Sticky().SetGap(1)
			model.Scroll().ToTop()
			var bodyFrom, bodyTop int
			if err := model.Element().SetPaint(func(frame scene.Frame) {
				window := model.StageWindow(frame)
				bodyFrom, bodyTop = window.BodyFrom, window.Pinned.Rows
				window.Content.Draw(frame.Sub(window.Header).View, window.HeaderFrom)
				window.Content.Draw(window.Body, window.BodyFrom)
			}); err != nil {
				t.Fatal(err)
			}
			root := transcriptScene(t, model)
			transcriptCommit(t, root, tc.width, 5, 1)
			model.Scroll().By(tc.offset)
			transcriptCommit(t, root, tc.width, 5, 2)
			before, ok := model.ReadingAnchor()
			if !ok || before.FollowingEnd || before.ViewportOffset != tc.pinnedRows {
				t.Fatalf("anchor before reflow=%+v", before)
			}
			surface := transcriptCommit(t, root, tc.nextWidth, 5, 3)
			after, ok := model.ReadingAnchor()
			if !ok || after.Point != before.Point || after.ViewportOffset != tc.nextPinnedRows {
				t.Fatalf("header reflow moved the logical reading position: before=%+v after=%+v", before, after)
			}
			point, valid := model.Layout().Caret(before.Point)
			if !valid || point.Row != bodyFrom || bodyTop != tc.nextPinnedRows || rowText(surface.View(), bodyTop) != model.Layout().Rows(point.Row, 1)[0].Content.String() {
				t.Fatalf("anchor is not the first visible body row: point=%+v body=%d+%d", point, bodyFrom, bodyTop)
			}
			if got := model.Scroll().Offset(); got != tc.nextOffset {
				t.Fatalf("reflow viewport offset=%d, want %d", got, tc.nextOffset)
			}
		})
	}
}

func TestTranscriptRevealKeepsMatchesBelowStickyFurniture(t *testing.T) {
	for _, tc := range []struct {
		name, header, body, nextHeader, tail, query string
		gap, collapse, height, nextHeight           int
	}{
		{name: "upward", header: "head", body: "a\nb\nc\nhit\ne\nf\ng\nh\ni\nj", query: "hit", height: 4},
		{name: "gap", header: "head", body: "a\nb\nc\nhit\ne\nf\ng\nh\ni\nj", query: "hit", gap: 1, height: 4},
		{name: "first body row", header: "head", body: "hit\nb\nc\nd\ne\nf\ng\nh\ni\nj", query: "hit", gap: 1, height: 4},
		{name: "inside collapsed header", header: "h0\nh1\nh2\nh3\nhit\nh5", body: "a\nb\nc\nd\ne\nf\ng\nh", query: "hit", collapse: 1, height: 4},
		{name: "push off", header: "a0\na1\na2", body: "b0\nb1\nb2\nhit", nextHeader: "next", tail: "t0\nt1\nt2\nt3\nt4\nt5", query: "hit", height: 5},
		{name: "whole match", header: "head", body: "a\nb\nhit\nalso\ne\nf\ng\nh\ni\nj", query: "hit\nalso", gap: 1, height: 4},
		{name: "pending resize", header: "h0\nh1\nh2\nh3\nh4", body: "a\nb\nc\nhit\ne\nf\ng\nh\ni\nj", query: "hit", height: 4, nextHeight: 8},
	} {
		for _, request := range []string{"match", "scroll"} {
			t.Run(tc.name+"/"+request, func(t *testing.T) {
				model := transcriptModel(t, base.TranscriptConfig{})
				header := transcriptAdd(t, model, transcriptBody(tc.header))
				transcriptAdd(t, model, transcriptBody(tc.body))
				model.Sticky().Add(header.ID())
				if tc.nextHeader != "" {
					next := transcriptAdd(t, model, transcriptBody(tc.nextHeader))
					model.Sticky().Add(next.ID())
					transcriptAdd(t, model, transcriptBody(tc.tail))
				}
				model.Sticky().SetGap(tc.gap)
				model.Sticky().SetMinHeight(tc.collapse)
				var bodyFrom, bodyTop int
				if err := model.Element().SetPaint(func(frame scene.Frame) {
					window := model.StageWindow(frame)
					bodyFrom, bodyTop = window.BodyFrom, window.Pinned.Rows
					window.Content.Draw(frame.Sub(window.Header).View, window.HeaderFrom)
					window.Content.Draw(window.Body, window.BodyFrom)
				}); err != nil {
					t.Fatal(err)
				}
				root := transcriptScene(t, model)
				transcriptCommit(t, root, 12, tc.height, 1)
				source := model.Layout()
				result, err := base.Scan(t.Context(), base.ScanConfig{
					Query: tc.query, Rows: source.Rows(source.StartRow(), source.Height()), StartRow: source.StartRow(),
				})
				if err != nil || len(result.Matches) != 1 {
					t.Fatalf("scan=%+v, %v", result, err)
				}
				match := result.Matches[0]
				if request == "match" {
					if err := model.SetMatches(source, result.Matches); err != nil {
						t.Fatal(err)
					}
					if !model.RevealMatch(0) {
						t.Fatal("current match was refused")
					}
				} else {
					first := match.Row - source.StartRow()
					model.Scroll().Reveal(first, first+len(match.Spans)-1)
				}
				height := tc.height
				if tc.nextHeight > 0 {
					height = tc.nextHeight
				}
				surface := transcriptCommit(t, root, 12, height, 2)
				for i, row := range source.Rows(match.Row, len(match.Spans)) {
					y := bodyTop + match.Row + i - bodyFrom
					if y < bodyTop || y >= height || rowText(surface.View(), y) != row.Content.String() {
						t.Fatalf("revealed row %d is obscured: viewport=%d body=%d+%d targetY=%d", match.Row+i, model.Scroll().Offset(), bodyFrom, bodyTop, y)
					}
				}
			})
		}
	}
}

func TestTranscriptRevealUsesTheCandidateWidthForStickyGeometry(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	header := transcriptAdd(t, model, transcriptBody("abcdefghijkl"))
	transcriptAdd(t, model, transcriptBody("0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11"))
	model.Sticky().Add(header.ID())
	model.Sticky().SetGap(1)
	var bodyFrom, bodyTop int
	if err := model.Element().SetPaint(func(frame scene.Frame) {
		window := model.StageWindow(frame)
		bodyFrom, bodyTop = window.BodyFrom, window.Pinned.Rows
		window.Content.Draw(frame.Sub(window.Header).View, window.HeaderFrom)
		window.Content.Draw(window.Body, window.BodyFrom)
	}); err != nil {
		t.Fatal(err)
	}
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 6, 1)
	const target = 6
	model.Scroll().Reveal(target, target)
	surface := transcriptCommit(t, root, 4, 6, 2)
	y := bodyTop + target - bodyFrom
	if y < bodyTop || y >= 6 || rowText(surface.View(), y) != model.Layout().Rows(target, 1)[0].Content.String() {
		t.Fatalf("candidate-width reveal is obscured: body=%d+%d targetY=%d", bodyFrom, bodyTop, y)
	}
}
