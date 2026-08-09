package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
)

func TestBuiltInThemePalettes(t *testing.T) {
	tests := []struct {
		name string
		got  kit.Theme
		want kit.Theme
	}{
		{
			name: "dark",
			got:  kit.Dark(),
			want: theme(
				rgb(0xE1, 0xE1, 0xE1),
				rgb(0x6C, 0x6C, 0x6C),
				rgb(0x58, 0x58, 0x58),
				rgb(0x7A, 0xA2, 0xF7),
				rgb(0x9E, 0xCE, 0x6A),
				rgb(0xE0, 0xAF, 0x68),
				rgb(0xF7, 0x76, 0x8E),
				rgb(0x7D, 0xCF, 0xFF),
				rgb(0x32, 0x32, 0x37),
				rgb(0x14, 0x14, 0x14),
				rgb(0x1C, 0x1C, 0x1C),
				rgb(0x36, 0x36, 0x36),
				rgb(0x06, 0x38, 0x06),
				rgb(0x42, 0x0E, 0x14),
			),
		},
		{
			name: "light",
			got:  kit.Light(),
			want: theme(
				rgb(0x26, 0x26, 0x26),
				rgb(0x76, 0x76, 0x76),
				rgb(0xA5, 0xA5, 0xA5),
				rgb(0x2F, 0x64, 0xD2),
				rgb(0x37, 0x8E, 0x23),
				rgb(0xA2, 0x76, 0x12),
				rgb(0xCD, 0x30, 0x48),
				rgb(0x00, 0x82, 0xAA),
				rgb(0xC8, 0xC8, 0xCD),
				rgb(0xEE, 0xEE, 0xEE),
				rgb(0xE4, 0xE4, 0xE4),
				rgb(0xC6, 0xC6, 0xC6),
				rgb(0xDA, 0xF2, 0xDC),
				rgb(0xF5, 0xDA, 0xDE),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("theme = %#v, want %#v", tt.got, tt.want)
			}
		})
	}
}

func TestSuitedThemeUsesTerminalNeutrals(t *testing.T) {
	ground := grid.Ground{
		FG: rgb(0xE8, 0xDC, 0xC8),
		BG: rgb(0x18, 0x20, 0x28),
	}
	got := kit.Suited(ground)
	dark := kit.Dark()

	if !got.Text.FG.Default() || !got.Strong.FG.Default() || !got.Heading.FG.Default() {
		t.Error("body roles override the terminal foreground")
	}
	if got.Muted.FG != ground.BG.Blend(ground.FG, 0.44) {
		t.Errorf("muted foreground = %#v", got.Muted.FG)
	}
	if got.Subtle.FG != ground.BG.Blend(ground.FG, 0.30) {
		t.Errorf("subtle foreground = %#v", got.Subtle.FG)
	}
	if got.Border.FG != ground.BG.Blend(ground.FG, 0.16) || got.Divider != got.Border {
		t.Error("structural lines do not share the terminal-derived colour")
	}
	if got.Surface.BG != ground.BG.Blend(ground.FG, 0.04) {
		t.Errorf("surface background = %#v", got.Surface.BG)
	}
	if got.Sunken.BG != ground.BG.Blend(ground.FG, 0.08) {
		t.Errorf("sunken background = %#v", got.Sunken.BG)
	}
	if got.Selection.BG != ground.BG.Blend(ground.FG, 0.18) {
		t.Errorf("selection background = %#v", got.Selection.BG)
	}
	if got.Added.BG != ground.BG.Blend(dark.Success.FG, 0.18) {
		t.Errorf("added background = %#v", got.Added.BG)
	}
	if got.Removed.BG != ground.BG.Blend(dark.Danger.FG, 0.18) {
		t.Errorf("removed background = %#v", got.Removed.BG)
	}
	if got.Accent != dark.Accent || got.Success != dark.Success || got.Warning != dark.Warning || got.Danger != dark.Danger || got.Info != dark.Info {
		t.Error("semantic colours changed while fitting neutral roles")
	}
	if got.Scrim != (kit.Scrim{Color: ground.BG, Opacity: 0.55}) {
		t.Errorf("scrim = %#v", got.Scrim)
	}
}

func TestSuitedThemeChangesWithTerminalPalette(t *testing.T) {
	a := kit.Suited(grid.Ground{FG: rgb(0xF0, 0xF0, 0xF0), BG: rgb(0x10, 0x10, 0x10)})
	b := kit.Suited(grid.Ground{FG: rgb(0xD0, 0xC0, 0xB0), BG: rgb(0x20, 0x18, 0x10)})

	if a.Muted == b.Muted || a.Surface == b.Surface || a.Selection == b.Selection {
		t.Error("different terminal palettes produced the same neutral roles")
	}
}

func theme(
	text, muted, subtle, accent,
	success, warning, danger, info,
	line, surface, sunken, selected,
	added, removed grid.Color,
) kit.Theme {
	return kit.Theme{
		Text:      grid.Style{FG: text},
		Muted:     grid.Style{FG: muted},
		Subtle:    grid.Style{FG: subtle},
		Strong:    grid.Style{FG: text, Attr: grid.Bold},
		Heading:   grid.Style{FG: text, Attr: grid.Bold},
		Accent:    grid.Style{FG: accent},
		Success:   grid.Style{FG: success},
		Warning:   grid.Style{FG: warning},
		Danger:    grid.Style{FG: danger},
		Info:      grid.Style{FG: info},
		Border:    grid.Style{FG: line},
		Divider:   grid.Style{FG: line},
		Selection: grid.Style{BG: selected},
		Surface:   grid.Style{BG: surface},
		Sunken:    grid.Style{BG: sunken},
		Added:     grid.Style{FG: success, BG: added},
		Removed:   grid.Style{FG: danger, BG: removed},
		Context:   grid.Style{FG: muted},
		Scrim:     kit.Scrim{Color: rgb(0, 0, 0), Opacity: 0.5},
	}
}

func rgb(r, g, b uint8) grid.Color { return grid.RGBColor(r, g, b) }
