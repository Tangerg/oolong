package base

import "testing"

func TestStickyReleasesDiscardedPrefixAllocation(t *testing.T) {
	var sticky Sticky
	for i := range 100000 {
		sticky.Add(BlockID(i + 1))
	}
	sticky.DiscardBefore(100000)
	if sticky.Len() != 1 || sticky.blocks.Capacity() > 4 || sticky.blocks.At(0) != 100000 {
		t.Fatalf("retained len=%d cap=%d", sticky.Len(), sticky.blocks.Capacity())
	}
}

func TestStickyRepeatedDiscardsKeepAllocationProportional(t *testing.T) {
	var sticky Sticky
	for i := range 10000 {
		sticky.Add(BlockID(i + 1))
	}
	for i := range 9999 {
		sticky.DiscardBefore(BlockID(i + 2))
	}
	if sticky.Len() != 1 || sticky.blocks.Capacity() > 18 {
		t.Fatalf("retained allocation: %d", sticky.blocks.Capacity())
	}
	sticky.Add(10001, 10002, 10003)
	sticky.SetBlocks([]BlockID{10004})
	sticky.DiscardBefore(10005)
	if sticky.blocks.Capacity() != 0 {
		t.Fatal("empty storage retained")
	}
}
