package kit

import "github.com/Tangerg/oolong/ui/core/grid"

// ThemeOverride describes only the roles explicitly replaced by one scope or
// instance. A non-nil pointer to a zero style deliberately selects terminal
// defaults. Construction copies the pointed-to values.
type ThemeOverride struct {
	Text      *grid.Style
	Muted     *grid.Style
	Subtle    *grid.Style
	Strong    *grid.Style
	Heading   *grid.Style
	Accent    *grid.Style
	Success   *grid.Style
	Warning   *grid.Style
	Danger    *grid.Style
	Info      *grid.Style
	Border    *grid.Style
	Divider   *grid.Style
	Selection *grid.Style
	Surface   *grid.Style
	Sunken    *grid.Style
	Added     *grid.Style
	Removed   *grid.Style
	Context   *grid.Style
	Scrim     *Scrim
}

func freezeToken[T any](dst **T, explicit *T) {
	if explicit != nil {
		value := *explicit
		*dst = &value
	}
}

// overrideToken replaces one inherited role. A role nobody overrides keeps its
// inherited value, so a role added to Theme but forgotten here is inherited
// rather than silently erased.
func overrideToken[T any](dst *T, token *T) {
	if token != nil {
		*dst = *token
	}
}

// role pairs where an override keeps one palette role's token with where a
// palette keeps its value.
type role[T any] struct {
	token **T
	value *T
}

// roles lists every palette role once. over and apply both walk it, so a role
// added to Theme and ThemeOverride needs adding in exactly one more place.
func roles(o *ThemeOverride, t *Theme) ([]role[grid.Style], role[Scrim]) {
	return []role[grid.Style]{
		{&o.Text, &t.Text},
		{&o.Muted, &t.Muted},
		{&o.Subtle, &t.Subtle},
		{&o.Strong, &t.Strong},
		{&o.Heading, &t.Heading},
		{&o.Accent, &t.Accent},
		{&o.Success, &t.Success},
		{&o.Warning, &t.Warning},
		{&o.Danger, &t.Danger},
		{&o.Info, &t.Info},
		{&o.Border, &t.Border},
		{&o.Divider, &t.Divider},
		{&o.Selection, &t.Selection},
		{&o.Surface, &t.Surface},
		{&o.Sunken, &t.Sunken},
		{&o.Added, &t.Added},
		{&o.Removed, &t.Removed},
		{&o.Context, &t.Context},
	}, role[Scrim]{&o.Scrim, &t.Scrim}
}

// Inherited tokens are already owned by an immutable scope. Only explicit
// overrides borrow pointers from the caller and need their own snapshot.
func (o ThemeOverride) over(parent ThemeOverride) ThemeOverride {
	var unused Theme
	inherited, inheritedScrim := roles(&parent, &unused)
	explicit, explicitScrim := roles(&o, &unused)
	for i := range inherited {
		freezeToken(inherited[i].token, *explicit[i].token)
	}
	freezeToken(inheritedScrim.token, *explicitScrim.token)
	return parent
}

func (o ThemeOverride) apply(palette Theme) Theme {
	styles, scrim := roles(&o, &palette)
	for _, r := range styles {
		overrideToken(r.value, *r.token)
	}
	overrideToken(scrim.value, *scrim.token)
	return palette
}
