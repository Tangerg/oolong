package scene_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestAttachmentHistorySurvivesEveryCompletedLifetime(t *testing.T) {
	var absent scene.Element
	element := scene.NewElement(scene.ElementConfig{})
	if absent.HasBeenAttached() || element.HasBeenAttached() {
		t.Fatal("an unmounted element reported an attachment history")
	}
	for range 2 {
		root, err := scene.NewRoot(scene.RootConfig{Element: element})
		if err != nil {
			t.Fatal(err)
		}
		if !element.HasBeenAttached() || !element.Alive() {
			t.Fatal("current attachment is missing from history")
		}
		root.Close()
		if !element.HasBeenAttached() || element.Alive() {
			t.Fatal("detachment erased attachment history or retained liveness")
		}
	}
}
