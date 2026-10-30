package kit

import (
	"errors"
	"math"
	"weak"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// ThemeSnapshot is one immutable palette and the application theme version that
// produced it. Scope and instance overrides are resolved before returning it.
type ThemeSnapshot struct {
	Palette  Theme
	Revision uint64
}

// ThemeOwner is the only mutable application palette. Updates run on the UI owner
// turn, wake every attached consumer, and are rejected during scene presentation.
// A ThemeOwner must not be copied after first use.
type ThemeOwner struct {
	noCopy    noCopy
	palette   Theme
	revision  uint64
	consumers []weak.Pointer[themeConsumer]
	root      ThemeScope
}

// NewThemeOwner installs the application's initial palette. A zero palette keeps
// terminal default colours; it is a complete palette, not an absent configuration.
func NewThemeOwner(initial Theme) *ThemeOwner {
	owner := &ThemeOwner{palette: initial, revision: 1}
	owner.root.owner = owner
	return owner
}

// Scope is the application's immutable root theme scope.
func (o *ThemeOwner) Scope() *ThemeScope { return &o.root }

// Revision advances exactly once per changed application palette.
func (o *ThemeOwner) Revision() uint64 { return o.revision }

// Set changes the application palette and invalidates all live consumers. It
// returns false for an identical palette. It panics before changing any state if
// one of its consumers is being painted, observed, or has a pending scene frame,
// or when its revision space is exhausted.
func (o *ThemeOwner) Set(palette Theme) bool {
	if o.palette == palette {
		return false
	}
	if o.revision == math.MaxUint64 {
		panic("kit: theme revision exhausted")
	}
	live := make([]weak.Pointer[themeConsumer], 0, len(o.consumers))
	for _, weakConsumer := range o.consumers {
		consumer := weakConsumer.Value()
		if consumer == nil {
			continue
		}
		if err := consumer.element.CheckMutation(); err != nil {
			panic(err)
		}
		live = append(live, weakConsumer)
	}
	clear(o.consumers[len(live):])
	o.consumers = live
	o.palette = palette
	o.revision++
	for _, weakConsumer := range live {
		if consumer := weakConsumer.Value(); consumer != nil {
			consumer.element.Invalidate()
		}
	}
	return true
}

// ThemeScope is an immutable branch of the application's token cascade. Pass the
// same scope to a composed subtree; With creates a more specific branch without
// copying mutable theme state. Behaviour and interaction eligibility are never
// theme tokens.
type ThemeScope struct {
	owner    *ThemeOwner
	parent   *ThemeScope
	override ThemeOverride
}

// With creates a child scope. Explicit zero styles are retained, so an override
// can deliberately restore terminal defaults. The caller's pointers are copied.
func (s *ThemeScope) With(override ThemeOverride) *ThemeScope {
	return &ThemeScope{owner: s.owner, parent: s, override: override.clone()}
}

// Snapshot resolves the current application palette through this branch in
// application, parent scope, then child scope order. It has no side effects.
func (s *ThemeScope) Snapshot() ThemeSnapshot {
	palette := s.owner.palette
	var apply func(*ThemeScope)
	apply = func(scope *ThemeScope) {
		if scope.parent != nil {
			apply(scope.parent)
		}
		palette = scope.override.apply(palette)
	}
	apply(s)
	return ThemeSnapshot{Palette: palette, Revision: s.owner.revision}
}

type themeConsumer struct{ element scene.Element }

type appearance struct {
	scope    *ThemeScope
	override ThemeOverride
	consumer *themeConsumer
}

// newAppearance accepts either an application's scope or a complete static
// palette. The latter receives a private owner; no component stores a writable
// copy of a live application palette.
func newAppearance(palette Theme, scope *ThemeScope, override ThemeOverride) (appearance, error) {
	if scope != nil && palette != (Theme{}) {
		return appearance{}, errors.New("kit: Theme and Scope are mutually exclusive")
	}
	if scope == nil {
		scope = NewThemeOwner(palette).Scope()
	}
	return appearance{scope: scope, override: override.clone()}, nil
}

func (a *appearance) bind(element scene.Element) {
	a.consumer = &themeConsumer{element: element}
	a.scope.owner.retain(a.consumer)
}

func (a *appearance) palette() Theme {
	if a.scope == nil {
		return Theme{}
	}
	return a.override.apply(a.scope.Snapshot().Palette)
}

func (o *ThemeOwner) retain(consumer *themeConsumer) {
	if len(o.consumers) == cap(o.consumers) {
		live := o.consumers[:0]
		for _, reference := range o.consumers {
			if reference.Value() != nil {
				live = append(live, reference)
			}
		}
		clear(o.consumers[len(live):])
		o.consumers = live
	}
	o.consumers = append(o.consumers, weak.Make(consumer))
}
