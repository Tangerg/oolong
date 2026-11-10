//go:build windows

package term

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestAWindowsConsoleWithoutTermHasColor(t *testing.T) {
	env := func(values map[string]string) func(string) (string, bool) {
		return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
	}
	if got := localDepth(env(nil)); got != grid.TrueColor {
		t.Fatalf("console depth = %v, want truecolor", got)
	}
	if got := localDepth(env(map[string]string{"NO_COLOR": "1"})); got != grid.NoColor {
		t.Fatalf("NO_COLOR depth = %v", got)
	}
	if got := localDepth(env(map[string]string{"TERM": "xterm-256color"})); got != grid.Depth256 {
		t.Fatalf("mintty depth = %v", got)
	}
}
