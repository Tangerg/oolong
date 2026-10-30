package grid

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unsafe"
)

// ErrCaptureLimit refuses content before a permanent-output owner accepts it.
var ErrCaptureLimit = errors.New("grid: captured content exceeds its budget")

var errScrollbackPaint = fmt.Errorf("grid: scrollback cannot retain native paint regions: %w", errors.ErrUnsupported)

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
	if cfg.Content == nil {
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
	defer surface.Reset()
	cfg.Content.Draw(surface.borrow(nil))
	surface.endDrawing()
	if len(surface.regions()) != 0 {
		return Frozen{}, errScrollbackPaint
	}
	out := Frozen{rows: make([][]Cell, height), width: cfg.Width}
	out.bytes = int64(height) * int64(unsafe.Sizeof([]Cell{}))
	for y := range height {
		row := trimBlankTail(surface.row(y))
		for _, cell := range row {
			cost := int64(unsafe.Sizeof(Cell{})) + int64(len(cell.content)) + int64(len(cell.Link))
			if cost > cfg.MaxBytes-out.bytes {
				return Frozen{}, ErrCaptureLimit
			}
			out.bytes += cost
		}
		out.rows[y] = slices.Clone(row)
		for x := range out.rows[y] {
			out.rows[y][x].content = strings.Clone(out.rows[y][x].content)
			out.rows[y][x].Link = strings.Clone(out.rows[y][x].Link)
		}
	}
	return out, nil
}

func (cfg CaptureConfig) normalized() (CaptureConfig, error) {
	if cfg.Width <= 0 || cfg.MaxCells < 0 || cfg.MaxBytes < 0 {
		return CaptureConfig{}, errors.New("grid: invalid capture configuration")
	}
	if cfg.MaxCells == 0 {
		cfg.MaxCells = 1 << 18
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 16 << 20
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
