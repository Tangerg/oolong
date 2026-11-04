package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestReadOnlyRefinementCannotPublishASecondScrollCandidate(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(*base.ScrollLayout)
	}{
		{name: "resize", run: func(candidate *base.ScrollLayout) { candidate.Resize(100) }},
		{name: "reveal", run: func(candidate *base.ScrollLayout) { candidate.Reveal(80, 80) }},
		{name: "anchor", run: func(candidate *base.ScrollLayout) { candidate.Anchor(80) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			var scroll base.Scroll
			armed := false
			var borrowed *base.ScrollLayout
			var element scene.Element
			element = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
				candidate := scroll.Stage(frame, 100, 10)
				borrowed = &candidate
				if !armed {
					return
				}
				before := candidate.Offset()
				element.ReadOnly(func() {
					defer func() {
						got, _ := recover().(error)
						if !errors.Is(got, scene.ErrBusy) {
							t.Errorf("read-only refinement=%v", got)
						}
					}()
					operation.run(&candidate)
				})
				if candidate.Offset() != before {
					t.Fatal("recovered refinement exposed an uncommitted second candidate")
				}
				candidate.Resize(20)
				if candidate.Offset() != before {
					t.Fatal("failed refinement leaked a reveal or anchor into later preparation")
				}
			}})
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			for sequence := uint64(1); sequence <= 2; sequence++ {
				candidate, err := root.Prepare(grid.NewSurface(8, 10).View())
				if err != nil {
					t.Fatal(err)
				}
				func() {
					defer func() {
						if recover() == nil {
							t.Error("expired ScrollLayout still exposed candidate state")
						}
					}()
					borrowed.Offset()
				}()
				candidate.Commit(sequence)
				if sequence == 1 {
					scroll.By(30)
					armed = true
				}
			}
			if scroll.Offset() != 30 {
				t.Fatal("frame committed geometry inconsistent with its recovered drawing")
			}
			scroll.Pages(1)
			if scroll.Offset() != 49 {
				t.Fatal("paging did not use the complete committed twenty-row window")
			}
		})
	}
}

func TestZeroTranscriptUsesReadingIntentSetBeforeMount(t *testing.T) {
	var transcript base.Transcript
	transcript.Scroll().ToTop()
	transcriptAdd(t, &transcript, transcriptBody("one\ntwo\nthree"))
	root := transcriptScene(t, &transcript)
	transcriptCommit(t, root, 10, 1, 1)
	if transcript.Scroll().FollowingEnd() || transcript.Scroll().Offset() != 0 {
		t.Fatal("initialization overwrote explicit reading intent")
	}
}

func TestScrollLayoutRefinementPreservesFollowIntent(t *testing.T) {
	for _, test := range []struct {
		name   string
		refine func(*base.ScrollLayout)
		offset int
	}{
		{name: "visible row", refine: func(candidate *base.ScrollLayout) { candidate.Reveal(95, 95) }, offset: 90},
		{name: "earlier row", refine: func(candidate *base.ScrollLayout) { candidate.Reveal(30, 30) }, offset: 30},
		{name: "resize after reveal", refine: func(candidate *base.ScrollLayout) {
			candidate.Reveal(95, 95)
			candidate.Resize(20)
		}, offset: 80},
		{name: "anchor cannot override following", refine: func(candidate *base.ScrollLayout) { candidate.Anchor(30) }, offset: 90},
	} {
		t.Run(test.name, func(t *testing.T) {
			var scroll base.Scroll
			scroll.ToBottom()
			total := 100
			refine := test.refine
			element := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
				candidate := scroll.Stage(frame, total, 10)
				if refine != nil {
					refine(&candidate)
				}
			}})
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			frame, err := root.Prepare(grid.NewSurface(8, 10).View())
			if err != nil {
				t.Fatal(err)
			}
			frame.Abort()
			if !scroll.FollowingEnd() || scroll.Offset() != 0 {
				t.Fatal("aborted refinement changed reading intent or committed position")
			}
			for sequence := uint64(1); sequence <= 2; sequence++ {
				frame, err = root.Prepare(grid.NewSurface(8, 10).View())
				if err != nil {
					t.Fatal(err)
				}
				frame.Commit(sequence)
				root.Wrote(sequence)
				if !scroll.FollowingEnd() {
					t.Fatal("candidate refinement changed following intent")
				}
				want := test.offset
				if sequence == 2 {
					want = 100
				}
				if scroll.Offset() != want {
					t.Fatalf("frame %d offset = %d, want %d", sequence, scroll.Offset(), want)
				}
				total, refine = 110, nil
			}
		})
	}
}
