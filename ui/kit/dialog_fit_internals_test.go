package kit

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type fitRows struct{ rows *int }

func (f fitRows) HeightForWidth(int) int { return *f.rows }
func (fitRows) Draw(grid.View)           {}

func TestAFittingDialogFollowsItsContentUpToItsPlacement(t *testing.T) {
	rows := 3
	where := layout.Placement{Width: 20, Height: 10}
	newDialog := func(fit bool) *Dialog {
		return NewDialog(DialogConfig{
			Behavior: base.DialogConfig{Content: scene.NewElement(scene.Passive(fitRows{rows: &rows}))},
			Glyphs:   ASCII(), Where: where, Fit: fit,
		})
	}
	space := image.Pt(40, 20)
	fitted, fixed := newDialog(true), newDialog(false)
	overhead := fitted.Panel().Box().Overhead().Y
	if got := fitted.place(space); got.Dx() != 20 || got.Dy() != rows+overhead {
		t.Fatalf("fitting dialog placed %v, want 20x%d", got, rows+overhead)
	}
	if got := fixed.place(space); got.Dy() != where.Height {
		t.Fatalf("fixed dialog placed %v, want height %d", got, where.Height)
	}
	rows = 30
	if got := fitted.place(space); got.Dy() != where.Height {
		t.Fatalf("overflowing content placed %v, want the placement's height %d", got, where.Height)
	}
	rows = 0
	if got := fitted.place(space); got.Dy() != overhead {
		t.Fatalf("empty content placed %v, want only the chrome", got)
	}
}
