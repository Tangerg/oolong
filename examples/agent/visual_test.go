package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/examples/internal/visualtest"
)

var visualGround = grid.Ground{
	FG: grid.RGBColor(0xE8, 0xDC, 0xC8),
	BG: grid.RGBColor(0x18, 0x20, 0x28),
}

func runVisualAgent(t *testing.T, width, height int, depth grid.Depth) (*visualtest.Host, func()) {
	t.Helper()
	host := visualtest.New(t, width, height, visualGround)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- program.Run(ctx, program.Config{
			Host:  host,
			Color: depth,
			Inline: func(runtime *program.InlineRuntime) program.Component {
				return headless.NewRoot(newAgent(runtime, mockBackend{}))
			},
		})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("visual agent stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func TestReviewScreenAtRepresentativeWidths(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{name: "narrow", width: 44, height: 18},
		{name: "wide", width: 90, height: 24},
	} {
		t.Run(size.name, func(t *testing.T) {
			host, stop := runVisualAgent(t, size.width, size.height, grid.TrueColor)
			defer stop()
			host.Shows(t, "Ask the mock agent")
			host.Type("move validation into its owner")
			host.Press(input.Enter)
			host.Shows(t, "Allow this tool call?")

			capture := host.Capture(t)
			visualtest.Match(t, filepath.Join("testdata", size.name+"-review.golden"), capture.Rows)
		})
	}
}

func TestColorDepthChangesEncodingWithoutChangingLayout(t *testing.T) {
	tests := []struct {
		name  string
		depth grid.Depth
		want  string
		avoid []string
	}{
		{name: "truecolor", depth: grid.TrueColor, want: ";38;2;"},
		{name: "256", depth: grid.Depth256, want: ";38;5;", avoid: []string{";38;2;", ";48;2;"}},
		{name: "16", depth: grid.Depth16, want: "\x1b[0;", avoid: []string{";38;2;", ";48;2;", ";38;5;", ";48;5;"}},
		{name: "none", depth: grid.NoColor, want: "\x1b[0;1m", avoid: []string{";38;", ";48;"}},
	}

	var rows string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, stop := runVisualAgent(t, 72, 18, tt.depth)
			host.Shows(t, "Ask the mock agent")
			capture := host.Capture(t)
			stop()

			if !strings.Contains(capture.Encoding, tt.want) {
				t.Errorf("frame at %s depth does not contain %q: %q", tt.name, tt.want, capture.Encoding)
			}
			for _, forbidden := range tt.avoid {
				if strings.Contains(capture.Encoding, forbidden) {
					t.Errorf("frame at %s depth contains %q: %q", tt.name, forbidden, capture.Encoding)
				}
			}

			got := strings.Join(capture.Rows, "\n")
			if rows == "" {
				rows = got
			} else if got != rows {
				t.Error("colour depth changed the screen geometry or text")
			}
		})
	}
}
