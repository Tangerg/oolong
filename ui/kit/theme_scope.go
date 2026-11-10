package kit

import (
	"errors"
	"math"
	"slices"
	"weak"

	"github.com/Tangerg/oolong/ui/core/grid"

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
}

// NewThemeOwner installs the application's initial palette. A zero palette keeps
// terminal default colours; it is a complete palette, not an absent configuration.
func NewThemeOwner(initial Theme) *ThemeOwner {
	return &ThemeOwner{palette: initial, revision: 1}
}

// Scope is the application's immutable root theme scope.
func (o *ThemeOwner) Scope() *ThemeScope { return &ThemeScope{owner: o} }

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
	// Register every wake before a scene predicate can interrupt publication.
	for _, weakConsumer := range slices.Backward(live) {
		if consumer := weakConsumer.Value(); consumer != nil {
			defer consumer.element.Invalidate()
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
	override ThemeOverride
}

// With creates a child scope. Explicit zero styles are retained, so an override
// can deliberately restore terminal defaults. The caller's pointers are copied.
func (s *ThemeScope) With(override ThemeOverride) *ThemeScope {
	return &ThemeScope{owner: s.owner, override: override.over(s.override)}
}

// Snapshot resolves the current application palette through this branch in
// application, parent scope, then child scope order. It has no side effects.
func (s *ThemeScope) Snapshot() ThemeSnapshot {
	return ThemeSnapshot{Palette: s.override.apply(s.owner.palette), Revision: s.owner.revision}
}

type themeConsumer struct{ element scene.Element }

type appearance struct {
	scope    ThemeScope
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
	if scope.owner == nil {
		return appearance{}, errors.New("kit: theme scope has no owner")
	}
	return appearance{scope: *scope.With(override)}, nil
}

func (a *appearance) bind(element scene.Element) {
	a.consumer = &themeConsumer{element: element}
	a.scope.owner.retain(a.consumer)
}

// rowStyle paints a list row's selection band and returns the style its text
// is drawn with. Every list skin shares this one selection policy, so a row
// reads as selected the same way wherever it is shown.
func (a *appearance) rowStyle(view grid.View, selected bool) grid.Style {
	palette := a.palette()
	if !selected {
		return palette.Text
	}
	view.Fill(view.Bounds(), palette.Selection)
	return palette.Text.Merge(palette.Selection)
}

func (a *appearance) palette() Theme {
	if a.scope.owner == nil {
		return Theme{}
	}
	return a.scope.Snapshot().Palette
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
