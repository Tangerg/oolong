// Package keymap maps terminal keystrokes to named actions.
//
// Lookups do not mutate a [Map]. Sequence progress lives in [Pending], so a map can
// be shared by independent readers without sharing their partially typed sequences,
// provided its bindings are not changed concurrently.
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

// Pending records progress through one reader's partially typed sequence.
// Its zero value has no progress.
type Pending struct {
	keys input.Keys
	at   time.Time
}

// Keys returns a copy of the chords typed so far.
func (p *Pending) Keys() input.Keys {
	if p == nil {
		return nil
	}
	return slices.Clone(p.keys)
}

// Clear abandons the partially typed sequence.
func (p *Pending) Clear() {
	if p != nil {
		p.keys, p.at = nil, time.Time{}
	}
}

// current returns the current prefix, or nil when it expired. Events with no
// arrival time do not expire because no elapsed time can be inferred from them.
func (p *Pending) current(timeout time.Duration, now time.Time) input.Keys {
	if p == nil || len(p.keys) == 0 {
		return nil
	}
	if !p.at.IsZero() && !now.IsZero() {
		elapsed := now.Sub(p.at)
		if elapsed < 0 || elapsed > timeout {
			return nil
		}
	}
	return p.keys
}

// Binding associates a chord sequence with an action.
type Binding struct {
	Keys   input.Keys
	Action Action
}

// String returns the chord sequence followed by its action identifier.
func (b Binding) String() string { return b.Keys.String() + " " + b.Action.String() }

// Map associates chord sequences with actions.
//
// A sequence that is a proper prefix of another binding is unreachable: without a
// timer driving lookup, the shorter binding cannot be chosen while the longer one
// may still arrive. The longer sequence therefore takes precedence.
//
// The zero value is an empty map. Use Map by pointer after binding keys; copying a
// populated Map would share its internal tree.
type Map struct {
	// Timeout controls how long a partially typed sequence remains current. Zero
	// and negative values use [DefaultTimeout].
	Timeout time.Duration

	bound []Binding
	root  *trieNode
}

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

// Bindings returns a deep copy of every binding, in binding order.
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

// Lookup advances pending with key and returns the completed action and whether the
// key belongs to this map. A sequence prefix belongs to the map even though its
// returned action is empty. A nil Pending can resolve only single-chord bindings.
func (m *Map) Lookup(key input.Key, pending *Pending) (Action, bool) {
	if m == nil || !key.Down() {
		return "", false
	}
	chord := key.Chord()
	prefix := pending.current(m.timeout(), key.At)
	node, ok := m.follow(append(slices.Clone(prefix), chord))
	if !ok && len(prefix) > 0 {
		prefix = nil
		node, ok = m.follow(input.Keys{chord})
	}
	pending.Clear()
	if !ok {
		return "", false
	}
	if len(node.next) > 0 {
		if pending == nil {
			return "", false
		}
		pending.keys = append(slices.Clone(prefix), chord)
		pending.at = key.At
		return "", true
	}
	if node.action == "" {
		return "", false
	}
	return node.action, true
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
