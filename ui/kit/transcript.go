package kit

import (
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// TranscriptConfig configures one base reading controller and the appearance of its existing scene element.
type TranscriptConfig struct {
	Behavior   base.TranscriptConfig
	Theme      Theme
	Scope      *ThemeScope
	Appearance ThemeOverride
	Glyphs     Glyphs
}

// Transcript contributes appearance to one base controller. Reading, selection,
// retention and publication are owned by that controller, including when a caller
// supplies a different skin for its element.
// A Transcript must not be copied after first use: its controller retains this appearance.
type Transcript struct {
	noCopy     noCopy
	controller *base.Transcript
	appearance appearance
	glyphs     Glyphs
}

// NewTranscript creates one controller and fills its paint slot before it can be
// attached. It panics if Theme and Scope are both specified, and for a Behavior
// [base.TranscriptConfig.Validate] refuses.
func NewTranscript(cfg TranscriptConfig) *Transcript {
	t := &Transcript{
		controller: base.NewTranscript(cfg.Behavior),
		appearance: newAppearance(cfg.Theme, cfg.Scope, cfg.Appearance),
		glyphs:     cfg.Glyphs.clone(),
	}
	element := t.controller.Element()
	t.appearance.bind(element)
	element.SetPaint(func(frame scene.Frame) { t.Draw(frame) })
	return t
}

func (t *Transcript) Controller() *base.Transcript {
	return t.controller
}

func (t *Transcript) Element() scene.Element {
	return t.controller.Element()
}

// Draw paints the transcript and returns the window it staged, as [Box.Draw] returns
// its interior. A paint slot that decorates the transcript — a marker beside the
// current item, a placeholder while it is empty — draws over those rows instead of
// staging a second window, which the frame forbids, or repainting selection and
// matches itself.
func (t *Transcript) Draw(frame scene.Frame) base.TranscriptView {
	window := t.controller.StageWindow(frame)
	if window.Pinned.Rows > 0 {
		t.drawHeader(window, frame.View)
		if !window.Header.Empty() {
			t.mark(frame.View.Sub(window.Header), window.HeaderFrom, window.Content)
		}
	}
	window.Content.Draw(window.Body, window.BodyFrom)
	t.mark(window.Body, window.BodyFrom, window.Content)
	return window
}

func (t *Transcript) drawHeader(window base.TranscriptView, view grid.View) {
	palette := t.appearance.palette()
	window.Content.Draw(view.Sub(window.Header), window.HeaderFrom)
	visible := window.Header.Intersect(view.Visible())
	for y := visible.Min.Y; y < visible.Max.Y; y++ {
		for x := visible.Min.X; x < visible.Max.X; x++ {
			restyle(view, x, y, palette.Surface)
		}
	}
	view.Fade(visible, 1-window.Pinned.Fade)
	headerRows := window.Pinned.Visible()
	if window.Pinned.Rows > headerRows {
		text.Of(t.glyphs.Rule, palette.Divider).Tile(view, headerRows)
	}
}

func (t *Transcript) mark(view grid.View, from int, content base.TranscriptLayout) {
	visible := view.Visible()
	if visible.Empty() {
		return
	}
	palette := t.appearance.palette()
	if rects, err := t.controller.Selection().Rects(content); err == nil {
		for _, rect := range rects {
			area := rect.Sub(image.Pt(0, from)).Intersect(visible)
			for y := area.Min.Y; y < area.Max.Y; y++ {
				for x := area.Min.X; x < area.Max.X; x++ {
					restyle(view, x, y, palette.Selection)
				}
			}
		}
	}
	t.markMatches(view, from, content, palette)
}

func (t *Transcript) markMatches(view grid.View, from int, content base.TranscriptLayout, palette Theme) {
	visible := view.Visible()
	first, matches := t.controller.MatchesIn(content, layout.Sum(from, visible.Min.Y), layout.Sum(from, visible.Max.Y))
	for i, match := range matches {
		style := palette.Selection
		if first+i == t.controller.CurrentMatch(content) {
			style = palette.Accent
		}
		for row, span := range match.Spans {
			y := layout.Relative(layout.Sum(match.Row, row), from)
			if y < visible.Min.Y || y >= visible.Max.Y {
				continue
			}
			end := min(layout.Sum(span.Col, span.Width), visible.Max.X)
			for x := max(span.Col, visible.Min.X); x < end; x++ {
				restyle(view, x, y, style)
			}
		}
	}
}

func restyle(view grid.View, x, y int, style grid.Style) {
	if style != (grid.Style{}) {
		view.MergeStyle(x, y, style)
	}
}

func (t *Transcript) Glyphs() Glyphs { return t.glyphs.clone() }

// SetGlyphs panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (t *Transcript) SetGlyphs(value Glyphs) { setting.Replace(t.Element(), &t.glyphs, value.clone()) }
