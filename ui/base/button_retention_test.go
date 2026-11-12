package base_test

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/Tangerg/oolong/ui/base"
)

func TestButtonReleasesReplacedConstructionLabel(t *testing.T) {
	button, oldLabel := func() (*base.Button, weak.Pointer[byte]) {
		label := strings.Repeat("original label", 1024)
		button := base.NewButton(base.ButtonConfig{Label: label})
		reference := weak.Make(unsafe.StringData(label)) //nolint:gosec // Observe the backing allocation without dereferencing its bytes.
		button.SetLabel("replacement")
		return button, reference
	}()
	runtime.GC()
	retained := oldLabel.Value() != nil
	runtime.KeepAlive(button)
	if retained {
		t.Fatal("button callbacks retained the replaced construction label")
	}
}
