package visualtest

import (
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
