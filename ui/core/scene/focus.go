package scene

import "slices"

func (r *Root) enabled(n *node) bool {
	if n == nil || n.root != r {
		return false
	}
	for ancestor := n; ancestor != nil; ancestor = ancestor.parent {
		if ancestor.config.Enabled != nil && !r.project(ancestor.config.Enabled) {
			return false
		}
	}
	return true
}

func (r *Root) allowed(n *node) bool {
	return n != nil && r.allowedIn(n, r.modal())
}

func (r *Root) allowedIn(n, modal *node) bool { return r.eligibilityIn(n, modal) == eligible }

// eligibility is whether a node can take input now, and if not, why not.
type eligibility uint8

func (e eligibility) reason() string {
	switch e {
	case disabled:
		return "target is disabled"
	case outsideModal:
		return "target is outside the active modal layer"
	default:
		return ""
	}
}

func (r *Root) eligibilityIn(n, modal *node) eligibility {
	if !r.enabled(n) {
		return disabled
	}
	if modal == nil {
		return eligible
	}
	for ancestor := n; ancestor != nil; ancestor = ancestor.parent {
		if ancestor == modal {
			return eligible
		}
	}
	return outsideModal
}

func (r *Root) walkModals(visit func(*node)) {
	r.walk(r.content.node, func(n *node) bool {
		if n.group != nil && n.group.arrange.modal && len(n.group.items) != 0 && r.enabled(n) {
			visit(n)
		}
		return true
	})
}

func (r *Root) modal() *node {
	var current *node
	r.walkModals(func(n *node) { current = n })
	return current
}

func (r *Root) focusableIn(n, modal *node) bool {
	return r.allowedIn(n, modal) && n.config.Focusable != nil && r.project(n.config.Focusable)
}

// Focus returns the single current keyboard target, or zero when none is eligible.
func (r *Root) Focus() Target {
	if r == nil {
		return Target{}
	}
	return r.focus
}

// RequestFocus focuses an eligible current target and defers notifications until state has settled.
func (r *Root) RequestFocus(target Target) error {
	if err := r.mutable(); err != nil {
		return err
	}
	n := r.resolve(target)
	if n == nil {
		return ErrStale
	}
	active := r.reconcileModals()
	if !r.focusableIn(n, active) {
		return ErrUnavailable
	}
	r.setFocus(target, r.keyboardRoute(target, active))
	return nil
}

func (r *Root) setFocus(target Target, keyboard []Target) {
	r.recordKeyboardRoute(keyboard)
	if r.focus == target {
		return
	}
	previous := r.focus
	r.focus = target
	r.focusTurn = new(byte)
	if old := r.nodes[previous.node]; previous.root == r && old != nil && old.generation == previous.generation {
		if notify := old.config.FocusChanged; notify != nil {
			r.notify(old, func() { notify(false) })
		}
	}
	if n := r.resolve(target); n != nil && n.config.FocusChanged != nil {
		notify := n.config.FocusChanged
		turn := r.focusTurn
		r.notify(n, func() {
			if r.focusTurn == turn {
				notify(true)
			}
		})
	}
	r.modelRevision = next(r.modelRevision)
	r.signal()
}

func (r *Root) keyboardRoute(focus Target, modal *node) []Target {
	origin := r.resolve(focus)
	if origin == nil {
		origin = modal
		if origin == nil {
			origin = r.content.node
		}
	}
	var current []Target
	if r.allowedIn(origin, modal) {
		for n := origin; n != nil; n = n.parent {
			current = append(current, n.target())
			if n == modal {
				break
			}
		}
	}
	return current
}

// History can revoke earlier scopes but cannot authorize input. Input and leases
// derive current eligibility from focus, modals and the composition in keyboardRoute.
func (r *Root) recordKeyboardRoute(current []Target) {
	for _, previous := range r.previousKeyboardRoute {
		if !slices.Contains(current, previous) {
			if n := r.resolve(previous); n != nil {
				n.keyboardEpoch = new(byte)
			}
		}
	}
	r.previousKeyboardRoute = current
}

func containsNode(ancestor, descendant *node) bool {
	for n := descendant; n != nil; n = n.parent {
		if n == ancestor {
			return true
		}
	}
	return false
}

// Each modal layer remembers the focus to restore when that layer disappears.
type focusScope struct{ modal, opener Target }

func (r *Root) settleFocus() {
	settled := false
	defer func() {
		// Eligibility callbacks may exit after the composition changes. Without a
		// completed route, returning to the old composition must not revive input.
		if !settled {
			r.recordKeyboardRoute(nil)
		}
	}()
	active := r.reconcileModals()
	if n := r.resolve(r.focus); n != nil && r.focusableIn(n, active) {
		r.recordKeyboardRoute(r.keyboardRoute(r.focus, active))
		settled = true
		return
	}
	first := r.firstFocusable(active, autofocused)
	if first.IsZero() {
		first = r.firstFocusable(active, func(*node) bool { return true })
	}
	r.setFocus(first, r.keyboardRoute(first, active))
	settled = true
}

// firstFocusable is the first focusable target in tree order that wanted admits.
func (r *Root) firstFocusable(active *node, wanted func(*node) bool) Target {
	var first Target
	r.walk(r.content.node, func(n *node) bool {
		if r.focusableIn(n, active) && wanted(n) {
			first = n.target()
			return false
		}
		return true
	})
	return first
}

// autofocused reports whether n is inside an element where focus asks to start.
func autofocused(n *node) bool {
	for ; n != nil; n = n.parent {
		if n.config.Autofocus {
			return true
		}
	}
	return false
}

// reconcileModals brings the remembered layers up to the live modal stack and
// returns its top. A new layer records the current focus as its opener, so it runs
// before anything moves focus into that layer; a layer that has gone returns focus
// to its opener.
func (r *Root) reconcileModals() (active *node) {
	var modals []Target
	r.walkModals(func(n *node) {
		active = n
		modals = append(modals, n.target())
	})
	common := 0
	for common < len(modals) && common < len(r.scopes) && modals[common] == r.scopes[common].modal {
		common++
	}
	if common < len(r.scopes) || common < len(modals) {
		// A layer's last focus is the next layer's opener, or the current focus
		// for the top layer. Carry it by attachment when layers move. Removing a
		// covered layer splices its opener into the surviving return path.
		within := make(map[Target]Target, len(r.scopes)-common)
		opener := r.focus
		for i := len(r.scopes) - 1; i >= common; i-- {
			scope := r.scopes[i]
			within[scope.modal] = opener
			opener = scope.opener
		}
		r.scopes = slices.Delete(r.scopes, common, len(r.scopes))
		for _, modal := range modals[common:] {
			r.scopes = append(r.scopes, focusScope{modal: modal, opener: opener})
			opener = within[modal]
		}
		if opener != r.focus {
			if n := r.resolve(opener); n != nil && r.focusableIn(n, active) {
				r.setFocus(opener, r.keyboardRoute(opener, active))
			}
		}
	}
	return active
}

// FocusNext moves focus to the next eligible target, wrapping after the last.
// Composite controllers may request a particular target instead to implement
// roving or validation-error policies.
func (r *Root) FocusNext() bool { return r.cycleFocus(1) }

// FocusPrev moves focus to the previous eligible target, wrapping before the first.
func (r *Root) FocusPrev() bool { return r.cycleFocus(-1) }

// cycleFocus steps one eligible target in direction, which is 1 or -1. Without a
// current target, a step forward lands on the first and a step back on the last.
func (r *Root) cycleFocus(direction int) bool {
	if r.mutable() != nil {
		return false
	}
	var targets []Target
	active := r.reconcileModals()
	r.walk(r.content.node, func(n *node) bool {
		if r.focusableIn(n, active) {
			targets = append(targets, n.target())
		}
		return true
	})
	if len(targets) == 0 {
		return false
	}
	at := -1
	for i, target := range targets {
		if target == r.focus {
			at = i
			break
		}
	}
	if at < 0 && direction < 0 {
		at = len(targets)
	}
	at = (at + direction + len(targets)) % len(targets)
	r.setFocus(targets[at], r.keyboardRoute(targets[at], active))
	return true
}

func (r *Root) walk(n *node, visit func(*node) bool) bool {
	if n == nil || n.root != r {
		return true
	}
	if !visit(n) {
		return false
	}
	if n.group != nil {
		for _, item := range n.group.items {
			if !r.walk(item.Element.node, visit) {
				return false
			}
		}
	}
	return true
}
