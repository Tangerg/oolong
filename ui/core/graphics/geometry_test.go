package graphics_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/graphics"
)

func TestPaintRejectsInvalidCellDimensionsBeforeWriting(t *testing.T) {
	sizes := []image.Point{image.Pt(-1, 2), image.Pt(2, -1)}
	if strconv.IntSize == 64 {
		outside := uint64(math.MaxUint32) + 1
		sizes = append(sizes, image.Pt(int(outside), 2), image.Pt(2, int(outside)))
	}
	placement := (graphics.Image{ID: 1, Size: image.Pt(2, 2)}).Placement(1)
	for _, size := range sizes {
		var output bytes.Buffer
		if err := placement.Paint(&output, size, -1); !errors.Is(err, errors.ErrUnsupported) || output.Len() != 0 {
			t.Errorf("Paint(%v) = %v, output %q; want refusal before writing", size, err, &output)
		}
	}
}

func TestPaintPreservesAutomaticAndBoundaryCellDimensions(t *testing.T) {
	sizes := []image.Point{{}, image.Pt(0, 2), image.Pt(2, 0)}
	if strconv.IntSize == 64 {
		limit := uint64(math.MaxUint32)
		sizes = append(sizes, image.Pt(int(limit), int(limit)))
	} else {
		sizes = append(sizes, image.Pt(math.MaxInt32, math.MaxInt32))
	}
	placement := (graphics.Image{ID: 1}).Placement(1)
	for _, size := range sizes {
		var output bytes.Buffer
		err := placement.Paint(&output, size, -1)
		want := fmt.Sprintf(",c=%d,r=%d,", size.X, size.Y)
		if err != nil || !strings.Contains(output.String(), want) {
			t.Errorf("Paint(%v) = %v, output %q; want %q", size, err, &output, want)
		}
	}
}

func TestPaintValidatesEveryEncodedSourceCoordinate(t *testing.T) {
	if strconv.IntSize != 64 {
		t.Skip("all nonnegative int coordinates fit the protocol on this architecture")
	}
	outside := uint64(math.MaxUint32) + 1
	overflow := int(outside)
	placement := (graphics.Image{ID: 1, Size: image.Pt(overflow+1, overflow+1)}).Placement(1)
	for _, source := range []image.Rectangle{
		image.Rect(overflow, 0, overflow+1, 1),
		image.Rect(0, overflow, 1, overflow+1),
		image.Rect(0, 0, overflow, 1),
		image.Rect(0, 0, 1, overflow),
	} {
		placement.Source = source
		var output bytes.Buffer
		if err := placement.Paint(&output, image.Pt(1, 1), -1); !errors.Is(err, errors.ErrUnsupported) || output.Len() != 0 {
			t.Errorf("Paint source %v = %v, output %q; want refusal before writing", source, err, &output)
		}
	}
	placement.Source = image.Rect(overflow-2, overflow-2, overflow-1, overflow-1)
	var output bytes.Buffer
	if err := placement.Paint(&output, image.Pt(1, 1), -1); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(",x=%d,y=%d,w=1,h=1,", overflow-2, overflow-2)
	if !strings.Contains(output.String(), want) {
		t.Fatalf("boundary crop = %q; want %q", &output, want)
	}
}
