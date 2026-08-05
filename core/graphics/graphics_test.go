package graphics_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/graphics"
)

// png builds the first 24 bytes of a PNG — the part this package reads — padded
// to total, with the given dimensions.
func png(w, h uint32, total int) []byte {
	buf := make([]byte, max(total, 24))
	copy(buf, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	binary.BigEndian.PutUint32(buf[8:12], 13) // the IHDR chunk's length
	copy(buf[12:16], "IHDR")
	binary.BigEndian.PutUint32(buf[16:20], w)
	binary.BigEndian.PutUint32(buf[20:24], h)
	return buf
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetection(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want graphics.Protocol
	}{
		{"nothing at all", map[string]string{}, graphics.None},
		{"a plain xterm", map[string]string{"TERM": "xterm-256color"}, graphics.None},
		{"kitty by window id", map[string]string{"KITTY_WINDOW_ID": "1"}, graphics.Kitty},
		{"kitty by TERM", map[string]string{"TERM": "xterm-kitty"}, graphics.Kitty},
		{"ghostty by program", map[string]string{"TERM_PROGRAM": "Ghostty"}, graphics.Kitty},
		{"ghostty by resources", map[string]string{"GHOSTTY_RESOURCES_DIR": "/opt"}, graphics.Kitty},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, graphics.Kitty},
		{"warp", map[string]string{"TERM_PROGRAM": "WarpTerminal"}, graphics.Kitty},
		{"iterm2 speaks its own", map[string]string{"TERM_PROGRAM": "iTerm.app"}, graphics.None},
		{"apple terminal", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, graphics.None},
		{"vscode", map[string]string{"TERM_PROGRAM": "vscode"}, graphics.None},
		{
			"a window id outranks a plain TERM",
			map[string]string{"KITTY_WINDOW_ID": "3", "TERM": "xterm-256color"},
			graphics.Kitty,
		},
	} {
		if got := graphics.DetectIn(env(tc.env)); got != tc.want {
			t.Errorf("%s: = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTheZeroProtocolShowsNothing(t *testing.T) {
	// Anything that has not been told what it is talking to must draw no images
	// rather than print escape sequences at the user.
	var p graphics.Protocol
	if p != graphics.None {
		t.Fatalf("the zero protocol is %v, want graphics.None", p)
	}
}

func TestPNGSizeReadsTheHeader(t *testing.T) {
	w, h, err := graphics.PNGSize(png(640, 480, 64))
	if err != nil || w != 640 || h != 480 {
		t.Fatalf("= %dx%d, %v; want 640x480", w, h, err)
	}
}

func TestPNGSizeRefusesWhatItCannotSize(t *testing.T) {
	corruptTag := png(1, 1, 32)
	copy(corruptTag[12:16], "JUNK")
	wrongSignature := png(1, 1, 32)
	wrongSignature[1] = 'X'

	for name, bad := range map[string][]byte{
		"nothing":            nil,
		"empty":              {},
		"not a png at all":   []byte("this is definitely not a png file"),
		"truncated":          png(10, 10, 32)[:20],
		"a corrupt IHDR tag": corruptTag,
		"a wrong signature":  wrongSignature,
		"no width":           png(0, 480, 64),
		"no height":          png(640, 0, 64),
	} {
		if _, _, err := graphics.PNGSize(bad); !errors.Is(err, graphics.ErrNotPNG) {
			t.Errorf("%s: accepted, want graphics.ErrNotPNG", name)
		}
	}
}

func TestTransmitWritesNothingForDataItCannotSize(t *testing.T) {
	var buf bytes.Buffer
	if _, err := graphics.Transmit(&buf, 1, []byte("hello")); err == nil {
		t.Fatal("transmitted something that is not a PNG")
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %q for data it rejected", buf.String())
	}
}

func TestTransmitSendsOneEscapeWhenItFits(t *testing.T) {
	image := png(12, 34, 64)
	var buf bytes.Buffer
	got, err := graphics.Transmit(&buf, 7, image)
	if err != nil {
		t.Fatal(err)
	}
	if got != (graphics.Image{ID: 7, Width: 12, Height: 34}) {
		t.Fatalf("= %+v, want id 7 at 12x34", got)
	}
	want := fmt.Sprintf("\x1b_Ga=T,f=100,i=7,q=2,m=0;%s\x1b\\",
		base64.StdEncoding.EncodeToString(image))
	if buf.String() != want {
		t.Fatalf("wrote\n %q\nwant\n %q", buf.String(), want)
	}
}

func TestTransmitChunksWhatDoesNotFit(t *testing.T) {
	// The protocol takes 4096 base64 bytes per escape, so anything larger has to
	// arrive in pieces with every piece but the last saying more is coming.
	image := png(1, 1, 8000)
	var buf bytes.Buffer
	if _, err := graphics.Transmit(&buf, 1, image); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if n := strings.Count(out, "\x1b_G"); n < 2 {
		t.Fatalf("wrote %d escapes, want the payload split across several", n)
	}
	if !strings.Contains(out, "m=1;") {
		t.Fatal("no chunk said more was coming")
	}
	if !strings.HasSuffix(out, "\x1b\\") || !strings.Contains(out, "m=0;") {
		t.Fatal("no chunk said it was the last")
	}
	// The payload has to survive being cut up.
	var rebuilt strings.Builder
	for _, part := range strings.Split(out, "\x1b_G")[1:] {
		body := part[strings.Index(part, ";")+1 : strings.Index(part, "\x1b\\")]
		rebuilt.WriteString(body)
	}
	decoded, err := base64.StdEncoding.DecodeString(rebuilt.String())
	if err != nil {
		t.Fatalf("the chunks did not rejoin into valid base64: %v", err)
	}
	if !bytes.Equal(decoded, image) {
		t.Fatal("the rejoined payload is not the image that went in")
	}
}

func TestPlaceAndDeleteNameTheImage(t *testing.T) {
	var buf bytes.Buffer
	if err := graphics.Place(&buf, 9, 20, 10); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "i=9") ||
		!strings.Contains(got, "c=20") || !strings.Contains(got, "r=10") {
		t.Fatalf("place = %q, want the id and the cell box in it", got)
	}
	buf.Reset()
	if err := graphics.Delete(&buf, 9); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "a=d") || !strings.Contains(got, "i=9") {
		t.Fatalf("delete = %q, want it to name the image", got)
	}
}

func TestFit(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		pxW, pxH, cellW, cellH         int
		maxCols, maxRows, wantC, wantR int
	}{
		{"a small image is not shrunk", 100, 40, 10, 20, 80, 24, 10, 2},
		{"capped width shrinks the height with it", 800, 400, 10, 20, 20, 24, 20, 5},
		{"capped height shrinks the width with it", 400, 800, 10, 20, 80, 10, 10, 10},
		{"rounded up so it is never squeezed", 15, 25, 10, 20, 80, 24, 2, 2},
		{"nothing sensible gives one cell", 0, 0, 10, 20, 80, 24, 1, 1},
		{"no cell size gives one cell", 100, 100, 0, 0, 80, 24, 1, 1},
		{"a zero cap is still one cell", 100, 100, 10, 20, 0, 0, 1, 1},
	} {
		c, r := graphics.Fit(tc.pxW, tc.pxH, tc.cellW, tc.cellH, tc.maxCols, tc.maxRows)
		if c != tc.wantC || r != tc.wantR {
			t.Errorf("%s: = %dx%d cells, want %dx%d", tc.name, c, r, tc.wantC, tc.wantR)
		}
	}
}

func TestFitKeepsTheProportionsItWasGiven(t *testing.T) {
	// A box that squashed the image against one edge would be worse than one that
	// left space beside it.
	cols, rows := graphics.Fit(1000, 500, 10, 20, 40, 40)
	if cols != 40 {
		t.Fatalf("cols = %d, want the cap", cols)
	}
	// 1000x500 at 10x20 per cell is 100x25 cells, an aspect of 4:1. Capped to 40
	// wide that is 10 tall.
	if rows != 10 {
		t.Fatalf("rows = %d, want the height that keeps a 4:1 box", rows)
	}
}
