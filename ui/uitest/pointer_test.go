package uitest_test

import (
	"context"
	"image"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestPointerAfterRepaintAcknowledgementNeedsNoClockAdvance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := uitest.New(t, uitest.Config{Width: 11, Height: 1})
		mouse := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(10, 0)}
		if host.Pointer(mouse) {
			t.Fatal("unbound host accepted pointer intent")
		}
		var painted atomic.Int64
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(*program.Runtime) (scene.Element, error) {
			slider := base.NewSlider(base.SliderConfig{Maximum: 100})
			element := slider.Element()
			err := element.SetPaint(func(frame scene.Frame) {
				slider.Stage(frame, frame.Bounds())
				frame.Text(0, 0, strconv.Itoa(slider.Value()), grid.Style{})
				painted.Store(int64(slider.Value()))
			})
			return element, err
		}})
		if err != nil {
			t.Fatal(err)
		}
		go func() { done <- app.Run(ctx) }()
		defer func() {
			cancel()
			if err := <-done; err != nil {
				t.Error(err)
			}
			if host.Pointer(mouse) {
				t.Error("ended program retained its pointer binding")
			}
		}()
		if _, err := host.Repaint(ctx); err != nil {
			t.Fatal(err)
		}
		acknowledged := time.Now()
		physical := mouse
		physical.At = acknowledged
		if host.Pointer(physical) {
			t.Fatal("physical arrival timestamp entered the synthetic input path")
		}
		if !host.Pointer(mouse) {
			t.Fatal("completed output refused pointer intent")
		}
		if !time.Now().Equal(acknowledged) {
			t.Fatal("test advanced the clock between acknowledgement and input admission")
		}
		if _, err := host.Repaint(ctx); err != nil {
			t.Fatal(err)
		}
		if got := painted.Load(); got != 100 {
			t.Fatalf("click after repaint acknowledgement was lost: value = %d", got)
		}
	})
}
