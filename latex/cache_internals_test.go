package latex

import (
	"fmt"
	"testing"
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
	count := 0
	resolver.cache.Range(func(any, any) bool {
		count++
		return true
	})
	return count
}
