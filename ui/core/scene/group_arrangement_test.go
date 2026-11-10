package scene_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestAGroupRefusesAGapNoArrangementUses(t *testing.T) {
	for name, config := range map[string]scene.GroupConfig{
		"negative gap": {Gap: -1},
		"overlay gap":  {Overlay: true, Gap: 1},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				err, _ := recover().(error)
				if !errors.Is(err, scene.ErrInvalid) {
					t.Fatalf("NewGroup accepted %+v: %v", config, err)
				}
			}()
			scene.NewGroup(config)
		})
	}
}
