// Package graphics puts images in a terminal that can show them.
//
// It writes escape sequences and nothing else: it does not decode an image, hold
// one, decide where it goes, or know what a cell contains. What it needs is a PNG
// that somebody else already has and a place the caller has already worked out.
//
// # Why only PNG, and only kitty
//
// PNG because its dimensions are in the first twenty-four bytes, which means the
// size can be known without a decoder and therefore without a dependency. The
// promise this library makes about its dependency list is worth more than the
// convenience of accepting a JPEG.
//
// Kitty because it is the protocol the terminals worth targeting converged on —
// kitty, Ghostty, WezTerm and Warp all speak it. iTerm2's own protocol and sixel
// are not here, and a terminal that speaks neither this nor nothing at all gets
// [None], which is the caller's cue to draw a placeholder.
package graphics

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Protocol is the inline-image capability of a terminal.
//
// The zero value is [None], so anything that has not been told what it is talking
// to draws no images rather than corrupting a screen with escape sequences the
// terminal will print instead of obey.
type Protocol uint8

const (
	// None is a terminal that cannot show an image. A caller draws a placeholder.
	None Protocol = iota
	// Kitty is the kitty graphics protocol: kitty, Ghostty, WezTerm and Warp.
	Kitty
)

// ErrNotPNG is reported for data that is not a PNG this package can size.
var ErrNotPNG = errors.New("graphics: not a PNG")

// Image is a transmitted image and the size it arrived at.
type Image struct {
	// ID is the number the terminal now knows the image by, which is what
	// [Place] and [Delete] refer to.
	ID uint32
	// Width and Height are the image's size in pixels, for working out how many
	// cells it should occupy with [Fit].
	Width, Height int
}

// pngSignature is the eight bytes a PNG begins with.
var pngSignature = [8]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// PNGSize is the pixel size of a PNG, read from its header.
//
// It reads the signature and the IHDR chunk and nothing else. A decoder would be
// a dependency, and every byte it would decode is a byte this package has no use
// for: the pixels are the terminal's problem, and only the dimensions are ours.
func PNGSize(png []byte) (width, height int, err error) {
	// Eight of signature, four of chunk length, four of "IHDR", then the two
	// dimensions.
	if len(png) < 24 || [8]byte(png[:8]) != pngSignature || string(png[12:16]) != "IHDR" {
		return 0, 0, ErrNotPNG
	}
	w := binary.BigEndian.Uint32(png[16:20])
	h := binary.BigEndian.Uint32(png[20:24])
	if w == 0 || h == 0 || w > 1<<31-1 || h > 1<<31-1 {
		return 0, 0, ErrNotPNG
	}
	return int(w), int(h), nil
}

// chunkLimit is the most base64 the kitty protocol takes in one escape.
const chunkLimit = 4096

// Transmit sends a PNG to the terminal under an ID, without placing it.
//
// Transmission and placement are separate because they happen at different times:
// an image is sent once and placed on every frame that shows it, and re-sending
// the payload each frame would put a megabyte on the wire to move a picture by
// one row.
func Transmit(w io.Writer, id uint32, png []byte) (Image, error) {
	width, height, err := PNGSize(png)
	if err != nil {
		return Image{}, err
	}
	payload := base64.StdEncoding.EncodeToString(png)

	for first := true; ; first = false {
		chunk := payload
		if len(chunk) > chunkLimit {
			chunk = chunk[:chunkLimit]
		}
		payload = payload[len(chunk):]
		more := 0
		if len(payload) > 0 {
			more = 1
		}
		// a=T transmits and shows, f=100 says the payload is PNG, q=2 asks the
		// terminal not to answer — there is nobody reading for a reply, and an
		// unread one would arrive in the middle of the next keystroke.
		if first {
			_, err = fmt.Fprintf(w, "\x1b_Ga=T,f=100,i=%d,q=2,m=%d;%s\x1b\\", id, more, chunk)
		} else {
			_, err = fmt.Fprintf(w, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
		if err != nil {
			return Image{}, err
		}
		if more == 0 {
			return Image{ID: id, Width: width, Height: height}, nil
		}
	}
}

// Place shows a transmitted image at the cursor, scaled into cols by rows cells.
//
// The cursor is positioned by the caller, and the escape belongs after the cell
// diff of the frame it appears in: the diff would otherwise write over the image
// with the blanks it thinks are underneath it.
func Place(w io.Writer, id uint32, cols, rows int) error {
	// z=-1 puts the image behind the text, so a caller can still write over it.
	_, err := fmt.Fprintf(w, "\x1b_Ga=p,i=%d,c=%d,r=%d,z=-1,q=2;\x1b\\", id, cols, rows)
	return err
}

// Delete removes every placement of an image and forgets it.
func Delete(w io.Writer, id uint32) error {
	_, err := fmt.Fprintf(w, "\x1b_Ga=d,d=I,i=%d,q=2;\x1b\\", id)
	return err
}

// Fit is the cell box an image of pxW by pxH should occupy, keeping its aspect
// ratio and staying inside maxCols by maxRows.
//
// cellW and cellH are the size of one cell in pixels, which a terminal reports
// and a caller has to have asked for. Nothing sensible can be computed without
// them, so an unusable argument gives a single cell rather than a division by
// zero.
func Fit(pxW, pxH, cellW, cellH, maxCols, maxRows int) (cols, rows int) {
	maxCols, maxRows = max(maxCols, 1), max(maxRows, 1)
	if pxW <= 0 || pxH <= 0 || cellW <= 0 || cellH <= 0 {
		return 1, 1
	}

	// Rounded up, so an image that fits is never shrunk to fit better.
	cols = max((pxW+cellW-1)/cellW, 1)
	rows = max((pxH+cellH-1)/cellH, 1)

	// Each axis is capped and the other shrinks with it, so the box keeps the
	// image's proportions instead of squashing it against one edge.
	if cols > maxCols {
		rows = max((rows*maxCols+cols/2)/cols, 1)
		cols = maxCols
	}
	if rows > maxRows {
		cols = max((cols*maxRows+rows/2)/rows, 1)
		rows = maxRows
	}
	return cols, rows
}

// kittyPrograms are the TERM_PROGRAM values, matched as lowercase substrings, of
// terminals that speak the kitty protocol without saying so in TERM.
var kittyPrograms = []string{"ghostty", "wezterm", "warp"}

// DetectIn works out the protocol from an environment.
//
// It takes its own lookup rather than reading the process environment, which is
// what makes it a function of its inputs: [term.DetectGraphics] passes os.Getenv,
// and a test passes whatever it wants to say the terminal is. There is no cached
// global and no override hook, for the same reason there is no global palette —
// a program with two terminals could not have two answers, and a test could not
// pin either.
func DetectIn(getenv func(string) string) Protocol {
	if getenv("KITTY_WINDOW_ID") != "" || getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return Kitty
	}
	if strings.Contains(getenv("TERM"), "kitty") {
		return Kitty
	}
	if program := strings.ToLower(getenv("TERM_PROGRAM")); program != "" {
		for _, name := range kittyPrograms {
			if strings.Contains(program, name) {
				return Kitty
			}
		}
	}
	return None
}
