package program_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
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
	_ = runFixture(t.Context(), program.InlineConfig{Host: host, Build: func(*program.InlineRuntime) (scene.Element, error) { return fixtureElement(root), nil }})
	t.Fatal("draw panic was swallowed")
}

func TestImageOwnerCanReleaseTransmittedData(t *testing.T) {
	host := newHost(t)
	err := runFixture(t.Context(), program.ScreenConfig{Host: host, Build: func(r *program.Runtime) (scene.Element, error) {
		if err := r.Images().Release(graphics.Image{ID: 1}); err != nil {
			t.Fatal(err)
		}
		r.Quit()
		return fixtureElement(&printer{}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(host.releasedImages) != 1 || host.releasedImages[0].ID != 1 {
		t.Fatal("image release did not reach host")
	}
}
