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

	"github.com/Tangerg/oolong/core/input"
)

// Action identifies an operation independently of the keystrokes bound to it.
// Empty is reserved for no action.
type Action string

// String returns the action identifier.
func (a Action) String() string { return string(a) }

// DefaultTimeout is how long a partially typed sequence remains current when a
// [Map] does not specify a positive Timeout.
const DefaultTimeout = time.Second

// Resolver schedules resolve after wait and returns a function that cancels it.
//
// It is the policy seam for a binding that is both an exact match and a prefix of a
// longer binding. An event owner's one-shot scheduler has this shape; keymap neither
// imports that owner nor owns a clock. Resolve must run on the same goroutine that
// calls [Matcher.Handle]. Calling it after cancellation is harmless.
type Resolver func(wait time.Duration, resolve func()) (cancel func())

// Matcher reads one [Map] for one event owner.
//
// It owns partial sequence progress, exact-prefix resolution and action dispatch.
// The zero value is ready to use. A Matcher belongs to one goroutine and must not be
// copied after first use.
type Matcher struct {
	noCopy noCopy

	keys   input.Keys
	at     time.Time
	exact  Action
	do     func(Action) bool
	cancel func()
	ticket uint64
}

// Keys returns a copy of the chords typed so far.
func (m *Matcher) Keys() input.Keys {
	if m == nil {
		return nil
	}
	return slices.Clone(m.keys)
}

// Clear abandons the partially typed sequence and cancels its resolver.
func (m *Matcher) Clear() {
	if m == nil {
		return
	}
	cancel := m.cancel
	m.keys, m.at, m.exact, m.do, m.cancel = nil, time.Time{}, "", nil, nil
	m.ticket++
	if cancel != nil {
		cancel()
	}
}

// Binding associates a chord sequence with an action. Values returned by
// [Map.Bindings] are caller-owned snapshots, so an application may sort, present or
// serialize them without changing the map.
type Binding struct {
	Keys   input.Keys
	Action Action
}

// String returns the chord sequence followed by its action identifier.
func (b Binding) String() string { return b.Keys.String() + " " + b.Action.String() }

// Map associates chord sequences with actions.
//
// The zero value is an empty map. A Map must not be copied after first use: a copy
// would share the binding store and lookup tree while rebuilding only one of them.
type Map struct {
	noCopy noCopy

	// Timeout controls how long a partially typed sequence remains current. Zero
	// and negative values use [DefaultTimeout].
	Timeout time.Duration
	// Resolve decides when an exact binding that is also a prefix should run. Nil
	// waits for another key: a continuation takes the longer binding, while a key
	// that cannot continue it settles the exact binding first. Assign the event
	// owner's one-shot scheduler to make the exact binding run after Timeout even
	// when no further input arrives.
	Resolve Resolver

	bound []Binding
	root  *trieNode
}

// noCopy makes the ownership contract above visible to go vet. Its methods are
// never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Bind associates keys with action, replacing an existing binding for the same
// sequence. Empty actions and empty sequences are ignored.
func (m *Map) Bind(action Action, keys ...input.Chord) {
	if m == nil || action == "" || len(keys) == 0 {
		return
	}
	sequence := input.Keys(slices.Clone(keys))
	m.bound = slices.DeleteFunc(m.bound, func(binding Binding) bool {
		return slices.Equal(binding.Keys, sequence)
	})
	m.bound = append(m.bound, Binding{
		Keys: sequence, Action: Action(strings.Clone(action.String())),
	})
	m.rebuild()
}

// Unbind removes a sequence and reports whether it was bound.
func (m *Map) Unbind(keys ...input.Chord) bool {
	if m == nil {
		return false
	}
	before := len(m.bound)
	m.bound = slices.DeleteFunc(m.bound, func(binding Binding) bool {
		return slices.Equal(binding.Keys, input.Keys(keys))
	})
	if len(m.bound) == before {
		return false
	}
	m.trim()
	m.rebuild()
	return true
}

// trim releases a registry high-water mark once it is no longer useful. The slack
// keeps ordinary bind/unbind edits amortized while a map reduced far below an old
// peak stops carrying the peak for its complete lifetime.
func (m *Map) trim() {
	if len(m.bound) == 0 {
		m.bound = nil
		return
	}
	if cap(m.bound) > 2*len(m.bound)+16 {
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

// Action returns the action named by a complete sequence.
func (m *Map) Action(keys ...input.Chord) (Action, bool) {
	node, ok := m.follow(keys)
	if !ok || node.action == "" {
		return "", false
	}
	return node.action, true
}

// Handle advances the matcher with key and runs a completed action through do.
// Matched reports whether this key belonged to bindings. Handled reports the
// callback's answer, except that an unfinished prefix is always handled.
//
// A prefix belongs to the map before it names an action. If an earlier ambiguous
// exact match is settled while this key is being read, this key is still matched
// independently and the result reports whether the new key belongs. A nil Matcher,
// Map or callback handles nothing.
func (m *Matcher) Handle(bindings *Map, key input.Key, do func(Action) bool) (matched, handled bool) {
	if m == nil || bindings == nil || do == nil || !key.Down() {
		return false, false
	}
	if m.expired(bindings.timeout(), key.At) {
		m.resolveExact()
	}

	chord := key.Chord()
	prefix := slices.Clone(m.keys)
	node, ok := bindings.follow(append(prefix, chord))
	if !ok && len(prefix) > 0 {
		// The key proves the longer binding will not arrive. Settle an exact prefix,
		// if there was one, then give this key a fresh reading of its own.
		m.resolveExact()
		prefix = nil
		node, ok = bindings.follow(input.Keys{chord})
	}
	if !ok {
		m.Clear()
		return false, false
	}

	m.Clear()
	if len(node.next) == 0 {
		if node.action == "" {
			return false, false
		}
		return true, do(node.action)
	}

	prefix = append(prefix, chord)
	m.keys = prefix
	m.at = key.At
	if node.action != "" {
		m.deferExact(bindings, node.action, do)
	}
	return true, true
}

// expired reports whether timed input can no longer continue the held sequence.
// Synthetic events with no arrival time never expire because no elapsed time can be
// inferred from them.
func (m *Matcher) expired(timeout time.Duration, now time.Time) bool {
	if len(m.keys) == 0 || m.at.IsZero() || now.IsZero() {
		return false
	}
	elapsed := now.Sub(m.at)
	return elapsed < 0 || elapsed > timeout
}

// deferExact remembers the action at an ambiguous node and asks the map's policy to
// settle it. The ticket makes a late callback from a cancelled resolver inert.
func (m *Matcher) deferExact(bindings *Map, action Action, do func(Action) bool) {
	m.exact, m.do = action, do
	if bindings.Resolve == nil {
		return
	}
	ticket := m.ticket
	cancel := bindings.Resolve(bindings.timeout(), func() { m.resolve(ticket) })
	if m.ticket == ticket && len(m.keys) > 0 {
		m.cancel = cancel
	} else if cancel != nil {
		// A resolver is allowed to resolve synchronously. If it also returns a
		// cancellation function, nothing should retain the already-finished work.
		cancel()
	}
}

func (m *Matcher) resolve(ticket uint64) {
	if m == nil || m.ticket != ticket {
		return
	}
	m.resolveExact()
}

func (m *Matcher) resolveExact() {
	if m == nil {
		return
	}
	action, do := m.exact, m.do
	m.Clear()
	if action != "" && do != nil {
		do(action)
	}
}

type trieNode struct {
	next   map[input.Chord]*trieNode
	action Action
}

func (m *Map) follow(keys input.Keys) (*trieNode, bool) {
	if m == nil || m.root == nil || len(keys) == 0 {
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
}

func (m *Map) timeout() time.Duration {
	if m.Timeout > 0 {
		return m.Timeout
	}
	return DefaultTimeout
}
