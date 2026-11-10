package kit

import (
	"reflect"
	"testing"
)

// Theme, ThemeOverride, over and apply list the roles by hand. Every role must
// appear in all four, or a scope would erase or ignore it without a compile error.
func TestThemeOverrideCoversEveryRole(t *testing.T) {
	themeType, overrideType := reflect.TypeFor[Theme](), reflect.TypeFor[ThemeOverride]()
	if themeType.NumField() != overrideType.NumField() {
		t.Fatalf("Theme has %d roles, ThemeOverride has %d", themeType.NumField(), overrideType.NumField())
	}
	for field := range themeType.Fields() {
		token, ok := overrideType.FieldByName(field.Name)
		if !ok || token.Type != reflect.PointerTo(field.Type) {
			t.Fatalf("ThemeOverride has no *%s token for role %s", field.Type, field.Name)
		}
	}
	for i := range themeType.NumField() {
		name := themeType.Field(i).Name
		value := reflect.New(themeType.Field(i).Type)
		reflect.Indirect(value).Set(reflect.ValueOf(Dark()).Field(i))
		var override ThemeOverride
		reflect.ValueOf(&override).Elem().FieldByName(name).Set(value)
		frozen := override.over(ThemeOverride{})
		if reflect.ValueOf(frozen).FieldByName(name).IsNil() {
			t.Fatalf("over drops an explicit %s token", name)
		}
		applied := reflect.ValueOf(frozen.apply(Theme{}))
		for j := range themeType.NumField() {
			got, want := applied.Field(j), reflect.Zero(themeType.Field(j).Type)
			if j == i {
				want = reflect.ValueOf(Dark()).Field(i)
			}
			if !reflect.DeepEqual(got.Interface(), want.Interface()) {
				t.Fatalf("overriding %s changed %s to %+v", name, themeType.Field(j).Name, got.Interface())
			}
		}
	}
}
