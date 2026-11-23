package kit

import "testing"

func TestMemoDerivesOncePerKey(t *testing.T) {
	var m memo[int, string]
	derivations := 0
	derive := func(value string) func() string {
		return func() string { derivations++; return value }
	}
	if got := *m.get(1, derive("one")); got != "one" {
		t.Fatalf("first get = %q", got)
	}
	if got := *m.get(1, derive("again")); got != "one" || derivations != 1 {
		t.Fatalf("same key = %q after %d derivations, want the memoized value after 1", got, derivations)
	}
	if got := *m.get(2, derive("two")); got != "two" || derivations != 2 {
		t.Fatalf("new key = %q after %d derivations", got, derivations)
	}
}
