package keymap

import (
	"slices"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

// Matcher reads one [Map] for one event owner.
//
// It owns partial sequence progress, exact-prefix resolution and action dispatch.
// The zero value is ready to use. A Matcher belongs to one goroutine and must not be
// copied after first use. Changing bindings or switching maps abandons pending
// sequences; a delayed resolver cannot execute a binding from an older map version.
type Matcher struct {
	noCopy noCopy

	pending *sequence
	// A fresh identity on abandonment makes a late resolver inert without a
	// finite counter that could eventually revive it.
	ticket *byte
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
	m.ticket = new(byte)
	return cancel
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

func (m *Matcher) resolve(ticket *byte) {
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
