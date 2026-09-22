package program_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
)

type panicDraw struct{ calls int }

func (p *panicDraw) Draw(grid.View)        { p.calls++; panic("original draw failure") }
func (*panicDraw) Handle(input.Event) bool { return false }

func TestInlinePanicDoesNotDrawAgainDuringCleanup(t *testing.T) {
	host := newHost(t)
	root := &panicDraw{}
	defer func() {
		if failure := recover(); failure != "original draw failure" || root.calls != 1 {
			t.Fatalf("panic=%v, draws=%d", failure, root.calls)
		}
	}()
	_ = program.Run(t.Context(), program.Config{Host: host, Inline: func(*program.InlineRuntime) program.Component { return root }})
	t.Fatal("draw panic was swallowed")
}

func TestImageOwnerCanReleaseTransmittedData(t *testing.T) {
	host := newHost(t)
	err := program.Run(t.Context(), program.Config{Host: host, Root: func(r *program.Runtime) program.Component {
		if err := r.Images().Release(graphics.Image{ID: 1}); err != nil {
			t.Fatal(err)
		}
		r.Quit()
		return &printer{}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(host.releasedImages) != 1 || host.releasedImages[0].ID != 1 {
		t.Fatal("image release did not reach host")
	}
}
