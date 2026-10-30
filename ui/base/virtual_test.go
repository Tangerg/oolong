package base_test

import (
	"errors"
	"fmt"
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type virtualSource struct {
	revision  uint64
	keys      []base.ItemKey
	calls     int
	mutate    bool
	duplicate bool
}

func (s *virtualSource) Revision() uint64 { return s.revision }
func (s *virtualSource) Len() int         { return len(s.keys) }
func (s *virtualSource) Key(i int) base.ItemKey {
	if s.duplicate {
		return "duplicate"
	}
	return s.keys[i]
}

func (s *virtualSource) Measure(index, width int) int {
	s.calls++
	if s.mutate {
		s.revision++
	}
	return 1 + index%3 + 20/width
}

func virtualData(n int) *virtualSource {
	s := &virtualSource{revision: 1, keys: make([]base.ItemKey, n)}
	for i := range n {
		s.keys[i] = base.ItemKey(fmt.Sprintf("row-%d", i))
	}
	return s
}

func TestVirtualWindowMeasuresAndMountsOnlyBoundedWindow(t *testing.T) {
	data := virtualData(10000)
	builds := 0
	window, err := base.NewVirtualWindow(base.VirtualConfig{Source: data, EstimatedHeight: 2, Overscan: 2, MaxMeasurements: 32, Build: func(key base.ItemKey) scene.Element {
		builds++
		return scene.NewElement(scene.ElementConfig{Key: string(key), Paint: func(frame scene.Frame) { frame.Text(0, 0, string(key), grid.Style{}) }})
	}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := window.Resolve(40, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if data.calls > 32 || builds > 32 || len(first.Items) > 32 || first.Exact {
		t.Fatalf("eager dataset rendering calls=%d builds=%d range=%+v", data.calls, builds, first)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: window.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	candidate, err := root.Prepare(grid.NewSurface(40, 10).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Commit(1)
	root.Wrote(1)
	firstTarget := window.Element().Target()
	for offset := range 100 {
		current, err := window.Resolve(40, 10, offset*50)
		if err != nil {
			t.Fatal(err)
		}
		if window.Cached() > 32 || len(current.Items) > 32 {
			t.Fatal("height cache grew with session age")
		}
	}
	if window.Element().Target() != firstTarget {
		t.Fatal("window replaced its scene identity")
	}
}

func TestVirtualWindowDatasetChangeAndBudgetFailureAreAtomic(t *testing.T) {
	data := virtualData(10)
	window, err := base.NewVirtualWindow(base.VirtualConfig{Source: data, MaxItems: 12, MaxMeasurements: 32})
	if err != nil {
		t.Fatal(err)
	}
	before, err := window.Resolve(40, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	data.duplicate = true
	data.revision++
	if _, err := window.Resolve(40, 3, 0); err == nil {
		t.Fatal("duplicate keys accepted")
	}
	if got := window.Snapshot(); got.Revision != before.Revision || got.Total != before.Total {
		t.Fatal("failed data resolution replaced owner state")
	}
	data.duplicate = false
	data.mutate = true
	data.revision++
	if _, err := window.Resolve(10, 3, 0); !errors.Is(err, base.ErrVirtualSourceChanged) {
		t.Fatalf("changing source=%v", err)
	}
	if window.Snapshot().Revision != before.Revision {
		t.Fatal("mid-measure source changes committed")
	}
}

func TestVirtualWindowDoesNotPretendUnknownHeightsAreExact(t *testing.T) {
	data := virtualData(3)
	window, err := base.NewVirtualWindow(base.VirtualConfig{Source: data, MaxMeasurements: 8})
	if err != nil {
		t.Fatal(err)
	}
	complete, err := window.Resolve(40, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !complete.Exact || complete.Total != 6 {
		t.Fatalf("fully measured range=%+v", complete)
	}
	snapshot := window.Snapshot()
	snapshot.Items[0].Top = 999
	if window.Snapshot().Items[0].Top == 999 {
		t.Fatal("snapshot mutated the window")
	}
}

func TestVirtualWindowResolveIsAnOwnerPhase(t *testing.T) {
	data := virtualData(1)
	window, err := base.NewVirtualWindow(base.VirtualConfig{Source: data, Build: func(key base.ItemKey) scene.Element { return scene.NewElement(scene.ElementConfig{Key: string(key)}) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = window.Resolve(20, 2, 0); err != nil {
		t.Fatal(err)
	}
	element := scene.NewElement(scene.ElementConfig{Paint: func(scene.Frame) {
		if _, err = window.Resolve(20, 2, 0); !errors.Is(err, scene.ErrBusy) {
			t.Fatalf("resolve inside paint=%v", err)
		}
	}})
	group := scene.NewGroup(scene.GroupConfig{Layout: func(image.Point, []scene.Item) []image.Rectangle {
		return []image.Rectangle{image.Rect(0, 0, 20, 2), image.Rect(0, 0, 20, 2)}
	}})
	if err = group.Set(scene.Item{Element: window.Element()}, scene.Item{Element: element}); err != nil {
		t.Fatal(err)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	candidate, err := root.Prepare(grid.NewSurface(20, 2).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Commit(1)
}
