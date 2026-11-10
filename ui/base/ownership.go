package base

import (
	"slices"

	"github.com/Tangerg/oolong/ui/internal/retention"
)

// noCopy is the standard go vet marker for mutable owners whose internal references
// make copying after first use unsafe. Its methods are never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// own replaces dst with a copy of src while keeping spare storage proportional to
// the live collection. Ordinary refreshes reuse their allocation; a collection that
// shrinks far below an old peak releases that peak instead of retaining it for the
// lifetime of a streaming interface.
func own[T any](dst, src []T) []T {
	if len(src) == 0 {
		clear(dst)
		return nil
	}
	if cap(dst) < len(src) || retention.ShouldCompact(cap(dst), len(src)) {
		return slices.Clone(src)
	}
	old := len(dst)
	dst = dst[:max(old, len(src))]
	copy(dst, src)
	if old > len(src) {
		clear(dst[len(src):old])
	}
	return dst[:len(src)]
}

// trim releases storage that is far larger than the live collection. It is for
// models that rebuild directly into their own buffer and therefore have no source
// slice to pass through own.
func trim[T any](items []T) []T {
	if len(items) == 0 {
		clear(items)
		return nil
	}
	if retention.ShouldCompact(cap(items), len(items)) {
		return slices.Clone(items)
	}
	return items
}

// requireNonNegative is the one rule for counts of rows, gaps and items: a
// constructor and its setter refuse a negative value in the same words.
func requireNonNegative(name string, value int) {
	if value < 0 {
		panic("base: " + name + " cannot be negative")
	}
}
