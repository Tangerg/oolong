package kit

import (
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// TranscriptConfig configures one base reading controller and the appearance of its existing scene element.
type TranscriptConfig struct {
	Key            string
	Name           string
	Publisher      *publication.Publisher
	Retention      base.Retention
	PublishPartial bool
	KeepCompleted  int
	MaxItems       int
	Keys           *keymap.Map
	Theme          Theme
	Scope          *ThemeScope
	Appearance     ThemeOverride
	Glyphs         Glyphs
}

// Transcript contributes appearance to one base controller. Reading, selection,
// retention and publication are owned by that controller, including when a caller
// supplies a different skin for its element.
type Transcript struct {
	controller *base.Transcript
	appearance appearance
	Glyphs     Glyphs
}

// NewTranscript creates one controller and fills its paint slot before it can be attached.
// A nonzero static Theme and a live Scope are mutually exclusive.
func NewTranscript(cfg TranscriptConfig) (*Transcript, error) {
	look, err := newAppearance(cfg.Theme, cfg.Scope, cfg.Appearance)
	if err != nil {
		return nil, err
	}
	controller, err := base.NewTranscript(base.TranscriptConfig{Key: cfg.Key, Name: cfg.Name, Publisher: cfg.Publisher, Retention: cfg.Retention, PublishPartial: cfg.PublishPartial, KeepCompleted: cfg.KeepCompleted, MaxItems: cfg.MaxItems, Keys: cfg.Keys})
	if err != nil {
		return nil, err
	}
	t := &Transcript{controller: controller, appearance: look, Glyphs: cfg.Glyphs}
	t.appearance.bind(controller.Element())
	if err := controller.Element().SetPaint(t.Draw); err != nil {
		return nil, err
	}
	return t, nil
}

// Controller exposes the sole owner of transcript content, reading and publication policy.
func (t *Transcript) Controller() *base.Transcript {
	if t == nil {
		return nil
	}
	return t.controller
}

// Element returns the controller attachment, preserving identity through appearance changes.
func (t *Transcript) Element() scene.Element {
	if t == nil || t.controller == nil {
		return scene.Element{}
	}
	return t.controller.Element()
}

// Draw composes the controller candidate with header, selection and search appearance without changing behavior.
func (t *Transcript) Draw(frame scene.Frame) {
	if t == nil || t.controller == nil {
		return
	}
	window := t.controller.StageWindow(frame)
	if window.Pinned.Rows > 0 {
		t.drawHeader(window.Content, frame.View, window.Pinned)
		if !window.Header.Empty() {
			t.mark(frame.View.Sub(window.Header), window.HeaderFrom, window.Content)
		}
	}
	window.Content.Draw(window.Body, window.From)
	t.mark(window.Body, window.From, window.Content)
}

func (t *Transcript) drawHeader(content base.TranscriptLayout, view grid.View, pinned base.Pinned) {
	palette := t.appearance.palette()
	width, _ := view.Size()
	block := content.Block(pinned.Block)
	if block == nil {
		return
	}
	visible := pinned.Visible()
	block.Draw(view.Sub(image.Rect(0, -pinned.ClipTop, width, visible)))
	for y := range visible {
		for x := range width {
			restyle(view, x, y, palette.Surface)
		}
	}
	view.Fade(image.Rect(0, 0, width, visible), 1-pinned.Fade)
	if t.Glyphs.Horizontal != "" && pinned.Rows > visible {
		for x := range width {
			view.Text(x, visible, t.Glyphs.Horizontal, palette.Divider)
		}
	}
}

func (t *Transcript) mark(view grid.View, from int, content base.TranscriptLayout) {
	palette := t.appearance.palette()
	selection := t.controller.Selection()
	width, height := view.Size()
	if rects, err := selection.Rects(content); err == nil {
		for _, rect := range rects {
			for y := max(rect.Min.Y-from, 0); y < min(rect.Max.Y-from, height); y++ {
				for x := max(rect.Min.X, 0); x < min(rect.Max.X, width); x++ {
					restyle(view, x, y, palette.Selection)
				}
			}
		}
	}
	t.markMatches(view, from, palette)
}

func (t *Transcript) markMatches(view grid.View, from int, palette Theme) {
	width, height := view.Size()
	first, matches := t.controller.MatchesIn(from, layout.Sum(from, height))
	for i, match := range matches {
		style := palette.Selection
		if first+i == t.controller.CurrentMatch() {
			style = palette.Accent
		}
		for row, span := range match.Spans {
			y := layout.Relative(layout.Sum(match.Row, row), from)
			if y < 0 || y >= height {
				continue
			}
			start := max(span.Col, 0)
			end := min(layout.Sum(start, span.Width), width)
			for x := start; x < end; x++ {
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
