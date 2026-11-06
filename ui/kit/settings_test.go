package kit_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestSettingsComposeASelectableFittedValueList(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	type preference struct{ name string }
	items := []preference{{"theme"}, {"wrap"}}
	values := []string{"dark", "on"}
	settings := kit.NewSettings(kit.SettingsConfig[preference]{
		Items: items,
		Label: func(item preference) string { return item.name },
		Value: func(item preference) string {
			for i := range items {
				if items[i] == item {
					return values[i]
				}
			}
			return ""
		},
		Change: func(index int, _ preference, action keymap.Action) scene.ActionResult[bool] {
			if action != base.Increase {
				return scene.ActionResult[bool]{}
			}
			values[index] = "off"
			return scene.ActionResult[bool]{State: scene.Applied, Value: true}
		},
	})

	equalRows(t, paintWidget(scenes, 16, 2, settings), []string{
		"theme.......dark",
		"wrap..........on",
	})
	scenes.Mount(settings).Handle(input.Key{Code: input.Down})
	if !scenes.Mount(settings).Handle(input.Key{Code: input.Right}) || values[1] != "off" {
		t.Fatalf("right left values %v, want the selected value changed", values)
	}
	equalRows(t, paintWidget(scenes, 16, 2, settings), []string{
		"theme.......dark",
		"wrap.........off",
	})
}

func TestSettingsOwnTheirItemSlice(t *testing.T) {
	items := []string{"one", "two"}
	settings := kit.NewSettings(kit.SettingsConfig[string]{
		Items: items, Label: func(item string) string { return item },
		Value: func(string) string { return "" },
	})
	items[0] = "changed"
	if got := settings.Controller().List().Items(); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("Items = %v, want the controller-owned input", got)
	}
}
