package main

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/program"
)

// imageLedger owns every diagram image the terminal accepted until a release
// succeeds; a refused release keeps the image for the next replacement or close.
// Upload workers record a resource before its delivery can be revoked, so the
// ledger is shared with them and guarded by its own lock.
type imageLedger struct {
	host program.ImageHost
	mu   sync.Mutex
	live map[graphics.Resource]struct{}
}

// receive waits for one upload and records its resource. A cancelled receiver
// releases the resource itself: its revoked delivery was the only other owner.
func (l *imageLedger) receive(ctx context.Context, upload <-chan graphics.UploadResult) (graphics.Resource, error) {
	outcome := <-upload
	l.mu.Lock()
	defer l.mu.Unlock()
	if outcome.Resource.Live() {
		if l.live == nil {
			l.live = make(map[graphics.Resource]struct{})
		}
		l.live[outcome.Resource] = struct{}{}
	}
	if ctx.Err() != nil {
		return graphics.Resource{}, errors.Join(context.Cause(ctx), l.release(outcome.Resource))
	}
	return outcome.Resource, outcome.Err
}

func (l *imageLedger) releaseAll() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var failures []error
	for resource := range l.live {
		failures = append(failures, l.release(resource))
	}
	return errors.Join(failures...)
}

func (l *imageLedger) release(resource graphics.Resource) error {
	if resource.Live() {
		if err := l.host.ReleaseImage(resource); err != nil {
			return err
		}
	}
	delete(l.live, resource)
	return nil
}
