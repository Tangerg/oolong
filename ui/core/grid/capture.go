package grid

import (
	"errors"
	"fmt"
	"slices"
	"unsafe"
)

// ErrCaptureLimit refuses content before a permanent-output owner accepts it.
var ErrCaptureLimit = errors.New("grid: captured content exceeds its budget")

var errScrollbackPaint = fmt.Errorf("grid: scrollback cannot retain native paint regions: %w", errors.ErrUnsupported)

const (
	defaultCaptureCells = 1 << 18
	defaultCaptureBytes = 16 << 20
)

// CaptureConfig bounds the cells and owned strings produced by a drawable. The
// bounds cover framework allocations, not allocations made inside user callbacks.
type CaptureConfig struct {
	Content  Drawable
	Width    int
	Ground   Ground
	MaxCells int
	MaxBytes int64
}

// Frozen contains only owned cell values. It retains no drawable, painter,
// borrowed view, or caller-owned slice, so accepted output cannot change later.
// Its zero value is empty and copies share immutable storage.
type Frozen struct {
	rows  [][]Cell
	width int
	bytes int64
}

// Capture measures and draws once. Failure transfers nothing; the drawing view
// expires even if the callback exits abnormally. Native paint needs a live frame
// and cannot be made permanent by copying its cell rectangle.
func Capture(cfg CaptureConfig) (Frozen, error) {
	var err error
	cfg, err = cfg.normalized()
	if err != nil {
		return Frozen{}, err
	}
	if IsNilDrawable(cfg.Content) {
		return Frozen{}, nil
	}
	height := cfg.Content.HeightForWidth(cfg.Width)
	if height <= 0 {
		return Frozen{}, nil
	}
	if height > cfg.MaxCells/cfg.Width || int64(height) > cfg.MaxBytes/int64(unsafe.Sizeof([]Cell{})) {
		return Frozen{}, ErrCaptureLimit
	}
	cellCount := cfg.Width * height
	if int64(cellCount) > cfg.MaxBytes/int64(unsafe.Sizeof(Cell{})) {
		return Frozen{}, ErrCaptureLimit
	}
	surface := NewSurface(cfg.Width, height)
	surface.SetGround(cfg.Ground)
	surface.capture = &captureStorage{limit: cfg.MaxBytes, values: make(map[string]string)}
	defer surface.releaseScratch()
	cfg.Content.Draw(surface.borrow(nil))
	surface.endDrawing()
	if surface.capture.err != nil {
		return Frozen{}, surface.capture.err
	}
	return surface.freeze(cfg.MaxBytes)
}

// A permanent capture owns a distinct string once. Admission happens before
// copying caller data, including text that a later drawing operation overwrites.
type captureStorage struct {
	limit, used int64
	err         error
	values      map[string]string
}

func (p *captureStorage) own(value string) string {
	if p == nil || value == "" {
		return ownedCluster(value)
	}
	if owned, ok := p.values[value]; ok {
		return owned
	}
	cost := int64(len(value))
	if cost > p.limit-p.used {
		p.err = ErrCaptureLimit
		return ""
	}
	owned := ownedCluster(value)
	p.values[owned] = owned
	p.used += cost
	return owned
}

// Once drawing ends, consuming a string from the pool charges only its first
// retained occurrence. Overwritten data never counts against a published receipt.
func (p *captureStorage) retainedCost(value string) int64 {
	if _, ok := p.values[value]; !ok {
		return 0
	}
	delete(p.values, value)
	return int64(len(value))
}

func (s *Surface) freeze(limit int64) (Frozen, error) {
	out := Frozen{rows: make([][]Cell, s.h), width: s.w}
	out.bytes = int64(s.h) * int64(unsafe.Sizeof([]Cell{}))
	for y := range s.h {
		row := trimBlankTail(s.row(y))
		cost := int64(len(row)) * int64(unsafe.Sizeof(Cell{}))
		for _, cell := range row {
			cost += s.capture.retainedCost(cell.content) + s.capture.retainedCost(cell.Link)
		}
		if cost > limit-out.bytes {
			return Frozen{}, ErrCaptureLimit
		}
		out.bytes += cost
		out.rows[y] = slices.Clone(row)
	}
	return out, nil
}

func (cfg CaptureConfig) normalized() (CaptureConfig, error) {
	if cfg.Width <= 0 || cfg.MaxCells < 0 || cfg.MaxBytes < 0 {
		return CaptureConfig{}, errors.New("grid: invalid capture configuration")
	}
	if cfg.MaxCells == 0 {
		cfg.MaxCells = defaultCaptureCells
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = defaultCaptureBytes
	}
	return cfg, nil
}

// Width is the capture width; Rows can project these same atoms at another width.
func (f Frozen) Width() int { return f.width }

// RowCount includes intentional empty rows in the complete captured content.
func (f Frozen) RowCount() int { return len(f.rows) }

// StoredBytes counts owned cell, row-header, text and link storage, excluding caller allocations and encoding.
func (f Frozen) StoredBytes() int64 { return f.bytes }

// Rows returns owned rows reflowed at width. A complete display atom is never
// split. An atom wider than the target refuses the whole projection, leaving
// accepted content available to the compositor at a representable width.
func (f Frozen) Rows(width int) ([][]Cell, error) {
	if width <= 0 {
		return nil, errors.New("grid: frozen rows require a positive width")
	}
	var rows [][]Cell
	for _, source := range f.rows {
		if len(source) == 0 {
			rows = append(rows, nil)
			continue
		}
		for len(source) > 0 {
			end := 0
			for end < len(source) {
				unit := paintUnitAt(source, end, len(source)-end)
				if unit.width > width-end {
					break
				}
				end += unit.width
			}
			if end == 0 {
				return nil, fmt.Errorf("grid: captured atom width %d exceeds width %d: %w", source[0].Width(), width, errors.ErrUnsupported)
			}
			rows = append(rows, slices.Clone(source[:end]))
			source = source[end:]
		}
	}
	return rows, nil
}
