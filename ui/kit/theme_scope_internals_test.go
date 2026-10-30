package kit

import (
	"math"
	"runtime"
	"testing"
)

func TestThemeRevisionExhaustionCannotWrapOrChangePalette(t *testing.T) {
	initial := Dark()
	owner := NewThemeOwner(initial)
	owner.revision = math.MaxUint64
	func() {
		defer func() {
			if recover() == nil {
				t.Error("theme revision wrapped")
			}
		}()
		owner.Set(Light())
	}()
	if owner.revision != math.MaxUint64 || owner.palette != initial {
		t.Fatal("exhausted revision partially changed theme")
	}
}

func TestThemeConsumersArePrunedBeforeTheirRegistryGrows(t *testing.T) {
	owner := NewThemeOwner(Dark())
	for range 16 {
		for range 32 {
			NewButton(ButtonConfig{Scope: owner.Scope(), Label: "ephemeral"})
		}
		runtime.GC()
	}
	// Register once more so pruning does not depend on a later palette update.
	live := NewButton(ButtonConfig{Scope: owner.Scope(), Label: "live"})
	if len(owner.consumers) > 64 || cap(owner.consumers) > 128 {
		t.Fatalf("retained weak handles grew with session age: len=%d cap=%d", len(owner.consumers), cap(owner.consumers))
	}
	runtime.KeepAlive(live)
}
