package link

// Map records where links were drawn, so that a click can be answered.
//
// It is filled while drawing and read when a click arrives. That is the only
// arrangement of these two that cannot fall out of step: the record is produced by
// the pass that drew the cells, so there is nothing to invalidate, no second
// detection over text that may have changed since, and no cache to be wrong.
//
// A [Map] holds screen positions, so it belongs to a frame. Reset it at the start of
// each one — see [Map.Reset] for why that is cheap.
type Map struct{ regions []region }

// region is one run of columns on one row that carries a target.
type region struct {
	y, x, w int
	url     string
}

// Reset empties the map, keeping the space it had. A frame draws roughly what the
// last one did, so the allocation from the first frame serves every frame after.
func (m *Map) Reset() { m.regions = m.regions[:0] }

// Add records that w columns from (x, y) carry url. A run of no width records
// nothing, which is what a link scrolled off the edge comes to.
func (m *Map) Add(x, y, w int, url string) {
	if w <= 0 || url == "" {
		return
	}
	m.regions = append(m.regions, region{y: y, x: x, w: w, url: url})
}

// At is the target at a screen position, and whether there is one.
//
// Later records win, which is what overlapping draws mean: something drawn over a
// link covers it, and a click lands on what is in front.
func (m *Map) At(x, y int) (string, bool) {
	for i := len(m.regions) - 1; i >= 0; i-- {
		if r := m.regions[i]; r.y == y && x >= r.x && x < r.x+r.w {
			return r.url, true
		}
	}
	return "", false
}

// Len is how many runs the map holds, which is what a test asserts on and what a
// caller checks before bothering to hit-test a click at all.
func (m *Map) Len() int { return len(m.regions) }
