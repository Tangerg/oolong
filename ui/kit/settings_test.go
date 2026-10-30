package kit_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestSettingsComposeASelectableFittedValueList(t *testing.T) {
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
		Change: func(index int, _ preference, action keymap.Action) bool {
			if action != base.Increase {
				return false
			}
			values[index] = "off"
			return true
		},
	})

	equalRows(t, paintWidget(16, 2, settings), []string{
		"theme.......dark",
		"wrap..........on",
	})
	newTestRoot(settings).Handle(input.Key{Code: input.Down})
	if !newTestRoot(settings).Handle(input.Key{Code: input.Right}) || values[1] != "off" {
		t.Fatalf("right left values %v, want the selected value changed", values)
	}
	equalRows(t, paintWidget(16, 2, settings), []string{
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
