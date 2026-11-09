package keymap

import (
	"errors"
	"slices"
	"weak"

	"github.com/Tangerg/oolong/ui/internal/retention"
)

var (
	ErrChecking   = errors.New("keymap: mutation during consumer checks")
	ErrPublishing = errors.New("keymap: mutation during consumer publication")
)

type mapPhase uint8

const (
	mapIdle mapPhase = iota
	mapChecking
	mapPublishing
)

type SubscriptionConfig struct {
	// Check is pure and refuses a mutation before any binding or policy changes.
	// Every live consumer must permit the change; a refusal panics with its error.
	// Mutating or subscribing during a check panics with ErrChecking.
	Check func() error
	// Publish updates the consumer's projection of the committed map. Pure
	// projection predicates are allowed; reentrant application notifications are
	// not. All consumers publish before Changed runs;
	// mutating or subscribing during publication panics with ErrPublishing.
	// Nil requires no publication before notifications.
	Publish func()
	// Changed runs synchronously after publication and may reenter the map. Delivery
	// already owned by that commit survives a later Close, panic, or Goexit.
	Changed func()
}

// Subscription belongs to a consumer. The map retains it weakly, so sharing a
// long-lived map cannot retain abandoned consumers. Calls use the map's owner
// goroutine; a subscription must not be copied after first use.
type Subscription struct {
	noCopy noCopy
	config SubscriptionConfig
}

// Subscribe borrows map changes until the returned subscription is closed or
// becomes unreachable. It panics if Check or Changed is nil, with ErrChecking
// during consumer checks, or with ErrPublishing during publication. A nil Map
// returns nil after validating the callbacks.
func (m *Map) Subscribe(config SubscriptionConfig) *Subscription {
	if config.Check == nil || config.Changed == nil {
		panic("keymap: subscription requires Check and Changed")
	}
	if m == nil {
		return nil
	}
	m.requireIdle()
	if len(m.subscribers) == cap(m.subscribers) {
		m.pruneSubscribers()
	}
	subscription := &Subscription{config: config}
	m.subscribers = append(m.subscribers, weak.Make(subscription))
	return subscription
}

func (s *Subscription) Close() {
	if s != nil {
		s.config = SubscriptionConfig{}
	}
}

func (m *Map) pruneSubscribers() {
	live := m.subscribers[:0]
	for _, reference := range m.subscribers {
		if subscription := reference.Value(); subscription != nil && subscription.config.Changed != nil {
			live = append(live, reference)
		}
	}
	clear(m.subscribers[len(live):])
	switch {
	case len(live) == 0:
		m.subscribers = nil
	case retention.ShouldCompact(cap(live), len(live)):
		m.subscribers = slices.Clone(live)
	default:
		m.subscribers = live
	}
}

func (m *Map) checkSubscribers() []SubscriptionConfig {
	m.requireIdle()
	m.phase = mapChecking
	defer func() { m.phase = mapIdle }()
	m.pruneSubscribers()
	var subscribers []SubscriptionConfig
	for _, reference := range m.subscribers {
		if subscription := reference.Value(); subscription != nil {
			subscribers = append(subscribers, subscription.config)
		}
	}
	for _, subscriber := range subscribers {
		if err := subscriber.Check(); err != nil {
			panic(err)
		}
	}
	return subscribers
}

func (m *Map) requireIdle() {
	switch m.phase {
	case mapIdle:
		return
	case mapChecking:
		panic(ErrChecking)
	case mapPublishing:
		panic(ErrPublishing)
	}
}

func (m *Map) notifySubscribers(subscribers []SubscriptionConfig) {
	defer notifyChanges(subscribers)
	m.phase = mapPublishing
	defer func() { m.phase = mapIdle }()
	for _, subscriber := range slices.Backward(subscribers) {
		if subscriber.Publish != nil {
			defer subscriber.Publish()
		}
	}
}

func notifyChanges(subscribers []SubscriptionConfig) {
	for _, subscriber := range slices.Backward(subscribers) {
		defer subscriber.Changed()
	}
}
