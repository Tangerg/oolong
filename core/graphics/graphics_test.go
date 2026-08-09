package graphics_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
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

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		value, ok := m[k]
		return value, ok
	}
}

func TestDetection(t *testing.T) {
	for _, tc := range []struct {
		desc  string
		env   map[string]string
		name  string
		sixel bool
		want  graphics.Protocol
	}{
		{desc: "nothing at all", env: map[string]string{}, want: graphics.None},
		{desc: "a plain xterm", env: map[string]string{"TERM": "xterm-256color"}, want: graphics.None},
		{desc: "kitty by window id", env: map[string]string{"KITTY_WINDOW_ID": "1"}, want: graphics.Kitty},
		{desc: "kitty by TERM", env: map[string]string{"TERM": "xterm-kitty"}, want: graphics.Kitty},
		{desc: "ghostty by program", env: map[string]string{"TERM_PROGRAM": "Ghostty"}, want: graphics.Kitty},
		{desc: "ghostty by resources", env: map[string]string{"GHOSTTY_RESOURCES_DIR": "/opt"}, want: graphics.Kitty},
		{desc: "wezterm", env: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: graphics.Kitty},
		{desc: "warp", env: map[string]string{"TERM_PROGRAM": "WarpTerminal"}, want: graphics.Kitty},
		{desc: "iterm2 by program", env: map[string]string{"TERM_PROGRAM": "iTerm.app"}, want: graphics.ITerm2},
		{desc: "iterm2 across an ssh hop", env: map[string]string{"LC_TERMINAL": "iTerm2"}, want: graphics.ITerm2},
		{desc: "mintty", env: map[string]string{"TERM_PROGRAM": "mintty"}, want: graphics.ITerm2},
		{desc: "apple terminal", env: map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, want: graphics.None},
		{desc: "vscode", env: map[string]string{"TERM_PROGRAM": "vscode"}, want: graphics.None},
		{
			desc: "a window id outranks a plain TERM",
			env:  map[string]string{"KITTY_WINDOW_ID": "3", "TERM": "xterm-256color"},
			want: graphics.Kitty,
		},

		// Sixel is the one nothing in the environment names, so it is the one that
		// has to be asked about.
		{desc: "a terminal that only claims sixel", env: map[string]string{"TERM": "xterm"}, sixel: true, want: graphics.Sixel},
		{
			desc: "the same terminal, never asked",
			env:  map[string]string{"TERM": "xterm"},
			want: graphics.None,
		},
		{
			name:  "a claim of sixel does not outrank a handle",
			env:   map[string]string{"TERM": "xterm-kitty"},
			sixel: true,
			want:  graphics.Kitty,
		},
		{
			name:  "nor does it outrank iTerm2, which at least says how big to draw",
			env:   map[string]string{"TERM_PROGRAM": "iTerm.app"},
			sixel: true,
			want:  graphics.ITerm2,
		},
	} {
		if got := graphics.DetectIn(env(tc.env), tc.name, tc.sixel); got != tc.want {
			t.Errorf("%s: = %v, want %v", tc.desc, got, tc.want)
		}
	}
}

// TestSupportsSeparatesShowingFromPlacing is the distinction the whole model exists
// for. A terminal that draws an image but cannot be told to move it is fine for
// output that is written once and wrong for a region redrawn sixty times a second.
func TestSupportsSeparatesShowingFromPlacing(t *testing.T) {
	for _, tc := range []struct {
		p             graphics.Protocol
		printed, live bool
	}{
		{graphics.None, false, false},
		{graphics.Kitty, true, true},
		{graphics.ITerm2, true, false},
		{graphics.Sixel, true, false},
	} {
		if got := tc.p.Supports(graphics.Printed); got != tc.printed {
			t.Errorf("%v printed = %v, want %v", tc.p, got, tc.printed)
		}
		if got := tc.p.Supports(graphics.Live); got != tc.live {
			t.Errorf("%v live = %v, want %v", tc.p, got, tc.live)
		}
	}
}

func TestUnknownCapabilitiesStayDisabled(t *testing.T) {
	unknownProtocol := graphics.Protocol(200)
	for _, where := range []graphics.Placement{graphics.Printed, graphics.Live} {
		if unknownProtocol.Supports(where) {
			t.Errorf("unknown protocol supports placement %d", where)
		}
	}
	for _, protocol := range []graphics.Protocol{graphics.Kitty, graphics.ITerm2, graphics.Sixel} {
		if protocol.Supports(graphics.Placement(200)) {
			t.Errorf("%v supports an unknown placement", protocol)
		}
	}
}

func TestPrintedIsTheZeroPlacement(t *testing.T) {
	// Code that has not said where an image is going gets the answer that holds in
	// both places, not the one that holds in neither.
	var where graphics.Placement
	if where != graphics.Printed {
		t.Fatalf("the zero placement is %v, want Printed", where)
	}
}

func TestProtocolNames(t *testing.T) {
	for p, want := range map[graphics.Protocol]string{
		graphics.None:          "none",
		graphics.Kitty:         "kitty",
		graphics.ITerm2:        "iterm2",
		graphics.Sixel:         "sixel",
		graphics.Protocol(200): "none",
	} {
		if got := p.String(); got != want {
			t.Errorf("%d = %q, want %q", p, got, want)
		}
	}
}

func TestInlineCarriesThePNGAndItsBox(t *testing.T) {
	var b strings.Builder
	data := png(320, 240, 16)
	if err := graphics.Inline(&b, data, 20, 10); err != nil {
		t.Fatalf("Inline: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"\x1b]1337;File=inline=1;",
		"size=" + strconv.Itoa(len(data)),
		"width=20", "height=10",
		"preserveAspectRatio=1",
		base64.StdEncoding.EncodeToString(data),
		"\x07",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the sequence does not carry %q: %q", want, out)
		}
	}
	// The payload is encoded for the same reason a clipboard's is: nothing in the
	// image can end the sequence early and have the rest read as commands.
	body := strings.SplitN(out, ":", 2)[1]
	if strings.ContainsAny(strings.TrimSuffix(body, "\x07"), "\x1b\x07") {
		t.Error("the payload can end its own sequence")
	}
}

func TestInlineRefusesWhatIsNotAPNG(t *testing.T) {
	var b strings.Builder
	if err := graphics.Inline(&b, []byte("not a png at all, truly"), 4, 2); err == nil {
		t.Error("something that is not a PNG was written anyway")
	}
	if b.Len() != 0 {
		t.Errorf("it wrote %q before refusing", b.String())
	}
}

func TestInlineBoxIsNeverEmpty(t *testing.T) {
	// A box of no cells would ask the terminal to draw nothing, which is not what a
	// caller who passed a zero meant.
	var b strings.Builder
	if err := graphics.Inline(&b, png(8, 8, 8), 0, 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "width=1") || !strings.Contains(b.String(), "height=1") {
		t.Errorf("got %q", b.String())
	}
}

func TestFitDoesNotOverflowAtIntegerLimits(t *testing.T) {
	maxInt := int(^uint(0) >> 1)

	cols, rows := graphics.Fit(maxInt, 1, maxInt-1, 1, maxInt, maxInt)
	if cols != 2 || rows != 1 {
		t.Fatalf("ceiling fit = %dx%d, want 2x1", cols, rows)
	}

	limit := maxInt / 2
	cols, rows = graphics.Fit(maxInt, maxInt, 1, 1, limit, maxInt)
	if cols != limit || rows != limit {
		t.Fatalf("scaled fit = %dx%d, want %dx%d", cols, rows, limit, limit)
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

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return max(len(p)-1, 0), nil }

func TestGraphicsWritesRejectSilentTruncation(t *testing.T) {
	image := png(1, 1, 24)
	operations := map[string]func() error{
		"transmit": func() error {
			_, err := graphics.Transmit(shortWriter{}, 1, image)
			return err
		},
		"place":  func() error { return graphics.Place(shortWriter{}, 1, 1, 1) },
		"delete": func() error { return graphics.Delete(shortWriter{}, 1) },
		"inline": func() error { return graphics.Inline(shortWriter{}, image, 1, 1) },
	}
	for name, operation := range operations {
		if err := operation(); !errors.Is(err, io.ErrShortWrite) {
			t.Errorf("%s error = %v, want io.ErrShortWrite", name, err)
		}
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

// TestWhatTheTerminalSaidOutranksTheEnvironment. An environment describes the terminal
// a session was started from; over ssh, in a container, or under a multiplexer that is
// not the terminal it is talking to.
func TestWhatTheTerminalSaidOutranksTheEnvironment(t *testing.T) {
	for _, tc := range []struct {
		desc string
		env  map[string]string
		name string
		want graphics.Protocol
	}{
		{
			desc: "ssh into a machine with no variables, from kitty",
			env:  map[string]string{"TERM": "xterm-256color"},
			name: "kitty(0.32.2)",
			want: graphics.Kitty,
		},
		{
			desc: "the environment is left over from the terminal that started the shell",
			env:  map[string]string{"TERM_PROGRAM": "Apple_Terminal"},
			name: "WezTerm 20240203",
			want: graphics.Kitty,
		},
		{
			desc: "iTerm2 naming itself",
			env:  map[string]string{},
			name: "iTerm2 3.5.0",
			want: graphics.ITerm2,
		},
		{
			desc: "a terminal that named itself something nobody knows falls back",
			env:  map[string]string{"TERM_PROGRAM": "iTerm.app"},
			name: "SomeNewTerminal 1.0",
			want: graphics.ITerm2,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if got := graphics.DetectIn(env(tc.env), tc.name, false); got != tc.want {
				t.Errorf("= %v, want %v", got, tc.want)
			}
		})
	}
}
