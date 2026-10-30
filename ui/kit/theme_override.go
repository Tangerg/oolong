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

func cloneToken[T any](token *T) *T {
	if token == nil {
		return nil
	}
	value := *token
	return &value
}

func applyToken[T any](token *T, current T) T {
	if token == nil {
		return current
	}
	return *token
}

func (o ThemeOverride) clone() ThemeOverride {
	return ThemeOverride{
		Text:      cloneToken(o.Text),
		Muted:     cloneToken(o.Muted),
		Subtle:    cloneToken(o.Subtle),
		Strong:    cloneToken(o.Strong),
		Heading:   cloneToken(o.Heading),
		Accent:    cloneToken(o.Accent),
		Success:   cloneToken(o.Success),
		Warning:   cloneToken(o.Warning),
		Danger:    cloneToken(o.Danger),
		Info:      cloneToken(o.Info),
		Border:    cloneToken(o.Border),
		Divider:   cloneToken(o.Divider),
		Selection: cloneToken(o.Selection),
		Surface:   cloneToken(o.Surface),
		Sunken:    cloneToken(o.Sunken),
		Added:     cloneToken(o.Added),
		Removed:   cloneToken(o.Removed),
		Context:   cloneToken(o.Context),
		Scrim:     cloneToken(o.Scrim),
	}
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
