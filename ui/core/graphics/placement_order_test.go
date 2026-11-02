package graphics_test

import (
	"bytes"
	"errors"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestNativeCompositionOrdersImagesIndependentlyOfTheirIDs(t *testing.T) {
	for _, mode := range []string{"screen", "inline"} {
		for _, sharedImage := range []bool{false, true} {
			t.Run(mode+"/shared="+strconv.FormatBool(sharedImage), func(t *testing.T) {
				var owner graphics.ImageOwner
				first := owner.Bind(graphics.Image{ID: 10, Size: image.Pt(2, 2)})
				second := first
				if !sharedImage {
					second = owner.Bind(graphics.Image{ID: 20, Size: image.Pt(2, 2)})
				}
				a, b := first.Placement(1), second.Placement(2)
				a.Source, b.Source = image.Rect(0, 0, 1, 1), image.Rect(1, 1, 2, 2)
				var frame func() grid.View
				var flush func(io.Writer) error
				if mode == "screen" {
					canvas := owner.NativeTarget().Screen(4, 2)
					frame, flush = canvas.Frame, canvas.Flush
				} else {
					canvas := owner.NativeTarget().Inline(4, 2)
					frame = canvas.Frame
					flush = func(w io.Writer) error { return canvas.Flush(w) }
				}
				for _, order := range [][]graphics.ResourcePlacement{{a, b}, {b, a}} {
					view := frame()
					for _, placement := range order {
						placement.Draw(view, grid.Area(0, 0, 2, 2))
					}
					var output bytes.Buffer
					if err := flush(&output); err != nil {
						t.Fatal(err)
					}
					placements := decodeKittyPlacements(t, output.String())
					if len(placements) != 2 || placements[0].layer >= placements[1].layer {
						t.Fatalf("registration order did not determine stacking: %#v", placements)
					}
					for _, placement := range placements {
						if placement.layer >= 0 || placement.layer < math.MinInt32/2 {
							t.Errorf("image escaped its plane: %#v", placement)
						}
					}
					wantTop := 2
					if order[1] == a {
						wantTop = 1
					}
					if placements[1].placement != wantTop {
						t.Errorf("top placement = %d, want %d", placements[1].placement, wantTop)
					}
				}
			})
		}
	}
}

type kittyPlacement struct{ image, placement, layer int }

func decodeKittyPlacements(t *testing.T, output string) []kittyPlacement {
	t.Helper()
	var placements []kittyPlacement
	var scanner ansi.Scanner
	if err := scanner.Feed(output, func(piece ansi.Piece) error {
		if piece.Kind != ansi.String || piece.Final != '_' || !strings.HasPrefix(piece.Body, "Ga=p,") {
			return nil
		}
		control, _, _ := strings.Cut(piece.Body[1:], ";")
		var placement kittyPlacement
		for parameter := range strings.SplitSeq(control, ",") {
			key, value, _ := strings.Cut(parameter, "=")
			if key != "i" && key != "p" && key != "z" {
				continue
			}
			number, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			switch key {
			case "i":
				placement.image = number
			case "p":
				placement.placement = number
			case "z":
				placement.layer = number
			}
		}
		placements = append(placements, placement)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return placements
}

func TestImagePlacementRefusesLayersOutsideItsPlane(t *testing.T) {
	placement := (graphics.Image{ID: 1}).Placement(1)
	for _, layer := range []int{math.MinInt32, math.MinInt32/2 - 1, 0, 1, math.MaxInt32} {
		var output bytes.Buffer
		if err := placement.Paint(&output, image.Pt(1, 1), layer); !errors.Is(err, errors.ErrUnsupported) || output.Len() != 0 {
			t.Errorf("layer %d wrote %q: %v", layer, output.String(), err)
		}
	}
}
