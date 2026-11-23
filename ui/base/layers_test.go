package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestLayersKeepTheMostRecentlyShownOverlayOnTop(t *testing.T) {
	content := scene.NewElement(scene.ElementConfig{})
	first, second := scene.NewElement(scene.ElementConfig{}), scene.NewElement(scene.ElementConfig{})
	layers := base.NewLayers(base.LayersConfig{Content: content})
	layers.Show(first)
	layers.Show(second)
	layers.Show(first)
	if got := layers.Overlays(); !slices.Equal(got, []scene.Element{second, first}) {
		t.Fatalf("overlays %v, want the raised overlay last", got)
	}
	if top, ok := layers.Top(); !ok || top != first {
		t.Fatal("the raised overlay is not on top")
	}
	if !layers.Dismiss(first) || layers.Dismiss(first) {
		t.Fatal("dismissal did not report exactly one removal")
	}
	replacement := scene.NewElement(scene.ElementConfig{})
	layers.SetContent(replacement)
	if layers.Content() != replacement || !slices.Equal(layers.Overlays(), []scene.Element{second}) {
		t.Fatal("replacing the content changed the overlays")
	}
	if top, ok := layers.Top(); !ok || top != second {
		t.Fatal("the remaining overlay is not on top")
	}
}

func TestTheTopModalLayerHoldsTheKeyboard(t *testing.T) {
	modal := func() (scene.Element, scene.Element) {
		child := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
		group := scene.NewGroup(scene.GroupConfig{Modal: true, Overlay: true, Items: []scene.Item{{Element: child}}})
		return group.Element(), child
	}
	first, firstChild := modal()
	second, secondChild := modal()
	layers := base.NewLayers(base.LayersConfig{Content: scene.NewElement(scene.ElementConfig{})})
	root, err := scene.NewRoot(scene.RootConfig{Element: layers.Element()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	layers.Show(first)
	layers.Show(second)
	root.FlushNotifications()
	if !secondChild.Focused() {
		t.Fatal("the later modal layer does not hold the keyboard")
	}
	layers.Show(first)
	root.FlushNotifications()
	if !firstChild.Focused() {
		t.Fatal("raising a modal layer did not give it the keyboard")
	}
}
