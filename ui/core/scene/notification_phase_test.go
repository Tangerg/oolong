package scene_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestNotificationsWaitForAFramePreparedByAnEarlierCallback(t *testing.T) {
	for _, completion := range []struct {
		name   string
		commit bool
	}{
		{name: "commit", commit: true},
		{name: "abort"},
	} {
		t.Run(completion.name, func(t *testing.T) {
			var r *scene.Root
			var pending *scene.Prepared
			defer func() { pending.Abort() }()
			var delivered []string
			live := scene.NewElement(scene.ElementConfig{})
			first := scene.NewElement(scene.ElementConfig{Detached: func() {
				delivered = append(delivered, "first")
				var err error
				pending, err = r.Prepare(grid.NewSurface(1, 1).View())
				if err != nil {
					t.Fatal(err)
				}
			}})
			second := scene.NewElement(scene.ElementConfig{Detached: func() {
				delivered = append(delivered, "second")
				if err := live.CheckMutation(); err != nil {
					t.Errorf("cleanup ran while the next frame was unsettled: %v", err)
				}
			}})
			r = root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{
				{Element: first}, {Element: second},
			}}).Element())
			if err := r.SetContent(live); err != nil {
				t.Fatal(err)
			}
			r.FlushNotifications()
			r.FlushNotifications()
			if !slices.Equal(delivered, []string{"first"}) {
				t.Fatalf("unsettled frame did not pause notification delivery: %v", delivered)
			}
			if completion.commit {
				pending.Commit(0)
			} else {
				pending.Abort()
			}
			r.FlushNotifications()
			if !slices.Equal(delivered, []string{"first", "second"}) {
				t.Fatalf("settlement lost or reordered notifications: %v", delivered)
			}
		})
	}
}
