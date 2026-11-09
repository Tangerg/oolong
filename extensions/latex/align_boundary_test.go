package latex_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/ui/core/layout"
)

func TestRenderRejectsInvalidAlignmentEvenForEmptySource(t *testing.T) {
	for _, source := range []string{"", "x"} {
		t.Run(source, func(t *testing.T) {
			defer func() {
				err, _ := recover().(error)
				if !errors.Is(err, layout.ErrInvalidAlign) {
					t.Errorf("panic = %v, want %v", err, layout.ErrInvalidAlign)
				}
			}()
			latex.Render(source, latex.Look{Align: layout.Align(math.MaxUint8)})
		})
	}
}
