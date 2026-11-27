package program

import (
	"bytes"
	"context"
	"image"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

type controlledFrameOutput struct {
	bytes.Buffer
	started chan struct{}
	release chan struct{}
}

func (s *controlledFrameOutput) Write(frame []byte) (int, error) {
	s.started <- struct{}{}
	<-s.release
	return s.Buffer.Write(frame)
}

func TestModalInputUsesCompletedWriteBeforeOwnerNotification(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var answers []input.Event
		prompt := scene.NewElement(scene.ElementConfig{
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { frame.Text(0, 0, "Approve?", grid.Style{}) },
			Handle: func(event scene.Event) scene.EventResult {
				answers = append(answers, event.Input())
				return scene.EventResult{Consumed: true}
			},
		})
		modal := scene.NewGroup(scene.GroupConfig{Modal: true, Overlay: true, Items: []scene.Item{{Element: prompt}}})
		root, err := scene.NewRoot(scene.RootConfig{Element: modal.Element()})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		output := &controlledFrameOutput{started: make(chan struct{}, 1), release: make(chan struct{})}
		writer := term.NewWriter(term.WriterConfig{Destination: output})
		p := &program{root: root, screen: grid.NewScreen(8, 1), writer: writer, tasks: newTaskQueue(), ctx: ctx, cancel: cancel}
		source := &pipeSource{events: make(chan input.Event, 1)}
		p.inputPipe = newInputPipe(ctx, root, source, source.events, writer.WrittenAt, Config{})
		t.Cleanup(func() {
			p.stop()
			close(output.release)
			if err := writer.Close(); err != nil {
				t.Error(err)
			}
		})
		due := deadline.NewTimer()
		defer due.Stop()
		deliver := func(event input.Event) {
			t.Helper()
			source.events <- event
			if stopped, err := p.wait(ctx, due); stopped || err != nil {
				t.Fatalf("routing input: stopped=%t, err=%v", stopped, err)
			}
		}
		if _, err := p.compose(true); err != nil {
			t.Fatal(err)
		}
		<-output.started
		deliver(input.Key{Code: input.Enter})
		if len(answers) != 0 {
			t.Fatalf("unwritten prompt received answers: %v", answers)
		}
		output.release <- struct{}{}
		if err := writer.Drain(time.Second); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(output.Bytes(), []byte("Approve?")) || writer.Written() <= root.Progress().WrittenSequence {
			t.Fatalf("prompt write must precede owner notification: output=%q, writer=%d, scene=%d", output.Bytes(), writer.Written(), root.Progress().WrittenSequence)
		}
		deliver(input.Key{Code: input.Enter})
		deliver(input.Paste{Text: "yes"})
		if len(answers) != 2 {
			t.Fatalf("completed prompt received %d answers, want a key and a paste", len(answers))
		}
	})
}

func TestPhysicalInputUsesCompletionEvidenceBeforeOwnerNotification(t *testing.T) {
	for _, clock := range []struct {
		name    string
		advance time.Duration
	}{
		{name: "distinct instants", advance: time.Nanosecond},
		{name: "equal instants"},
	} {
		for _, scenario := range []struct {
			name        string
			notifyFirst bool
			beforeWrite bool
			coalesced   bool
			want        string
		}{
			{name: "reader before notification", want: "B"},
			{name: "buffered input after notification", notifyFirst: true, want: "B"},
			{name: "input before next frame", notifyFirst: true, beforeWrite: true, want: "A"},
			{name: "coalesced completions", notifyFirst: true, coalesced: true, want: "B"},
		} {
			t.Run(clock.name+"/"+scenario.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					activated := ""
					button := func(name string) scene.Element {
						return scene.NewElement(scene.ElementConfig{
							Paint: func(frame scene.Frame) { frame.Text(0, 0, name, grid.Style{}) },
							Handle: func(event scene.Event) scene.EventResult {
								if mouse, ok := event.Input().(input.Mouse); ok && mouse.Action == input.MouseDown {
									activated = name
									return scene.EventResult{Consumed: true}
								}
								return scene.EventResult{}
							},
						})
					}
					group := scene.NewGroup(scene.GroupConfig{
						Items: []scene.Item{{Element: button("A")}, {Element: button("B")}},
						Layout: func(image.Point, []scene.Item) []image.Rectangle {
							return []image.Rectangle{image.Rect(0, 0, 1, 1), image.Rect(1, 0, 2, 1)}
						},
					})
					root, rootErr := scene.NewRoot(scene.RootConfig{Element: group.Element()})
					if rootErr != nil {
						t.Fatal(rootErr)
					}
					ctx, cancel := context.WithCancel(t.Context())
					output := &controlledFrameOutput{started: make(chan struct{}, 1), release: make(chan struct{})}
					writer := term.NewWriter(term.WriterConfig{Destination: output})
					p := &program{root: root, screen: grid.NewScreen(2, 1), writer: writer, tasks: newTaskQueue(), ctx: ctx, cancel: cancel}
					source := &pipeSource{events: make(chan input.Event, 1)}
					p.inputPipe = newInputPipe(ctx, root, source, source.events, writer.WrittenAt, Config{})
					t.Cleanup(func() {
						p.stop()
						close(output.release)
						if err := writer.Close(); err != nil {
							t.Error(err)
						}
					})
					compose := func() {
						t.Helper()
						if _, err := p.compose(true); err != nil {
							t.Fatal(err)
						}
						<-output.started
						if _, known := writer.WrittenAt(time.Now()); known {
							t.Fatal("in-progress write certified an input layout")
						}
						output.release <- struct{}{}
						if err := writer.Drain(time.Second); err != nil {
							t.Fatal(err)
						}
					}
					time.Sleep(time.Nanosecond)
					compose()
					p.wrote(writer.Written())
					time.Sleep(clock.advance)
					before := time.Now()
					time.Sleep(clock.advance)
					if err := group.Move(0, 1); err != nil {
						t.Fatal(err)
					}
					compose()
					time.Sleep(clock.advance)
					arrival := time.Now()
					if writer.Written() != 2 || root.Progress().WrittenSequence != 1 {
						t.Fatal("test did not leave completion ahead of the owner notification")
					}
					shown, screenErr := ptytest.NewScreen(ptytest.Size{Cols: 2, Rows: 1})
					if screenErr != nil {
						t.Fatal(screenErr)
					}
					if err := shown.Apply(output.Bytes()); err != nil {
						t.Fatal(err)
					}
					if err := shown.Flush(); err != nil {
						t.Fatal(err)
					}
					if shown.At(0, 0) != "B" || shown.At(1, 0) != "A" {
						t.Fatalf("transport output = %v", shown.Rows())
					}
					if scenario.beforeWrite {
						arrival = before
					}
					if scenario.coalesced {
						if err := group.Move(0, 1); err != nil {
							t.Fatal(err)
						}
						time.Sleep(clock.advance)
						compose()
					}
					if scenario.notifyFirst {
						if err := p.writerChanged(true); err != nil {
							t.Fatal(err)
						}
					}
					source.events <- input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, At: arrival}
					received := <-p.inputPipe.events
					if !scenario.notifyFirst {
						if err := p.writerChanged(true); err != nil {
							t.Fatal(err)
						}
					}
					if err := p.handleInput(received.event); err != nil {
						t.Fatal(err)
					}
					want := scenario.want
					if clock.advance == 0 {
						want = ""
					}
					if activated != want {
						t.Fatalf("activated %q, want %q", activated, want)
					}
				})
			})
		}
	}
}
