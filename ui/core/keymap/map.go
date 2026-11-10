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
	"weak"

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

	pending *sequence
	// ticket advances whenever matching is abandoned, so a resolver or callback
	// from an earlier sequence can tell it has been superseded.
	ticket uint64
}

// sequence is one partially typed chord sequence. It exists only while a prefix
// waits for its next chord and is replaced as a whole, so its parts can never
// describe different sequences.
type sequence struct {
	bindings *Map
	version  *byte
	keys     input.Keys
	at       time.Time
	current  func() bool
	// exact is the action the prefix already names, settled by the resolver or by
	// a chord that proves no longer binding will arrive.
	exact  Action
	do     func(Action) bool
	cancel func()
}

// Keys returns a copy of the chords typed so far.
func (m *Matcher) Keys() input.Keys {
	if m == nil || m.pending == nil {
		return nil
	}
	return slices.Clone(m.pending.keys)
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
	var bindings *Map
	var version *byte
	if m.pending != nil {
		bindings = m.pending.bindings
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

// ResetStale cancels matching only when its map has changed. A delayed map
// notification cannot erase a sequence already started at the current version.
// Change runs in either case, with the same cancellation ordering as Reset.
func (m *Matcher) ResetStale(change func()) {
	if m != nil && m.pending != nil && m.pending.version != m.pending.bindings.version {
		m.Reset(change)
		return
	}
	if change != nil {
		change()
	}
}

func (m *Matcher) abandon() func() {
	var cancel func()
	if m.pending != nil {
		cancel = m.pending.cancel
	}
	m.pending = nil
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
	subscribers     []weak.Pointer[Subscription]
	phase           mapPhase
}

// noCopy makes the ownership contract above visible to go vet. Its methods are
// never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Bind associates keys with action, replacing an existing binding for the same
// sequence. Empty actions, empty sequences and a binding the map already holds
// change nothing, so they neither notify consumers nor abandon a pending sequence.
func (m *Map) Bind(action Action, keys ...input.Chord) {
	if m == nil || action == "" || len(keys) == 0 {
		return
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
	if m == nil {
		return false
	}
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

	var prefix input.Keys
	if m.pending != nil {
		prefix = m.pending.keys
	}
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
		return true, do(node.action)
	}

	m.pending = &sequence{
		bindings: bindings, version: bindings.version,
		keys: append(prefix, chord), at: key.At, current: current,
	}
	if node.action != "" {
		m.deferExact(bindings, node.action, do)
	}
	return true, true
}

// Settle the previous sequence before matching another key. Its cancellation or
// exact-prefix callback may replace the matcher, revoking the interrupted dispatch.
func (m *Matcher) prepare(bindings *Map, at time.Time, current func() bool) bool {
	if p := m.pending; p != nil && (p.bindings != bindings || p.version != bindings.version || !lifetimeCurrent(p.current)) && !m.Reset(nil) {
		return false
	}
	if !lifetimeCurrent(current) {
		m.Clear()
		return false
	}
	return !m.expired(bindings.Timeout(), at) || !m.resolveExact()
}

func lifetimeCurrent(current func() bool) bool { return current == nil || current() }

// expired reports whether timed input can no longer continue the held sequence.
// A chord arriving exactly at the timeout is still in the sequence: Timeout is an
// inclusive interaction window, while a Resolver callback is an owner-queue task
// that settles the prefix once that window has elapsed without another chord.
// Synthetic events with no arrival time never expire because no elapsed time can be
// inferred from them.
func (m *Matcher) expired(timeout time.Duration, now time.Time) bool {
	if m.pending == nil || m.pending.at.IsZero() || now.IsZero() {
		return false
	}
	elapsed := now.Sub(m.pending.at)
	return elapsed < 0 || elapsed > timeout
}

// deferExact remembers the action at an ambiguous node and asks the map's policy to
// settle it. The ticket makes a late callback from a cancelled resolver inert.
func (m *Matcher) deferExact(bindings *Map, action Action, do func(Action) bool) {
	m.pending.exact, m.pending.do = action, do
	if bindings.resolver == nil {
		return
	}
	ticket := m.ticket
	returned := false
	defer func() {
		// A scheduler can retain resolve before unwinding. Revoke that unfinished
		// dispatch without clearing a newer sequence started by scheduler reentry.
		if !returned && m.ticket == ticket {
			m.Clear()
		}
	}()
	cancel := bindings.resolver(bindings.Timeout(), func() { m.resolve(ticket) })
	returned = true
	if m.ticket == ticket && m.pending != nil {
		m.pending.cancel = cancel
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
	var action Action
	var do func(Action) bool
	var current func() bool
	var bindings *Map
	var version *byte
	if p := m.pending; p != nil {
		action, current, bindings, version = p.exact, p.current, p.bindings, p.version
		if p.version == p.bindings.version {
			do = p.do
		}
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
	if m == nil || m.Timeout() == timeout {
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
	if m == nil || m.resolver == nil && resolver == nil {
		return
	}
	subscribers := m.checkSubscribers()
	m.resolver = resolver
	m.version = new(byte)
	m.notifySubscribers(subscribers)
}
