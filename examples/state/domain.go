package main

import (
	"context"
	"errors"
	"math"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// notificationSettings is the domain owner, independent of either UI control.
// Changes and subscriptions run on the UI owner; background work enters through
// Dispatcher.Invoke. A field reads this version and never retains a writable copy.
type notificationSettings struct {
	state       base.ConfirmSnapshot
	subscribers map[*notificationSubscription]struct{}
}

type notificationSubscription struct{ changed func() }

func newNotificationSettings() *notificationSettings {
	return &notificationSettings{state: base.ConfirmSnapshot{Revision: 1}, subscribers: make(map[*notificationSubscription]struct{})}
}

func (s *notificationSettings) Snapshot() base.ConfirmSnapshot { return s.state }

func (s *notificationSettings) Change(ctx context.Context, request base.ConfirmRequest) (scene.ActionResult[base.ConfirmSnapshot], error) {
	if err := ctx.Err(); err != nil {
		return scene.ActionResult[base.ConfirmSnapshot]{}, err
	}
	if request.ExpectedRevision != s.state.Revision {
		return scene.ActionResult[base.ConfirmSnapshot]{}, base.ErrConfirmStale
	}
	if request.Value == s.state.Value {
		return scene.ActionResult[base.ConfirmSnapshot]{State: scene.Unchanged, Value: s.state}, nil
	}
	if s.state.Revision == math.MaxUint64 {
		return scene.ActionResult[base.ConfirmSnapshot]{}, errors.New("notification revision exhausted")
	}
	s.state = base.ConfirmSnapshot{Value: request.Value, Revision: s.state.Revision + 1}
	applied := s.state
	pending := make([]*notificationSubscription, 0, len(s.subscribers))
	for subscriber := range s.subscribers {
		pending = append(pending, subscriber)
	}
	for _, subscriber := range pending {
		if subscriber.changed != nil {
			subscriber.changed()
		}
	}
	return scene.ActionResult[base.ConfirmSnapshot]{State: scene.Applied, Value: applied}, nil
}

func (s *notificationSettings) Subscribe(changed func()) func() {
	subscriber := &notificationSubscription{changed: changed}
	s.subscribers[subscriber] = struct{}{}
	return func() {
		delete(s.subscribers, subscriber)
		subscriber.changed = nil
	}
}
