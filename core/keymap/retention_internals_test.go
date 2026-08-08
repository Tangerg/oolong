package keymap

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/Tangerg/oolong/core/input"
)

func TestBindingsOwnActionNames(t *testing.T) {
	source := strings.Repeat("discarded ", 1<<12) + "kept"
	action := Action(source[len(source)-len("kept"):])
	var bindings Map
	bindings.Bind(action, input.Chord{Code: input.Enter})

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source)))                           //nolint:gosec // Test compares allocation identity and never dereferences the address.
	ownedStart := uintptr(unsafe.Pointer(unsafe.StringData(bindings.bound[0].Action.String()))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	if ownedStart >= sourceStart && ownedStart < sourceStart+uintptr(len(source)) {
		t.Fatal("bound action still shares the caller's source allocation")
	}
}

func TestUnbindingReleasesRegistryHighWaterStorage(t *testing.T) {
	const count = 1024
	var bindings Map
	keys := make([]input.Chord, count)
	for i := range keys {
		keys[i] = input.Chord{Code: input.Character, Rune: rune(0x1000 + i)}
		bindings.Bind("action", keys[i])
	}
	for _, key := range keys[1:] {
		if !bindings.Unbind(key) {
			t.Fatal("bound key could not be removed")
		}
	}
	if cap(bindings.bound) > 2*len(bindings.bound)+16 {
		t.Fatalf("one binding retains capacity %d from %d bindings", cap(bindings.bound), count)
	}
}
