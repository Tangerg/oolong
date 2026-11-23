// Package keymap maps terminal keystrokes to named actions.
//
// Matching does not mutate a [Map]. Sequence progress lives in [Matcher], so a map
// can be shared by independent readers without sharing their partially typed
// sequences, provided its bindings are not changed concurrently.
package keymap

import (
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/internal/retention"
	"github.com/Tangerg/oolong/ui/internal/weakset"
)

// Action identifies an operation independently of the keystrokes bound to it.
// Empty is reserved for no action.
type Action string

func (a Action) String() string { return string(a) }

// requireAction refuses the empty action, which names no operation and so cannot
// be the target of a binding.
func requireAction(action Action) {
	if action == "" {
		panic("keymap: empty action")
	}
}

const DefaultTimeout = time.Second

// Resolver schedules resolve after wait and returns a function that cancels it.
//
// It is the policy seam for a binding that is both an exact match and a prefix of a
// longer binding. An event owner's one-shot scheduler has this shape; keymap neither
// imports that owner nor owns a clock. Resolve must run on the same goroutine that
// calls [Matcher.Handle]. Calling it after cancellation is harmless.
type Resolver func(wait time.Duration, resolve func()) (cancel func())

// Binding associates a chord sequence with an action. Values returned by
// [Map.Bindings] are caller-owned snapshots, so an application may sort, present or
// serialize them without changing the map.
type Binding struct {
	Keys   input.Keys
	Action Action
}

func (b Binding) String() string { return b.Keys.String() + " " + b.Action.String() }

// Map associates chord sequences with actions.
//
// The zero value is an empty map. Like a built-in map, a nil *Map reads as empty and
// panics when changed. A Map must not be copied after first use: a copy
// would share the binding store and lookup tree while rebuilding only one of them.
// Binding and policy changes require every consumer's Check to succeed before
// commit; a refusal panics with that error. All consumers Publish the committed
// state before any Changed notification runs. Mutating or subscribing during
// checks panics with ErrChecking; during publication it panics with ErrPublishing.
type Map struct {
	noCopy noCopy

	sequenceTimeout time.Duration
	resolver        Resolver
	bound           []Binding
	root            *trieNode
	version         *byte
	subscribers     weakset.Set[Subscription]
	phase           mapPhase
}

// noCopy makes the ownership contract above visible to go vet. Its methods are
// never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Bind associates keys with action, replacing an existing binding for the same
// sequence. An empty sequence and a binding the map already holds change nothing,
// so they neither notify consumers nor abandon a pending sequence. An empty action,
// which is reserved for no action, and an invalid chord panic before any consumer
// checks or mutation.
func (m *Map) Bind(action Action, keys ...input.Chord) {
	requireAction(action)
	if len(keys) == 0 {
		return
	}
	for _, chord := range keys {
		if err := chord.Validate(); err != nil {
			panic(err)
		}
	}
	if slices.ContainsFunc(m.bound, func(binding Binding) bool {
		return binding.Action == action && slices.Equal(binding.Keys, input.Keys(keys))
	}) {
		return
	}
	subscribers := m.checkSubscribers()
	sequence := input.Keys(slices.Clone(keys))
	m.bound = slices.DeleteFunc(m.bound, func(binding Binding) bool {
		return slices.Equal(binding.Keys, sequence)
	})
	m.bound = append(m.bound, Binding{
		Keys: sequence, Action: Action(strings.Clone(action.String())),
	})
	m.rebuild()
	m.notifySubscribers(subscribers)
}

func (m *Map) Unbind(keys ...input.Chord) bool {
	index := slices.IndexFunc(m.bound, func(binding Binding) bool {
		return slices.Equal(binding.Keys, input.Keys(keys))
	})
	if index < 0 {
		return false
	}
	subscribers := m.checkSubscribers()
	m.bound = slices.Delete(m.bound, index, index+1)
	m.trim()
	m.rebuild()
	m.notifySubscribers(subscribers)
	return true
}

// Rebind replaces every sequence bound to action with sequences, in one change, which
// is what applying a user's configured shortcuts means: the defaults it replaces stop
// working and the configured order becomes the order help reads. A sequence bound to
// another action moves to this one, as with [Map.Bind]. No sequences unbinds the
// action; empty sequences are ignored. An empty action and an invalid chord panic
// before any consumer checks or mutation. A result equal to the current bindings
// changes nothing.
func (m *Map) Rebind(action Action, sequences ...input.Keys) {
	requireAction(action)
	var replacement []Binding
	for _, keys := range sequences {
		for _, chord := range keys {
			if err := chord.Validate(); err != nil {
				panic(err)
			}
		}
		if len(keys) == 0 || slices.ContainsFunc(replacement, func(binding Binding) bool { return slices.Equal(binding.Keys, keys) }) {
			continue
		}
		replacement = append(replacement, Binding{Keys: slices.Clone(keys), Action: Action(strings.Clone(action.String()))})
	}
	bound := slices.DeleteFunc(slices.Clone(m.bound), func(binding Binding) bool {
		return binding.Action == action || slices.ContainsFunc(replacement, func(next Binding) bool { return slices.Equal(next.Keys, binding.Keys) })
	})
	bound = append(bound, replacement...)
	if slices.EqualFunc(bound, m.bound, func(a, b Binding) bool { return a.Action == b.Action && slices.Equal(a.Keys, b.Keys) }) {
		return
	}
	subscribers := m.checkSubscribers()
	m.bound = bound
	m.trim()
	m.rebuild()
	m.notifySubscribers(subscribers)
}

// trim releases a registry high-water mark once it is no longer useful. The slack
// keeps ordinary bind/unbind edits amortized while a map reduced far below an old
// peak stops carrying the peak for its complete lifetime.
func (m *Map) trim() {
	if len(m.bound) == 0 {
		m.bound = nil
		return
	}
	if retention.ShouldCompact(cap(m.bound), len(m.bound)) {
		m.bound = slices.Clone(m.bound)
	}
}

// Keys returns copies of the sequences bound to action, in binding order.
func (m *Map) Keys(action Action) []input.Keys {
	if m == nil {
		return nil
	}
	var keys []input.Keys
	for _, binding := range m.bound {
		if binding.Action == action {
			keys = append(keys, slices.Clone(binding.Keys))
		}
	}
	return keys
}

// Bindings returns a deep copy of every binding, in binding order. It is the
// complete observation surface for applications that present or persist a key map;
// use [Map.Keys] when only one action is relevant.
func (m *Map) Bindings() []Binding {
	if m == nil {
		return nil
	}
	bindings := make([]Binding, len(m.bound))
	for i, binding := range m.bound {
		bindings[i] = Binding{
			Keys:   slices.Clone(binding.Keys),
			Action: binding.Action,
		}
	}
	return bindings
}

// Continuations returns copies of the bindings that a typed prefix can still complete,
// in binding order, so an interface can show what may follow a partial sequence such
// as [Matcher.Keys]. Each binding keeps its complete sequence; the prefix itself is
// not a continuation even when it names an action of its own.
func (m *Map) Continuations(prefix input.Keys) []Binding {
	if m == nil {
		return nil
	}
	var continuations []Binding
	for _, binding := range m.bound {
		if len(binding.Keys) > len(prefix) && slices.Equal(binding.Keys[:len(prefix)], prefix) {
			continuations = append(continuations, Binding{Keys: slices.Clone(binding.Keys), Action: binding.Action})
		}
	}
	return continuations
}

func (m *Map) Action(keys ...input.Chord) (Action, bool) {
	node, ok := m.follow(keys)
	if !ok || node.action == "" {
		return "", false
	}
	return node.action, true
}

// ActionFor resolves a single press or repeat using the same layout-aware
// precedence as Matcher.Handle. It never consumes a sequence prefix.
func (m *Map) ActionFor(key input.Key) (Action, bool) {
	if !key.Down() {
		return "", false
	}
	node, _ := m.matchKey(nil, key)
	if node == nil || node.action == "" {
		return "", false
	}
	return node.action, true
}

type trieNode struct {
	next   map[input.Chord]*trieNode
	action Action
}

func (m *Map) matchKey(prefix input.Keys, key input.Key) (*trieNode, input.Chord) {
	parent, ok := m.follow(prefix)
	if ok {
		for chord := range key.ShortcutChords() {
			if child := parent.next[chord]; child != nil {
				return child, chord
			}
		}
	}
	return nil, input.Chord{}
}

func (m *Map) follow(keys input.Keys) (*trieNode, bool) {
	if m == nil || m.root == nil {
		return nil, false
	}
	node := m.root
	for _, chord := range keys {
		next, ok := node.next[chord]
		if !ok {
			return nil, false
		}
		node = next
	}
	return node, true
}

func (m *Map) rebuild() {
	root := &trieNode{}
	for _, binding := range m.bound {
		node := root
		for _, chord := range binding.Keys {
			if node.next == nil {
				node.next = make(map[input.Chord]*trieNode)
			}
			next, ok := node.next[chord]
			if !ok {
				next = &trieNode{}
				node.next[chord] = next
			}
			node = next
		}
		node.action = binding.Action
	}
	m.root = root
	m.version = new(byte)
}

func (m *Map) Timeout() time.Duration {
	if m != nil && m.sequenceTimeout > 0 {
		return m.sequenceTimeout
	}
	return DefaultTimeout
}

// SetTimeout invalidates pending sequences with the old timing policy. Nonpositive
// durations select DefaultTimeout, including for the zero Map.
func (m *Map) SetTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if m.Timeout() == timeout {
		return
	}
	subscribers := m.checkSubscribers()
	m.sequenceTimeout = timeout
	m.version = new(byte)
	m.notifySubscribers(subscribers)
}

func (m *Map) Resolver() Resolver {
	if m == nil {
		return nil
	}
	return m.resolver
}

// SetResolver replaces exact-prefix scheduling and revokes old pending sequences.
// Nil waits for a further key to settle an exact prefix.
func (m *Map) SetResolver(resolver Resolver) {
	if m.resolver == nil && resolver == nil {
		return
	}
	subscribers := m.checkSubscribers()
	m.resolver = resolver
	m.version = new(byte)
	m.notifySubscribers(subscribers)
}
