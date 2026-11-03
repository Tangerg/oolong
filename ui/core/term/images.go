package term

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"time"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/input"
)

var (
	ErrImageNumbersExhausted  = errors.New("term: image request numbers exhausted")
	ErrImageUploadCapacity    = errors.New("term: too many unanswered image uploads")
	ErrImageResponseTimeout   = errors.New("term: image creation was not acknowledged")
	ErrImageCleanupIncomplete = errors.New("term: image cleanup is incomplete")
)

const (
	// Cancelled requests retain capacity until a late allocation is deleted, so
	// silence and output backpressure cannot turn abandonment into unbounded state.
	maxImageUploads      = 32
	imageResponseTimeout = 10 * time.Second
)

// Request numbers correlate replies across successive sessions in this process.
// They never claim uniqueness in the terminal's physical image namespace.
var graphicsRequestNumbers atomicSequence

func nextGraphicsRequestNumber() (uint32, error) {
	value, ok := graphicsRequestNumbers.next(math.MaxUint32)
	if !ok {
		return 0, ErrImageNumbersExhausted
	}
	return uint32(value), nil //nolint:gosec // atomicSequence.next refuses values above the supplied 32-bit protocol bound.
}

// Result delivery and allocation ownership end independently: cancellation must
// leave a reply correlation until the terminal's allocation can be released.
type imageUpload struct {
	ctx     context.Context //nolint:containedctx // The request owns cancellation eligibility until its asynchronous terminal reply settles.
	until   time.Time
	size    image.Point
	result  chan graphics.UploadResult
	settled chan struct{}
	done    chan struct{}
	image   graphics.Image
}

// Transmit admits a PNG upload without waiting on the UI event consumer. Its
// buffered result channel settles once, then closes. Only a successful terminal
// acknowledgement creates a Resource; context cancellation ends the unfinished
// request, not a resource already returned. Read the result off the UI owner.
func (t *Terminal) Transmit(ctx context.Context, png []byte) (<-chan graphics.UploadResult, error) {
	if ctx == nil {
		return nil, errors.New("term: image upload context is required")
	}
	if !t.Graphics().Supports(graphics.Live) {
		return nil, errors.ErrUnsupported
	}
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, context.Cause(ctx)
	}
	if err := t.imageAdmission(len(png)); err != nil {
		return nil, err
	}
	number, err := nextGraphicsRequestNumber()
	if err != nil {
		return nil, err
	}
	var payload bytes.Buffer
	size, err := graphics.Upload(&payload, number, png)
	if err != nil {
		return nil, err
	}
	if _, err := t.writer.Queue(payload.Bytes()); err != nil {
		return nil, err
	}
	if t.uploads == nil {
		t.uploads = make(map[uint32]*imageUpload)
	}
	upload := &imageUpload{
		ctx: ctx, until: time.Now().Add(imageResponseTimeout), size: size,
		result: make(chan graphics.UploadResult, 1), settled: make(chan struct{}), done: make(chan struct{}),
	}
	t.uploads[number] = upload
	go t.watchUpload(ctx, number, upload.until, upload.settled)
	return upload.result, nil
}

func (t *Terminal) imageAdmission(sourceBytes int) error {
	select {
	case <-t.stop:
		return ErrClosed
	default:
	}
	select {
	case <-t.readerDone:
		return io.EOF
	default:
	}
	if len(t.uploads) >= maxImageUploads {
		return ErrImageUploadCapacity
	}
	if sourceBytes > t.writer.maxFrameBytes {
		return ErrPayloadTooLarge
	}
	return nil
}

func (t *Terminal) watchUpload(ctx context.Context, number uint32, until time.Time, settled <-chan struct{}) {
	timer := time.NewTimer(time.Until(until))
	defer timer.Stop()
	for {
		advanced := t.writer.advanceSignal()
		if err := t.writer.Err(); err != nil {
			t.cancelUpload(number, err)
			return
		}
		select {
		case <-settled:
			return
		case <-ctx.Done():
			t.cancelUpload(number, context.Cause(ctx))
			return
		case <-timer.C:
			t.cancelUpload(number, ErrImageResponseTimeout)
			return
		case <-advanced:
		}
	}
}

func (u *imageUpload) settle(result graphics.UploadResult) {
	if u.result == nil {
		return
	}
	u.result <- result
	close(u.result)
	u.result, u.ctx = nil, nil
	close(u.settled)
}

func (u *imageUpload) expire() {
	if u.ctx == nil {
		return
	}
	if u.ctx.Err() != nil {
		u.settle(graphics.UploadResult{Err: context.Cause(u.ctx)})
	} else if !time.Now().Before(u.until) {
		u.settle(graphics.UploadResult{Err: ErrImageResponseTimeout})
	}
}

func (t *Terminal) cancelUpload(number uint32, cause error) {
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	if upload := t.uploads[number]; upload != nil {
		upload.settle(graphics.UploadResult{Err: cause})
	}
}

// Cancelling a result cannot retire its physical allocation obligation. A late
// acknowledgement must still be deleted while this session owns the terminal.
func (t *Terminal) cancelUploads(cause error) {
	if cause == nil {
		cause = io.EOF
	}
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	for _, upload := range t.uploads {
		upload.settle(graphics.UploadResult{Err: cause})
	}
}

// Ordinary writer admission must be sealed before this snapshot: no later
// upload may cross the boundary after its outstanding obligations were checked.
// Input stays owned and active until every request has a resource or deletion.
func (t *Terminal) awaitUploads() error {
	t.modeMu.Lock()
	pending := make([]<-chan struct{}, 0, len(t.uploads))
	for _, upload := range t.uploads {
		pending = append(pending, upload.done)
	}
	t.modeMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	grace := time.NewTimer(DrainGrace)
	defer grace.Stop()
	for _, done := range pending {
		select {
		case <-done:
		case <-grace.C:
			return t.uploadCleanupError()
		case <-t.stop:
			return ErrClosed
		}
	}
	return nil
}

func (t *Terminal) uploadCleanupError() error {
	t.modeMu.Lock()
	pending := len(t.uploads)
	t.modeMu.Unlock()
	if pending == 0 {
		return nil
	}
	return errors.Join(fmt.Errorf("%w: %d pending upload(s)", ErrImageCleanupIncomplete, pending), t.writer.Err())
}

func (t *Terminal) retireUpload(number uint32) {
	close(t.uploads[number].done)
	delete(t.uploads, number)
}

func (t *Terminal) revokeUploads() {
	for number := range t.uploads {
		t.retireUpload(number)
	}
}

func (t *Terminal) takeImageReply(event input.Event) (bool, error) {
	apc, ok := event.(input.APC)
	if !ok {
		return false, nil
	}
	reply, parseErr := graphics.ParseReply(apc.Body)
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	upload := t.uploads[reply.Number]
	if upload == nil {
		return false, nil
	}
	if upload.image.ID != 0 {
		return true, graphics.ErrMalformedReply
	}
	upload.expire()
	if parseErr != nil || reply.Placement != 0 {
		upload.settle(graphics.UploadResult{Err: graphics.ErrMalformedReply})
		return true, nil
	}
	if err := reply.Err(); err != nil {
		upload.settle(graphics.UploadResult{Err: err})
		t.retireUpload(reply.Number)
		return true, nil
	}
	return true, t.acceptImage(reply, upload)
}

func (t *Terminal) acceptImage(reply graphics.Reply, upload *imageUpload) error {
	if t.imageOwner.Contains(reply.ID) || t.deletingImage(reply.ID) {
		upload.settle(graphics.UploadResult{Err: graphics.ErrMalformedReply})
		return nil
	}
	wire := graphics.Image{ID: reply.ID, Size: upload.size}
	if upload.result != nil {
		upload.settle(graphics.UploadResult{Resource: t.imageOwner.Bind(wire)})
		t.retireUpload(reply.Number)
		return nil
	}
	upload.image = wire
	err := deleteImage(wire, t.writer.QueueControl)
	if err == nil {
		t.retireUpload(reply.Number)
		return nil
	}
	if errors.Is(err, ErrQueueFull) {
		go t.retryImageDelete(reply.Number, upload.done)
		return nil
	}
	return fmt.Errorf("term: release cancelled image: %w", err)
}

func (t *Terminal) deletingImage(id uint32) bool {
	for _, upload := range t.uploads {
		if upload.image.ID == id {
			return true
		}
	}
	return false
}

func (t *Terminal) retryImageDelete(number uint32, done <-chan struct{}) {
	for {
		advanced := t.writer.advanceSignal()
		t.modeMu.Lock()
		upload := t.uploads[number]
		if upload == nil {
			t.modeMu.Unlock()
			return
		}
		err := deleteImage(upload.image, t.writer.QueueControl)
		if err == nil {
			t.retireUpload(number)
		}
		t.modeMu.Unlock()
		if !errors.Is(err, ErrQueueFull) {
			return
		}
		select {
		case <-advanced:
		case <-done:
			return
		}
	}
}

func (t *Terminal) releasePendingImages() error {
	for number, upload := range t.uploads {
		if upload.image.ID == 0 {
			continue
		}
		if err := t.writer.Drain(DrainGrace); err != nil {
			return err
		}
		if err := deleteImage(upload.image, t.writer.QueueControl); err != nil {
			return err
		}
		t.retireUpload(number)
	}
	return nil
}
