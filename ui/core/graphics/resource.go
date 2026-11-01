package graphics

import (
	"errors"
	"image"
	"io"
	"sync"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// Resource errors distinguish use of another terminal's image from use after
// its own terminal has retired it.
var (
	ErrForeignImage   = errors.New("graphics: image belongs to another owner")
	ErrImageReleased  = errors.New("graphics: image is no longer live")
	ErrImageReleasing = errors.New("graphics: image release is in progress")
)

// ImageOwner binds successfully transmitted wire IDs to one terminal. Its zero
// value is ready. An ImageOwner must not be copied after first use.
type ImageOwner struct {
	mu     sync.Mutex
	target grid.NativeTarget
	live   map[uint32]*imageRecord
}

type imageRecord struct {
	owner     *ImageOwner
	wire      Image
	releasing bool
}

// Resource names one transmission, not merely its terminal-local number.
// Copies share its lifetime. The zero value is not a live image.
type Resource struct{ record *imageRecord }

// NativeTarget identifies the canvas on which this owner's resources may be
// placed. It cannot release images or change their wire identity.
func (o *ImageOwner) NativeTarget() grid.NativeTarget {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.target == (grid.NativeTarget{}) {
		o.target = grid.NewNativeTarget()
	}
	return o.target
}

// Bind records a successful transmission. It panics for a zero ID or if the
// same ID is bound again before its previous resource is retired.
func (o *ImageOwner) Bind(wire Image) Resource {
	o.mu.Lock()
	defer o.mu.Unlock()
	if wire.ID == 0 || o.live[wire.ID] != nil {
		panic("graphics: image ID must be nonzero and unbound")
	}
	if o.live == nil {
		o.live = make(map[uint32]*imageRecord)
	}
	r := &imageRecord{owner: o, wire: wire}
	o.live[wire.ID] = r
	return Resource{record: r}
}

// Release calls send with the wire descriptor only after claiming its resource.
// A refused admission preserves the resource for retry. If send exits abnormally
// it may already have admitted a deletion; that capability is not replayable.
func (o *ImageOwner) Release(r Resource, send func(Image) error) error {
	if send == nil {
		return errors.New("graphics: image release requires transport admission")
	}
	record := r.record
	if record == nil || record.owner != o {
		return ErrForeignImage
	}
	o.mu.Lock()
	switch {
	case o.live[record.wire.ID] != record:
		o.mu.Unlock()
		return ErrImageReleased
	case record.releasing:
		o.mu.Unlock()
		return ErrImageReleasing
	}
	record.releasing = true
	o.mu.Unlock()
	refused := false
	defer func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.live[record.wire.ID] != record {
			return
		}
		if refused {
			record.releasing = false
			return
		}
		delete(o.live, record.wire.ID)
	}()
	err := send(record.wire)
	refused = err != nil
	return err
}

// RevokeAll ends outstanding capabilities when their terminal is no longer
// available. It sends no commands to a display that may now have another owner.
func (o *ImageOwner) RevokeAll() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.live = nil
}

// ID reports the wire ID for diagnostics and frame identity. It grants no
// release authority and is not unique between terminals.
func (r Resource) ID() uint32 {
	if r.record == nil {
		return 0
	}
	return r.record.wire.ID
}

// Size is the size in pixels of the transmitted image.
func (r Resource) Size() image.Point {
	if r.record == nil {
		return image.Point{}
	}
	return r.record.wire.Size
}

// Live reports whether the transmission can still be placed.
func (r Resource) Live() bool { return r.availability() == nil }

func (r Resource) availability() error {
	if r.record == nil {
		return ErrImageReleased
	}
	o := r.record.owner
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.live[r.ID()] != r.record {
		return ErrImageReleased
	}
	if r.record.releasing {
		return ErrImageReleasing
	}
	return nil
}

// Placement derives one region without exporting the editable wire descriptor.
// It panics for an empty resource or a zero placement ID.
func (r Resource) Placement(id uint32) ResourcePlacement {
	if r.record == nil || id == 0 {
		panic("graphics: placement requires an image and a nonzero ID")
	}
	return ResourcePlacement{resource: r, id: id}
}

// ResourcePlacement retains the issuing transmission's lifetime across frames.
type ResourcePlacement struct {
	resource Resource
	id       uint32
}

// SamePlacement identifies a particular transmission even if a retired wire
// ID has been rebound on the same terminal.
func (p ResourcePlacement) SamePlacement(other grid.Painter) bool {
	q, ok := other.(ResourcePlacement)
	return ok && p == q
}

// CheckTarget refuses retired or foreign resources before the renderer can
// mistake a matching wire ID and rectangle for an unchanged region.
func (p ResourcePlacement) CheckTarget(target grid.NativeTarget) error {
	if err := p.resource.availability(); err != nil {
		return err
	}
	if p.target() != target {
		return grid.ErrForeignPaint
	}
	return nil
}

func (p ResourcePlacement) target() grid.NativeTarget {
	if p.resource.record == nil {
		return grid.NativeTarget{}
	}
	return p.resource.record.owner.NativeTarget()
}

// Paint refuses a retired transmission rather than naming a reused wire ID.
func (p ResourcePlacement) Paint(w io.Writer, size image.Point) error {
	if err := p.resource.availability(); err != nil {
		return err
	}
	if !p.target().Accepts(w) {
		return grid.ErrForeignPaint
	}
	return p.resource.record.wire.Placement(p.id).Paint(w, size)
}

// Erase has no work once data release has removed all of its placements.
func (p ResourcePlacement) Erase(w io.Writer) error {
	if err := p.resource.availability(); err != nil {
		if errors.Is(err, ErrImageReleased) {
			return nil
		}
		return err
	}
	if !p.target().Accepts(w) {
		return grid.ErrForeignPaint
	}
	return p.resource.record.wire.Placement(p.id).Erase(w)
}
