package base

import (
	"errors"
	"image"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// Virtual resolution failures preserve the previous mounted window and measurements.
var (
	ErrVirtualLimit         = errors.New("base: virtual window resource budget exhausted")
	ErrVirtualSourceChanged = errors.New("base: virtual source changed during resolution")
)

// ItemKey is the dataset-owned stable identity used across reordering and materialization.
type ItemKey string

// VirtualSource owns dataset order and identity. Measure is a pure query. A
// revision must change whenever keys, order or intrinsic heights change.
type VirtualSource interface {
	Revision() uint64
	Len() int
	Key(index int) ItemKey
	Measure(index, width int) int
}

// VirtualConfig separates geometry retention from per-resolution measurement work.
// Geometry knowledge survives scrolling until the source revision or width changes.
// MaxItems bounds its retained measurements; MaxMeasurementsPerResolve bounds new
// measurements in one call. Build runs only for materialized visible/overscan rows.
type VirtualConfig struct {
	Source                    VirtualSource
	Build                     func(ItemKey) scene.Element
	EstimatedHeight           int
	Overscan                  int
	MaxItems                  int
	MaxKeyBytes               int
	MaxMeasurementsPerResolve int
}

const (
	defaultVirtualItems                  = 100_000
	defaultVirtualKeyBytes               = 4 << 20
	defaultVirtualMeasurementsPerResolve = 512
)

// VirtualItem is one materialized content identity with its projected dataset index and row extent.
type VirtualItem struct {
	Key    ItemKey
	Index  int
	Top    int
	Height int
}

// VirtualRange is a bounded materialized window; Total is an estimate unless Exact is true.
type VirtualRange struct {
	Items    []VirtualItem
	Offset   int
	Total    int
	Exact    bool
	Revision uint64
}

type measuredHeight struct{ height int }

// VirtualWindow owns the measured-height cache and materialized window. Its
// scene Group alone owns actual attachment. Resolve is an explicit owner-stage
// operation and must run before painting; it never runs from Paint or Measure.
// A VirtualWindow must not be copied after first use.
type VirtualWindow struct {
	noCopy    noCopy
	cfg       VirtualConfig
	width     int
	keys      []ItemKey
	heights   map[ItemKey]measuredHeight
	window    VirtualRange
	group     *scene.Group
	resolving bool
}

// NewVirtualWindow validates budgets and creates the sole scene group for materialized items.
func NewVirtualWindow(cfg VirtualConfig) (*VirtualWindow, error) {
	if cfg.Source == nil || cfg.EstimatedHeight < 0 || cfg.Overscan < 0 || cfg.MaxItems < 0 || cfg.MaxKeyBytes < 0 || cfg.MaxMeasurementsPerResolve < 0 {
		return nil, errors.New("base: invalid virtual window configuration")
	}
	if cfg.EstimatedHeight == 0 {
		cfg.EstimatedHeight = 1
	}
	if cfg.MaxItems == 0 {
		cfg.MaxItems = defaultVirtualItems
	}
	if cfg.MaxKeyBytes == 0 {
		cfg.MaxKeyBytes = defaultVirtualKeyBytes
	}
	if cfg.MaxMeasurementsPerResolve == 0 {
		cfg.MaxMeasurementsPerResolve = defaultVirtualMeasurementsPerResolve
	}
	v := &VirtualWindow{cfg: cfg, heights: make(map[ItemKey]measuredHeight)}
	v.group = scene.NewGroup(scene.GroupConfig{Layout: func(size image.Point, children []scene.Item) []image.Rectangle {
		rects := make([]image.Rectangle, len(children))
		for i := range children {
			item := v.window.Items[i]
			rects[i] = image.Rect(0, item.Top-v.window.Offset, size.X, item.Top-v.window.Offset+item.Height)
		}
		return rects
	}})
	return v, nil
}

func (v *VirtualWindow) Element() scene.Element {
	if v == nil {
		return scene.Element{}
	}
	return v.group.Element()
}

// Snapshot returns a detached description of the most recently resolved window.
func (v *VirtualWindow) Snapshot() VirtualRange {
	if v == nil {
		return VirtualRange{}
	}
	out := v.window
	out.Items = slices.Clone(out.Items)
	return out
}

// Cached includes offscreen measurements needed to keep absolute row positions stable.
func (v *VirtualWindow) Cached() int {
	if v == nil {
		return 0
	}
	return len(v.heights)
}

// Resolve measures a bounded candidate window and publishes it only after every validation succeeds.
// It may invoke Build and therefore belongs to the owner phase, before scene preparation.
// It panics if a source or Build callback reenters Resolve on the same window.
func (v *VirtualWindow) Resolve(width, height, offset int) (VirtualRange, error) {
	if v == nil {
		return VirtualRange{}, errors.New("base: virtual window is nil")
	}
	if v.resolving {
		panic("base: reentrant virtual window resolution")
	}
	if err := v.group.Element().CheckMutation(); err != nil {
		return VirtualRange{}, err
	}
	if width <= 0 || height < 0 {
		return VirtualRange{}, errors.New("base: invalid virtual viewport")
	}
	v.resolving = true
	defer func() { v.resolving = false }()
	var candidate virtualCandidate
	var err error
	v.Element().ReadOnly(func() { candidate, err = v.prepare(width, height, offset) })
	if err != nil {
		return VirtualRange{}, err
	}
	children, err := v.materialize(candidate.window.Items)
	if err != nil {
		return VirtualRange{}, err
	}
	if !readProjection(v.Element(), func() bool { return v.sameSource(candidate.window.Revision, len(candidate.keys)) }) {
		return VirtualRange{}, ErrVirtualSourceChanged
	}
	if err = v.group.CheckSet(children...); err != nil {
		return VirtualRange{}, err
	}
	// Installing children invokes scene predicates after attachment. They must
	// already observe the range to which those children belong.
	v.keys, v.heights, v.width, v.window = candidate.keys, candidate.heights, width, candidate.window
	if err = v.group.Set(children...); err != nil {
		return VirtualRange{}, err
	}
	return v.Snapshot(), nil
}

type virtualCandidate struct {
	keys    []ItemKey
	heights map[ItemKey]measuredHeight
	window  VirtualRange
}

func (v *VirtualWindow) prepare(width, height, offset int) (virtualCandidate, error) {
	revision, count := v.cfg.Source.Revision(), v.cfg.Source.Len()
	keys, err := v.sourceKeys(revision, count)
	if err != nil {
		return virtualCandidate{}, err
	}
	cache := v.cachedHeights(width, revision)
	result, err := v.measureWindow(keys, cache, width, height, offset)
	if err != nil {
		return virtualCandidate{}, err
	}
	result.Revision = revision
	if !v.sameSource(revision, count) {
		return virtualCandidate{}, ErrVirtualSourceChanged
	}
	// Prefix positions depend on measurements outside the mounted window. Their
	// lifetime follows the bounded dataset revision and width, not visibility.
	return virtualCandidate{keys: keys, heights: cache, window: result}, nil
}

// A candidate can reuse measurements only under the exact source and width key.
// It receives a separate map so a refused candidate cannot change live geometry.
func (v *VirtualWindow) cachedHeights(width int, revision uint64) map[ItemKey]measuredHeight {
	cache := make(map[ItemKey]measuredHeight)
	if revision == v.window.Revision && width == v.width {
		maps.Copy(cache, v.heights)
	}
	return cache
}

func (v *VirtualWindow) sameSource(revision uint64, count int) bool {
	return v.cfg.Source.Revision() == revision && v.cfg.Source.Len() == count
}

func (v *VirtualWindow) sourceKeys(revision uint64, count int) ([]ItemKey, error) {
	if count < 0 || count > v.cfg.MaxItems {
		return nil, ErrVirtualLimit
	}
	if revision == v.window.Revision && v.keys != nil {
		if len(v.keys) != count {
			return nil, ErrVirtualSourceChanged
		}
		return v.keys, nil
	}
	keys := make([]ItemKey, count)
	seen := make(map[ItemKey]bool, count)
	bytes := 0
	for index := range count {
		key := v.cfg.Source.Key(index)
		if key == "" || seen[key] {
			return nil, errors.New("base: empty or duplicate virtual item key")
		}
		if len(key) > v.cfg.MaxKeyBytes-bytes {
			return nil, ErrVirtualLimit
		}
		bytes += len(key)
		seen[key] = true
		keys[index] = ItemKey(strings.Clone(string(key)))
	}
	return keys, nil
}

func (v *VirtualWindow) measureWindow(keys []ItemKey, cache map[ItemKey]measuredHeight, width, height, offset int) (VirtualRange, error) {
	measured := 0
	for {
		tops, exact, err := v.estimateTops(keys, cache)
		if err != nil {
			return VirtualRange{}, err
		}
		at := min(max(offset, 0), max(tops[len(keys)]-height, 0))
		from := max(at-v.cfg.Overscan, 0)
		through := at + min(height, math.MaxInt-at)
		through += min(v.cfg.Overscan, math.MaxInt-through)
		items, changed, err := v.measureRange(keys, tops, cache, width, from, through, &measured)
		if err != nil {
			return VirtualRange{}, err
		}
		if changed {
			continue
		}
		return VirtualRange{Items: items, Offset: at, Total: tops[len(keys)], Exact: exact || len(cache) == len(keys)}, nil
	}
}

func (v *VirtualWindow) estimateTops(keys []ItemKey, cache map[ItemKey]measuredHeight) ([]int, bool, error) {
	tops := make([]int, len(keys)+1)
	exact := true
	for index, key := range keys {
		rows := v.cfg.EstimatedHeight
		if cached, ok := cache[key]; ok {
			rows = cached.height
		} else {
			exact = false
		}
		if rows > math.MaxInt-tops[index] {
			return nil, false, ErrVirtualLimit
		}
		tops[index+1] = tops[index] + rows
	}
	return tops, exact, nil
}

func (v *VirtualWindow) measureRange(keys []ItemKey, tops []int, cache map[ItemKey]measuredHeight, width, from, through int, measured *int) ([]VirtualItem, bool, error) {
	if from >= through {
		return nil, false, nil
	}
	var items []VirtualItem
	changed := false
	for index, key := range keys {
		if tops[index+1] == tops[index] || tops[index+1] <= from || tops[index] >= through {
			continue
		}
		rows, ok := cache[key]
		if !ok {
			if *measured >= v.cfg.MaxMeasurementsPerResolve {
				return nil, false, ErrVirtualLimit
			}
			value := v.cfg.Source.Measure(index, width)
			if value < 0 {
				return nil, false, errors.New("base: negative virtual item height")
			}
			rows = measuredHeight{height: value}
			cache[key] = rows
			*measured++
			changed = changed || value != v.cfg.EstimatedHeight
		}
		items = append(items, VirtualItem{Key: key, Index: index, Top: tops[index], Height: rows.height})
	}
	return items, changed, nil
}

func (v *VirtualWindow) materialize(items []VirtualItem) ([]scene.Item, error) {
	if v.cfg.Build == nil {
		return nil, nil
	}
	current := v.group.Items()
	mounted := make(map[ItemKey]scene.Element, len(current))
	for _, item := range current {
		mounted[ItemKey(item.Key)] = item.Element
	}
	children := make([]scene.Item, len(items))
	for index, item := range items {
		element := mounted[item.Key]
		if element.IsZero() {
			element = v.cfg.Build(item.Key)
		}
		if element.IsZero() {
			return nil, errors.New("base: virtual item builder returned no element")
		}
		children[index] = scene.Item{Key: string(item.Key), Element: element}
	}
	return children, nil
}
