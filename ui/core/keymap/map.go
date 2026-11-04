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
)

// Action identifies an operation independently of the keystrokes bound to it.
// Empty is reserved for no action.
type Action string

func (a Action) String() string { return string(a) }

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
// copied after first use. Changing bindings or switching maps abandons pending
// sequences; a delayed resolver cannot execute a binding from an older map version.
type Matcher struct {
	noCopy noCopy

	bindings *Map
	version  *byte
	keys     input.Keys
	at       time.Time
	exact    Action
	do       func(Action) bool
	current  func() bool
	cancel   func()
	ticket   uint64
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
	m.Reset(nil)
}

// Reset abandons matching before change replaces the consumer's state, and cancels
// the old resolver after change returns. Cancellation may reenter the consumer; it
// sees the completed change and cannot erase matching started by that change.
// The cancellation also runs when change panics. Nil change only clears matching;
// a nil Matcher still runs change.
// Current reports whether matching and the old map stayed unchanged through change
// and cancellation. Input callers must stop dispatch when a newer sequence or map
// supersedes this reset. A nil Matcher reports true after running change.
func (m *Matcher) Reset(change func()) (current bool) {
	if m == nil {
		if change != nil {
			change()
		}
		return true
	}
	bindings := m.bindings
	var version *byte
	if bindings != nil {
		version = bindings.version
	}
	cancel := m.abandon()
	ticket := m.ticket
	defer func() {
		if cancel != nil {
			cancel()
		}
		current = m.ticket == ticket && (bindings == nil || bindings.version == version)
	}()
	if change != nil {
		change()
	}
	return true
}

func (m *Matcher) abandon() func() {
	cancel := m.cancel
	m.keys, m.at, m.exact, m.do, m.current, m.cancel = nil, time.Time{}, "", nil, nil, nil
	m.bindings, m.version = nil, nil
	m.ticket++
	return cancel
}

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

	bound   []Binding
	root    *trieNode
	version *byte
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

// Handle advances the matcher with key and runs a completed action through do.
// current is an optional pure lifetime receipt captured for this input. Once it
// becomes false it must stay false; the matcher cannot observe transitions between
// calls. A revoked sequence cannot continue or dispatch, even if its resolver was
// already queued. Nil leaves lifetime control with the direct caller.
// Matched reports whether this key belonged to bindings. Handled reports the
// callback's answer, except that an unfinished prefix is always handled.
//
// A prefix belongs to the map before it names an action. If an earlier ambiguous
// exact match is settled while this key is being read, this key is still matched
// independently and the result reports whether the new key belongs. A nil Matcher,
// Map or callback handles nothing.
// If resolving the older prefix cancels this matcher, replaces its map, or starts
// another sequence, the interrupted key is consumed without further dispatch.
func (m *Matcher) Handle(bindings *Map, key input.Key, current func() bool, do func(Action) bool) (matched, handled bool) {
	if m == nil || !key.Down() {
		return false, false
	}
	if bindings == nil || do == nil {
		m.Clear()
		return false, false
	}
	if !m.prepare(bindings, key.At, current) {
		return true, true
	}

	prefix := m.keys
	node, chord := bindings.matchKey(prefix, key)
	if node == nil && len(prefix) > 0 {
		// The key proves the longer binding will not arrive. Settle an exact prefix,
		// if there was one, then give this key a fresh reading of its own.
		if m.resolveExact() {
			return true, true
		}
		prefix = nil
		node, chord = bindings.matchKey(nil, key)
	}
	if node == nil {
		m.Clear()
		return false, false
	}

	if !m.Reset(nil) {
		return true, true
	}
	if !lifetimeCurrent(current) {
		return true, true
	}
	if len(node.next) == 0 {
		if node.action == "" {
			return false, false
		}
		return true, do(node.action)
	}

	prefix = append(prefix, chord)
	m.keys = prefix
	m.at = key.At
	m.bindings, m.version = bindings, bindings.version
	m.current = current
	if node.action != "" {
		m.deferExact(bindings, node.action, do)
	}
	return true, true
}

// Settle the previous sequence before matching another key. Its cancellation or
// exact-prefix callback may replace the matcher, revoking the interrupted dispatch.
func (m *Matcher) prepare(bindings *Map, at time.Time, current func() bool) bool {
	if m.bindings != nil && (m.bindings != bindings || m.version != bindings.version || !lifetimeCurrent(m.current)) && !m.Reset(nil) {
		return false
	}
	if !lifetimeCurrent(current) {
		m.Clear()
		return false
	}
	return !m.expired(bindings.timeout(), at) || !m.resolveExact()
}

func lifetimeCurrent(current func() bool) bool { return current == nil || current() }

// expired reports whether timed input can no longer continue the held sequence.
// A chord arriving exactly at the timeout is still in the sequence: Timeout is an
// inclusive interaction window, while a Resolver callback is an owner-queue task
// that settles the prefix once that window has elapsed without another chord.
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

func (m *Matcher) resolveExact() bool {
	if m == nil {
		return false
	}
	action, do, current := m.exact, m.do, m.current
	bindings, version := m.bindings, m.version
	if m.bindings == nil || m.version != m.bindings.version {
		do = nil
	}
	if !m.Reset(nil) {
		return true
	}
	ticket := m.ticket
	if action != "" && do != nil && lifetimeCurrent(current) {
		do(action)
	}
	return m.ticket != ticket || bindings != nil && bindings.version != version || !lifetimeCurrent(current)
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

func (m *Map) timeout() time.Duration {
	if m.Timeout > 0 {
		return m.Timeout
	}
	return DefaultTimeout
}
