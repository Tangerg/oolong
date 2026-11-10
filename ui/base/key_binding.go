package base

import (
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// The map owns bindings; this lease only connects its mutations to the consumer's
// scene phase and invalidation. Default maps stay private and immutable.
type keyBinding struct {
	current      *keymap.Map
	subscription *keymap.Subscription
}

// resolve is the map its consumer handles keys with: the consumer's own map, or
// its immutable defaults when none is set.
func (b *keyBinding) resolve(defaults func() *keymap.Map) *keymap.Map {
	if b.current != nil {
		return b.current
	}
	return defaults()
}

func (b *keyBinding) attach(element scene.Element, matcher *keymap.Matcher) {
	if b.current == nil || b.subscription != nil || element.IsZero() {
		return
	}
	var subscription *keymap.Subscription
	subscription = b.current.Subscribe(keymap.SubscriptionConfig{
		Check: element.CheckMutation,
		Publish: func() {
			if b.subscription == subscription {
				element.Invalidate()
			}
		},
		Changed: func() {
			// Closing a lease does not remove it from an already committed delivery.
			if b.subscription != subscription {
				return
			}
			matcher.ResetStale(nil)
		},
	})
	b.subscription = subscription
}

func (b *keyBinding) set(element scene.Element, matcher *keymap.Matcher, next *keymap.Map) {
	requireMutable(element)
	if b.current == next {
		return
	}
	matcher.Reset(func() {
		if b.subscription != nil {
			b.subscription.Close()
			b.subscription = nil
		}
		b.current = next
		b.attach(element, matcher)
		element.Invalidate()
	})
}
