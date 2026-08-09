package visualtest

import (
	"os"
	"slices"
	"testing"
)

func TestGoldenFormatKeepsVisibleSpacingOnly(t *testing.T) {
	rows := []string{"one  two   ", "", "  three  ", "   ", ""}
	want := []byte("one  two\n\n  three\n")
	if got := format(rows); !slices.Equal(got, want) {
		t.Fatalf("format = %q, want %q", got, want)
	}
	if rows[0] != "one  two   " {
		t.Fatal("format changed its caller's rows")
	}
}

func TestGoldenUpdateWritesTheComparisonFormat(t *testing.T) {
	path := t.TempDir() + "/screen.golden"
	old := *update
	*update = true
	t.Cleanup(func() { *update = old })

	Match(t, path, []string{"one  ", "", " two ", ""})
	got, err := os.ReadFile(path) //nolint:gosec // TempDir owns this test path.
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte("one\n\n two\n"); !slices.Equal(got, want) {
		t.Fatalf("updated golden = %q, want %q", got, want)
	}
}
