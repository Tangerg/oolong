package headless

import "testing"

func TestStickyReleasesDiscardedPrefixAllocation(t *testing.T) {
	var sticky Sticky
	sticky.blocks = make([]BlockID, 100000)
	for i := range sticky.blocks {
		sticky.blocks[i] = BlockID(i + 1)
	}
	sticky.DiscardBefore(100000)
	if len(sticky.blocks) != 1 || cap(sticky.blocks) > 4 || sticky.blocks[0] != 100000 {
		t.Fatalf("retained len=%d cap=%d", len(sticky.blocks), cap(sticky.blocks))
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
	if len(sticky.blocks) != 1 || cap(sticky.blocks)+sticky.releasedPrefix > 18 {
		t.Fatalf("retained allocation: %d + %d", cap(sticky.blocks), sticky.releasedPrefix)
	}
	sticky.Add(10001, 10002, 10003)
	sticky.SetBlocks([]BlockID{10004})
	sticky.DiscardBefore(10005)
	if sticky.blocks != nil || sticky.releasedPrefix != 0 {
		t.Fatal("empty storage retained")
	}
}
