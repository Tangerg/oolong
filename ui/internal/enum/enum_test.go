package enum_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/internal/enum"
)

type color uint8

func TestNameReportsUnknownValuesByKind(t *testing.T) {
	names := []string{"red", "", "blue"}
	for value, want := range map[color]string{0: "red", 1: "color(1)", 2: "blue", 9: "color(9)"} {
		if got := enum.Name(names, value, "color"); got != want {
			t.Errorf("Name(%d) = %q, want %q", value, got, want)
		}
	}
}
