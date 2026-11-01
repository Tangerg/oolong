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

func applyToken[T any](token *T, current T) T {
	if token == nil {
		return current
	}
	return *token
}

// Inherited tokens are already owned by an immutable scope. Only explicit
// overrides borrow pointers from the caller and need their own snapshot.
func (o ThemeOverride) over(parent ThemeOverride) ThemeOverride {
	freezeToken(&parent.Text, o.Text)
	freezeToken(&parent.Muted, o.Muted)
	freezeToken(&parent.Subtle, o.Subtle)
	freezeToken(&parent.Strong, o.Strong)
	freezeToken(&parent.Heading, o.Heading)
	freezeToken(&parent.Accent, o.Accent)
	freezeToken(&parent.Success, o.Success)
	freezeToken(&parent.Warning, o.Warning)
	freezeToken(&parent.Danger, o.Danger)
	freezeToken(&parent.Info, o.Info)
	freezeToken(&parent.Border, o.Border)
	freezeToken(&parent.Divider, o.Divider)
	freezeToken(&parent.Selection, o.Selection)
	freezeToken(&parent.Surface, o.Surface)
	freezeToken(&parent.Sunken, o.Sunken)
	freezeToken(&parent.Added, o.Added)
	freezeToken(&parent.Removed, o.Removed)
	freezeToken(&parent.Context, o.Context)
	freezeToken(&parent.Scrim, o.Scrim)
	return parent
}

func (o ThemeOverride) apply(palette Theme) Theme {
	return Theme{
		Text:      applyToken(o.Text, palette.Text),
		Muted:     applyToken(o.Muted, palette.Muted),
		Subtle:    applyToken(o.Subtle, palette.Subtle),
		Strong:    applyToken(o.Strong, palette.Strong),
		Heading:   applyToken(o.Heading, palette.Heading),
		Accent:    applyToken(o.Accent, palette.Accent),
		Success:   applyToken(o.Success, palette.Success),
		Warning:   applyToken(o.Warning, palette.Warning),
		Danger:    applyToken(o.Danger, palette.Danger),
		Info:      applyToken(o.Info, palette.Info),
		Border:    applyToken(o.Border, palette.Border),
		Divider:   applyToken(o.Divider, palette.Divider),
		Selection: applyToken(o.Selection, palette.Selection),
		Surface:   applyToken(o.Surface, palette.Surface),
		Sunken:    applyToken(o.Sunken, palette.Sunken),
		Added:     applyToken(o.Added, palette.Added),
		Removed:   applyToken(o.Removed, palette.Removed),
		Context:   applyToken(o.Context, palette.Context),
		Scrim:     applyToken(o.Scrim, palette.Scrim),
	}
}
