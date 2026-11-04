package latex

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func TestSymbolResolverRetainsOnlySuccessfulSymbols(t *testing.T) {
	var resolver symbolResolver
	for i := range 256 {
		name := fmt.Sprintf(`\oolongUnknown%d`, i)
		if _, err := resolver.resolve(name); err == nil {
			t.Fatalf("unknown symbol %q resolved successfully", name)
		}
	}
	if got := cacheEntries(&resolver); got != 0 {
		t.Fatalf("resolver retained %d untrusted failures, want none", got)
	}

	for range 2 {
		if _, err := resolver.resolve(`\alpha`); err != nil {
			t.Fatalf("known symbol did not resolve: %v", err)
		}
	}
	if got := cacheEntries(&resolver); got != 1 {
		t.Fatalf("resolver retained %d successful symbols, want one shared entry", got)
	}
}

func cacheEntries(resolver *symbolResolver) int {
	resolver.mu.RLock()
	defer resolver.mu.RUnlock()
	return len(resolver.cache)
}

func TestSymbolResolverSharesConcurrentSuccessfulLookups(t *testing.T) {
	var resolver symbolResolver
	var queries sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		queries.Go(func() {
			<-start
			for range 16 {
				if value, err := resolver.resolve(`\alpha`); value != "α" || err != nil {
					t.Errorf("alpha = %q, error = %v", value, err)
				}
				if _, err := resolver.resolve(`\oolongUnknown`); err == nil {
					t.Error("unknown symbol resolved successfully")
				}
			}
		})
	}
	close(start)
	queries.Wait()
	if got := cacheEntries(&resolver); got != 1 {
		t.Fatalf("resolver retained %d symbols, want one shared success", got)
	}
}

func TestSymbolResolverDetachesNamesFromFormulaSource(t *testing.T) {
	const macro = `\alpha`
	source := macro + "%" + strings.Repeat("discarded comment", 1<<12)
	var resolver symbolResolver
	if value, err := resolver.resolve(source[:len(macro)]); value != "α" || err != nil {
		t.Fatalf("alpha = %q, error = %v", value, err)
	}
	if got := cacheEntries(&resolver); got != 1 {
		t.Fatalf("resolver retained %d symbols, want one", got)
	}
	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	for name := range resolver.cache {
		at := uintptr(unsafe.Pointer(unsafe.StringData(name))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
		if at >= sourceStart && at < sourceEnd {
			t.Errorf("cached name %q retains the %d-byte formula source", name, len(source))
		}
	}
	runtime.KeepAlive(source)
}
