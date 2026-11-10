package kit_test

import (
	"context"
	"errors"
	"image"
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
		Change: func(_ context.Context, index int, _ preference, action keymap.Action) (scene.ActionResult[bool], error) {
			if action != base.Increase {
				return scene.ActionResult[bool]{}, nil
			}
			values[index] = "off"
			return scene.ActionResult[bool]{State: scene.Applied, Value: true}, nil
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

func TestSettingsSkinProjectsDomainFailuresOutsideSelectableRows(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	problem := errors.New("save failed")
	settings := kit.NewSettings(kit.SettingsConfig[string]{
		Items: []string{"theme", "wrap"}, Label: func(value string) string { return value },
		Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
			return scene.ActionResult[bool]{State: scene.Started, Value: true}, problem
		},
	})
	fixture := scenes.Mount(settings)
	if !fixture.Handle(input.Key{Code: input.Right}) || !errors.Is(settings.Controller().ChangeError(), problem) {
		t.Fatal("setting failure was not retained by its controller")
	}
	if height := settings.HeightForWidth(16); height != 3 {
		t.Fatalf("settings measured %d rows, want two choices and feedback", height)
	}
	equalRows(t, paintWidget(scenes, 16, 3, settings), []string{
		"theme...........", "wrap............", "save failed.....",
	})
	if fixture.Handle(input.Mouse{Pos: image.Pt(0, 2), Action: input.MouseDown, Button: input.ButtonLeft}) || settings.Controller().List().Selected() != 0 {
		t.Fatal("feedback row changed list selection")
	}
	settings.Controller().SetChange(func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
		return scene.ActionResult[bool]{Value: true}, nil
	})
	if !fixture.Handle(input.Key{Code: input.Right}) || settings.HeightForWidth(16) != 2 || settings.Controller().ChangeError() != nil {
		t.Fatal("successful retry did not clear the feedback row")
	}
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
