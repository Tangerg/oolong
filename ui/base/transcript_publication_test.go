package base_test

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type transcriptCaptureBlock func()

func (transcriptCaptureBlock) HeightForWidth(int) int { return 1 }

func (b transcriptCaptureBlock) Draw(view grid.View) {
	view.Text(0, 0, "content", grid.Style{})
	if b != nil {
		b()
	}
}

func TestTranscriptCaptureRejectsEveryOwnedReadingMutation(t *testing.T) {
	for _, attachment := range []string{"detached", "mounted"} {
		for _, operation := range []string{"append", "close", "scroll", "wheel", "selection", "click interval", "sticky", "stage transcript", "stage scroll"} {
			t.Run(attachment+"/"+operation, func(t *testing.T) {
				publisher, err := publication.New(publication.Config{Width: 20})
				if err != nil {
					t.Fatal(err)
				}
				model := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
				t.Cleanup(model.Close)
				if attachment == "mounted" {
					transcriptScene(t, model)
				}
				model.Scroll().ToTop()
				anchor := base.TextPoint{Content: model.FirstBlock(), Revision: 1}
				model.Selection().Begin(anchor)
				model.Sticky().Add(anchor.Content)
				mutate := func() {
					switch operation {
					case "append":
						_, _ = model.Add(transcriptBody("unexpected"))
					case "close":
						model.Close()
					case "scroll":
						model.Scroll().ToBottom()
					case "wheel":
						model.Scroll().Wheel(input.Wheel{})
					case "selection":
						model.Selection().Clear()
					case "click interval":
						model.Selection().SetClickInterval(time.Second)
					case "sticky":
						model.Sticky().SetBlocks(nil)
					case "stage transcript", "stage scroll":
						foreign, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
							if operation == "stage transcript" {
								model.Stage(frame, 10)
							} else {
								model.Scroll().Stage(frame, 10, 1)
							}
						}})})
						if err != nil {
							panic(err)
						}
						defer foreign.Close()
						candidate, err := foreign.Prepare(grid.NewSurface(10, 1).View())
						if err != nil {
							panic(err)
						}
						candidate.Commit(1)
					}
				}
				body := transcriptCaptureBlock(func() {
					if mutate != nil {
						mutate()
					}
				})
				func() {
					defer func() {
						if got, ok := recover().(error); !ok || !errors.Is(got, scene.ErrBusy) {
							t.Errorf("capture mutation panic=%v", got)
						}
					}()
					_, _ = model.Add(body)
				}()
				if model.Len() != 1 || model.Block(anchor.Content) == nil || model.Scroll().FollowingEnd() ||
					!model.Selection().Active() || model.Selection().Anchor() != anchor || model.Sticky().Len() != 1 {
					t.Fatal("capture changed retained content or its reading owners before rejection")
				}
				if items, _ := publisher.Pending(); items != 0 {
					t.Fatal("failed capture transferred publication responsibility")
				}
				mutate = nil
				next := transcriptAdd(t, model, transcriptBody("next"))
				if model.Len() != 0 || next.State() != base.ContentRetired {
					t.Fatal("capture failure prevented a later valid publication")
				}
			})
		}
	}
}

type reflowedCaptureBlock struct {
	measurements int
	onDraw       func()
}

func (b *reflowedCaptureBlock) HeightForWidth(width int) int {
	b.measurements++
	if width < 20 {
		return 4
	}
	return 2
}

func (b *reflowedCaptureBlock) Draw(view grid.View) {
	view.Text(0, 0, "first", grid.Style{})
	if b.onDraw != nil {
		b.onDraw()
	}
}

func TestTranscriptCaptureCannotReplaceItsCommittedReadingProjection(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.TryPublish(transcriptBody("occupied")); err != nil {
		t.Fatal(err)
	}
	model := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
	t.Cleanup(model.Close)
	body := &reflowedCaptureBlock{}
	first := transcriptAdd(t, model, body)
	suffix := transcriptAdd(t, model, transcriptBody("live"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 20, 1, 1)
	model.Selection().Begin(base.TextPoint{Content: first.ID(), Revision: 1})
	body.onDraw = func() {
		foreign, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			projection := model.Stage(frame, 10)
			model.Scroll().Stage(frame, projection.Height(), 1)
		}})})
		if err != nil {
			panic(err)
		}
		defer foreign.Close()
		defer func() {
			got, _ := recover().(error)
			if !errors.Is(got, scene.ErrBusy) {
				t.Errorf("capture replaced its frame projection: panic=%v", got)
			}
		}()
		candidate, err := foreign.Prepare(grid.NewSurface(10, 1).View())
		if err != nil {
			panic(err)
		}
		candidate.Commit(1)
	}
	body.measurements = 0
	publisher.Commit(publisher.Prepare(), 1)
	publisher.Written(1)
	publisher.FlushNotifications()
	if body.measurements != 1 {
		t.Fatal("foreign projection called measurement before refusing its owner")
	}
	if first.State() != base.ContentRetired || suffix.State() != base.ContentRetained || model.Len() != 1 || model.Selection().Active() {
		t.Fatal("refusing foreign geometry prevented accepted-prefix settlement")
	}
	if model.Scroll().Offset() != 0 || model.Width() != 20 || model.Height() != 3 {
		t.Fatal("retirement read a geometry owner replaced from capture")
	}
	transcriptCommit(t, root, 20, 1, 2)
	if model.StartRow() != 2 || model.Height() != 1 || model.EndRow() != 3 {
		t.Fatal("later valid frame lost the accepted prefix's coordinate origin")
	}
}

func TestTranscriptRetirementSurvivesCaptureAbnormalExit(t *testing.T) {
	for _, exit := range []string{"panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			publisher, err := publication.New(publication.Config{Width: 20, MaxItems: 2})
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err := publisher.TryPublish(transcriptBody("occupied")); err != nil {
					t.Fatal(err)
				}
			}
			model := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
			t.Cleanup(model.Close)
			first := transcriptAdd(t, model, transcriptBody("first\nsecond"))
			fail := false
			second := transcriptAdd(t, model, transcriptCaptureBlock(func() {
				if fail {
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic("capture interrupted")
				}
			}))
			third := transcriptAdd(t, model, transcriptBody("last"))
			root := transcriptScene(t, model)
			transcriptCommit(t, root, 20, 1, 1)
			span, ok := model.Layout().Hit(base.Point{})
			if !ok {
				t.Fatal("first content has no selectable text")
			}
			model.Selection().Set(span)
			model.Sticky().Add(first.ID())
			publisher.Commit(publisher.Prepare(), 1)
			publisher.Written(1)
			fail = true
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					got := recover()
					if exit == "panic" && got != "capture interrupted" || exit == "Goexit" && got != nil {
						t.Errorf("capture exit=%v", got)
					}
				}()
				publisher.FlushNotifications()
				t.Error("abnormal capture returned")
			}()
			<-done
			if first.State() != base.ContentRetired || first.Receipt().Snapshot().State != publication.Accepted ||
				second.State() != base.ContentRetained || third.State() != base.ContentRetained || model.Len() != 2 {
				t.Fatal("abnormal capture undid an accepted prefix or retired unaccepted content")
			}
			if model.Block(first.ID()) != nil || model.FirstBlock() != second.ID() ||
				model.Selection().Active() || model.Sticky().Len() != 0 || model.Scroll().Offset() != 1 {
				t.Fatal("accepted prefix did not settle its original reading owners")
			}
			if items, _ := publisher.Pending(); items != 1 {
				t.Fatalf("capture failure published %d items, want the accepted prefix only", items)
			}
			fail = false
			transcriptCommit(t, root, 20, 1, 2)
			if model.StartRow() != 2 || model.Height() != 2 || model.EndRow() != 4 {
				t.Fatal("replacement frame lost the settled prefix origin")
			}
			publisher.Commit(publisher.Prepare(), 2)
			publisher.Written(2)
			publisher.FlushNotifications()
			if first.Receipt().Snapshot().State != publication.Written || model.Len() != 0 ||
				second.State() != base.ContentRetired || third.State() != base.ContentRetired {
				t.Fatal("safe pre-admission failure could not resume the remaining suffix")
			}
			pending := publisher.Prepare().Items()
			if len(pending) != 2 || pending[0].ID != second.Receipt().Snapshot().ID || pending[1].ID != third.Receipt().Snapshot().ID {
				t.Fatal("retry duplicated or reordered the accepted prefix")
			}
		})
	}
}
