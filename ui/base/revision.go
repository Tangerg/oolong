package base

import "math"

func nextRevision(revision uint64) uint64 {
	if revision == math.MaxUint64 {
		panic("base: revision exhausted")
	}
	return revision + 1
}
