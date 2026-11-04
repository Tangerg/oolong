package graphics

import (
	"errors"
	"image"
	"sync"
	"sync/atomic"

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
	ledger atomic.Pointer[imageLedger]
}

// Handles may outlive the session that embeds ImageOwner. The ledger retains
// only image identity and lifetime; it must never point back to that session.
type imageLedger struct {
	mu     sync.Mutex
	target grid.NativeTarget
	live   map[uint32]*imageRecord
}

type imageRecord struct {
	ledger *imageLedger
	wire   Image
	state  imageState
}

type imageState uint8

const (
	imageLive imageState = iota
	imageReleasing
	imageRevokedDuringRelease
)

// Resource names one transmission, not merely its terminal-local number.
// Copies share its lifetime. The zero value is not a live image.
type Resource struct{ record *imageRecord }

// UploadResult is settled once. Writer admission alone cannot create a Resource.
type UploadResult struct {
	Resource Resource
	Err      error
}

// NativeTarget identifies the canvas on which this owner's resources may be
// placed. It cannot release images or change their wire identity.
func (o *ImageOwner) NativeTarget() grid.NativeTarget {
	return o.ensureLedger().target
}

func (o *ImageOwner) ensureLedger() *imageLedger {
	if ledger := o.ledger.Load(); ledger != nil {
		return ledger
	}
	ledger := &imageLedger{target: grid.NewNativeTarget()}
	if o.ledger.CompareAndSwap(nil, ledger) {
		return ledger
	}
	return o.ledger.Load()
}

func (o *ImageOwner) Contains(id uint32) bool {
	ledger := o.ledger.Load()
	if ledger == nil {
		return false
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	_, found := ledger.live[id]
	return found
}

// Bind records a successful transmission. It panics for a zero ID or if the
// same ID is bound again before its previous resource is retired.
func (o *ImageOwner) Bind(wire Image) Resource {
	ledger := o.ensureLedger()
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if wire.ID == 0 || ledger.live[wire.ID] != nil {
		panic("graphics: image ID must be nonzero and unbound")
	}
	if ledger.live == nil {
		ledger.live = make(map[uint32]*imageRecord)
	}
	r := &imageRecord{ledger: ledger, wire: wire}
	ledger.live[wire.ID] = r
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
	ledger := o.ledger.Load()
	if record == nil || record.ledger != ledger {
		return ErrForeignImage
	}
	ledger.mu.Lock()
	if err := record.available(); err != nil {
		ledger.mu.Unlock()
		return err
	}
	record.state = imageReleasing
	ledger.mu.Unlock()
	refused := false
	defer func() {
		ledger.mu.Lock()
		defer ledger.mu.Unlock()
		if refused && record.state == imageReleasing {
			record.state = imageLive
			return
		}
		delete(ledger.live, record.wire.ID)
	}()
	err := send(record.wire)
	refused = err != nil
	return err
}

// ReleaseAll submits deletion for each transmission owned at entry. Each
// successful admission retires that resource immediately. A refusal leaves
// the remaining resources available for retry.
func (o *ImageOwner) ReleaseAll(send func(Image) error) error {
	ledger := o.ledger.Load()
	if ledger == nil {
		return nil
	}
	ledger.mu.Lock()
	resources := make([]Resource, 0, len(ledger.live))
	for _, record := range ledger.live {
		resources = append(resources, Resource{record: record})
	}
	ledger.mu.Unlock()
	for _, resource := range resources {
		if err := o.Release(resource, send); err != nil {
			return err
		}
	}
	return nil
}

// RevokeAll ends outstanding capabilities when their terminal is no longer
// available. A deletion already inside its send callback keeps the wire ID
// reserved until return, so it cannot erase a newly bound transmission.
func (o *ImageOwner) RevokeAll() {
	ledger := o.ledger.Load()
	if ledger == nil {
		return
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for id, record := range ledger.live {
		if record.state == imageLive {
			delete(ledger.live, id)
		} else {
			record.state = imageRevokedDuringRelease
		}
	}
	if len(ledger.live) == 0 {
		ledger.live = nil
	}
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

func (r Resource) Live() bool { return r.availability() == nil }

func (r Resource) availability() error {
	if r.record == nil {
		return ErrImageReleased
	}
	ledger := r.record.ledger
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return r.record.available()
}

// The caller holds the owner's lock while testing or claiming the current ID.
func (r *imageRecord) available() error {
	if r.ledger.live[r.wire.ID] != r || r.state == imageRevokedDuringRelease {
		return ErrImageReleased
	}
	if r.state == imageReleasing {
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
	// Source is a half-open rectangle in the transmitted image's pixels.
	// Its zero value selects the entire image.
	Source image.Rectangle
}

func (p ResourcePlacement) SourceBounds() image.Rectangle {
	return sourceBounds(p.resource.Size(), p.Source)
}

// Draw uses the transmission and placement identities to reconcile a region across frames.
func (p ResourcePlacement) Draw(v grid.View, area image.Rectangle) {
	id := uint64(p.resource.ID())<<32 | uint64(p.id)
	v.Paint(area, id, p)
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
	return p.resource.record.ledger.target
}

// Paint refuses a retired transmission rather than naming a reused wire ID.
func (p ResourcePlacement) Paint(context grid.PaintContext) error {
	if err := p.resource.availability(); err != nil {
		return err
	}
	if !p.target().Accepts(context) {
		return grid.ErrForeignPaint
	}
	wire := p.resource.record.wire.Placement(p.id)
	wire.Source = p.Source
	return wire.Paint(context, context.Size(), context.Layer())
}

// Erase has no work once data release has removed all of its placements.
func (p ResourcePlacement) Erase(context grid.PaintContext) error {
	if err := p.resource.availability(); err != nil {
		if errors.Is(err, ErrImageReleased) {
			return nil
		}
		return err
	}
	if !p.target().Accepts(context) {
		return grid.ErrForeignPaint
	}
	return p.resource.record.wire.Placement(p.id).Erase(context)
}
