package scene_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// CurrentLayout and Read used to disagree for keys: Read refused revoked geometry
// while CurrentLayout still claimed the input belonged to it.
func TestKeyboardInputAgreesAboutRevokedGeometry(t *testing.T) {
	var current []bool
	element := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true },
		Handle: func(e scene.Event) scene.EventResult {
			current = append(current, e.CurrentLayout())
			return scene.EventResult{Consumed: true}
		},
	})
	r := root(t, element)
	draw(t, r, 1, 1, 0)
	r.InvalidateGeometry()
	r.Handle(input.Key{Code: input.Enter})
	draw(t, r, 1, 1, 0)
	r.Handle(input.Key{Code: input.Enter})
	if len(current) != 2 || current[0] || !current[1] {
		t.Fatalf("current layout = %v, want revoked then current", current)
	}
}
