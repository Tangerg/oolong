// Package graphics puts images in a terminal that can show them.
//
// It writes escape sequences and nothing else: it does not decode an image, hold
// one, decide where it goes, or know what a cell contains. What it needs is a PNG
// that somebody else already has and a place the caller has already worked out.
//
// # Why only PNG
//
// PNG because its dimensions are in the first twenty-four bytes, which means the
// size can be known without a decoder and therefore without a dependency. The
// promise this library makes about its dependency list is worth more than the
// convenience of accepting a JPEG.
//
// # The protocols, and what each is good for
//
// Showing an image and showing one in an interface that redraws are two different
// capabilities, and only one protocol has both.
//
// Kitty's gives the program a handle: an image is sent once under a number, placed
// as often as needed, moved, and deleted. That is what a live region requires —
// what it showed last frame has to be moved or taken away this frame, and a
// protocol with no way to name an image has no way to be told which one.
//
// iTerm2's protocol and sixel put pixels at the cursor and end there. No number, no
// z-order, no deletion. They are usable where nothing will redraw over the result —
// printed output, which belongs to the terminal from then on — and not in a region
// being drawn again sixty times a second. [Protocol.Supports] is that distinction,
// and it is why this package names more protocols than it writes.
//
// What it writes is kitty and iTerm2. Sixel is detected and reported and not
// produced: producing it means decoding the image into pixels, and a decoder is the
// dependency this package exists without. A caller holding an encoder of its own
// learns from [Sixel] that the terminal will take what it makes.
package graphics

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
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
	// Kitty is the kitty graphics protocol: kitty, Ghostty, WezTerm and Warp. It is
	// the only one usable in a region the interface redraws.
	Kitty
	// ITerm2 is iTerm2's own inline-image protocol, also spoken by WezTerm and
	// mintty. An image goes at the cursor and cannot be referred to again.
	ITerm2
	// Sixel is the oldest of the three and the most widely implemented after
	// kitty's: xterm, foot, mlterm, contour, and recent Windows Terminal. This
	// package reports it and does not write it — see the package comment.
	Sixel
)

// String names the protocol the way a diagnostic would.
func (p Protocol) String() string {
	switch p {
	case Kitty:
		return "kitty"
	case ITerm2:
		return "iterm2"
	case Sixel:
		return "sixel"
	default:
		return "none"
	}
}

// Placement is where an image is going, which is what decides whether a protocol
// will do.
type Placement uint8

const (
	// Printed is output written once that then belongs to the terminal, scrolling
	// away with the rest of the session. Nothing draws over it again, so a protocol
	// that cannot be told to remove an image is no worse off here than one that can.
	//
	// It is the zero value because it is the weaker requirement: code that has not
	// said where an image is going gets the answer that holds in both places.
	Printed Placement = iota
	// Live is a region the interface redraws. An image there has to be placeable
	// again on the next frame and removable on the frame after, which takes a
	// protocol that lets an image be named.
	Live
)

// Supports reports whether an image can go in that place over this protocol.
//
// Asking it is worth more than a single yes or no about the terminal. A terminal
// that draws images but cannot be told to move them is a different thing to tell
// the user about from one that draws none: the first shows a picture in printed
// output and nothing in a live view, and a caller holding one boolean can explain
// neither.
func (p Protocol) Supports(where Placement) bool {
	switch where {
	case Live:
		return p == Kitty
	case Printed:
		switch p {
		case Kitty, ITerm2, Sixel:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

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
		var sequence string
		if first {
			sequence = fmt.Sprintf("\x1b_Ga=T,f=100,i=%d,q=2,m=%d;%s\x1b\\", id, more, chunk)
		} else {
			sequence = fmt.Sprintf("\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
		if err = writeString(w, sequence); err != nil {
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
//
// The cursor is left where it was found. That is what lets an image go in a frame
// at all — every position in a frame is a movement from the last known one, and an
// inline block's whole position is relative — and it is the property the protocols
// that cannot be told to move an image also lack. See [Image.Paint], which is this
// under the name a frame asks for.
func Place(w io.Writer, id uint32, cols, rows int) error {
	// z=-1 puts the image behind the text, so a caller can still write over it, and
	// C=1 keeps the cursor where it is.
	return writeString(w, fmt.Sprintf("\x1b_Ga=p,i=%d,c=%d,r=%d,z=-1,C=1,q=2;\x1b\\", id, cols, rows))
}

// Paint puts the image in a region of a frame, and Erase takes it away again.
//
// The two of them are what a frame asks of anything that writes itself onto the
// terminal rather than into cells — see
// [github.com/Tangerg/oolong/core/grid.Painter], which this satisfies without either
// package knowing about the other: one says what a region needs, the other happens
// to be able to do it.
//
// Only an image that was transmitted has them, which is the same distinction the
// package comment draws. An image put on the screen with [Inline] has no name to
// place again or take away, so there is nothing here for it to be.
func (i Image) Paint(w io.Writer, cols, rows int) error { return Place(w, i.ID, cols, rows) }

// Erase removes every placement of the image and forgets it.
func (i Image) Erase(w io.Writer) error { return Delete(w, i.ID) }

// Delete removes every placement of an image and forgets it.
func Delete(w io.Writer, id uint32) error {
	return writeString(w, fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2;\x1b\\", id))
}

// Inline writes an image at the cursor over iTerm2's protocol.
//
// There is no counterpart to [Place] or [Delete], because the protocol has none:
// the image goes where the cursor is and the program never hears of it again. That
// is why it is only for [Printed] output — see the package comment — and why this
// takes the payload every time rather than an identifier.
//
// The box is in cells, and the image is fitted inside it rather than stretched to
// it, so a caller passes what [Fit] worked out and gets the aspect ratio kept.
func Inline(w io.Writer, png []byte, cols, rows int) error {
	if _, _, err := PNGSize(png); err != nil {
		return err
	}
	// The payload is base64 for the same reason a clipboard's is: the alphabet holds
	// neither the escape byte nor the terminator, so image data cannot end the
	// sequence early and have the rest of itself read as commands.
	sequence := fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1:%s\x07",
		len(png), max(cols, 1), max(rows, 1), base64.StdEncoding.EncodeToString(png))
	return writeString(w, sequence)
}

// writeString gives every graphics sequence io.Writer's complete-write semantics.
// io.Copy detects a writer that returns a short count without the required error;
// formatting directly into such a writer would otherwise report a truncated escape
// as successfully delivered.
func writeString(w io.Writer, sequence string) error {
	_, err := io.Copy(w, strings.NewReader(sequence))
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

	// Rounded up, so an image that fits is never shrunk to fit better. Quotient and
	// remainder avoid the overflowing n+d-1 spelling of ceiling division.
	cols = max(ceilDiv(pxW, cellW), 1)
	rows = max(ceilDiv(pxH, cellH), 1)

	// Each axis is capped and the other shrinks with it, so the box keeps the
	// image's proportions instead of squashing it against one edge.
	if cols > maxCols {
		rows = max(scaleNearest(rows, maxCols, cols), 1)
		cols = maxCols
	}
	if rows > maxRows {
		cols = max(scaleNearest(cols, maxRows, rows), 1)
		rows = maxRows
	}
	return cols, rows
}

func ceilDiv(n, d int) int {
	quotient := n / d
	if n%d != 0 {
		quotient++
	}
	return quotient
}

const maxInt = int(^uint(0) >> 1)

// scaleNearest returns total*part/whole rounded to the nearest integer without an
// overflowing intermediate product. Fit needs nearest rather than layout's endpoint
// rounding: losing a row from a two-row image visibly changes its aspect ratio.
func scaleNearest(total, part, whole int) int {
	if total <= 0 || part <= 0 || whole <= 0 {
		return 0
	}
	if part >= whole {
		return total
	}
	hi, lo := bits.Mul64(uint64(total), uint64(part))
	quotient, remainder := bits.Div64(hi, lo, uint64(whole))
	if remainder >= uint64(whole)/2+uint64(whole)%2 {
		quotient++
	}
	// part < whole proves quotient <= total, and total is an int. Keep the proof at
	// the conversion boundary as well: a future change to the arithmetic cannot turn
	// a representability assumption into a wrapped image dimension.
	if quotient > uint64(maxInt) {
		return maxInt
	}
	return int(quotient)
}

// kittyPrograms are the TERM_PROGRAM values, matched as lowercase substrings, of
// terminals that speak the kitty protocol without saying so in TERM.
//
// WezTerm speaks iTerm2's protocol too and is listed here rather than there, because
// where both are available the one with handles is the one worth having.
var kittyPrograms = []string{"kitty", "ghostty", "wezterm", "warp"}

// iterm2Programs are the same for iTerm2's protocol.
// The substring that identifies each, not its full name: a terminal names itself
// differently in every place it is asked. iTerm2 is "iTerm.app" in TERM_PROGRAM,
// "iTerm2" in LC_TERMINAL, and "iTerm2 3.5.0" when it is asked directly.
var iterm2Programs = []string{"iterm", "mintty"}

// DetectIn works out the richest protocol a terminal supports.
//
// name is what the terminal said it was when asked, or empty when nothing was asked or
// nothing answered. It outranks the environment for the reason above.
//
// It takes these facts because fewer are not enough. The environment names the terminal,
// which is how kitty's protocol and iTerm2's are found; nothing in the environment
// names sixel, so a terminal that supports sixel and nothing else is
// indistinguishable from a terminal that supports nothing. That answer only comes
// from a device-attribute response, and sixel says what it said.
//
// The lookup is passed in rather than read, which is what makes this a function of
// its inputs: an adapter passes its environment lookup and negotiated answers, and
// a test passes whatever facts it wants. There is no cached global and no
// override hook, for the same reason there is no global palette — a program with two
// terminals could not have two answers, and a test could not pin either.
func DetectIn(lookup func(string) (string, bool), name string, sixel bool) Protocol {
	// What the terminal called itself, first. An environment describes the terminal a
	// session was started from, which over ssh, in a container, or under a multiplexer
	// is not the terminal it is talking to; an answer to a question came from the one
	// that is actually drawing.
	if p, ok := named(name); ok {
		return p
	}
	if environmentValue(lookup, "KITTY_WINDOW_ID") != "" ||
		environmentValue(lookup, "GHOSTTY_RESOURCES_DIR") != "" {
		return Kitty
	}
	if p, ok := named(environmentValue(lookup, "TERM") + " " +
		environmentValue(lookup, "TERM_PROGRAM") + " " +
		environmentValue(lookup, "LC_TERMINAL")); ok {
		return p
	}
	if sixel {
		return Sixel
	}
	return None
}

func environmentValue(lookup func(string) (string, bool), name string) string {
	if lookup == nil {
		return ""
	}
	value, _ := lookup(name)
	return value
}

// named is the protocol whichever terminal an identity names speaks, if it is one this
// package knows.
func named(identity string) (Protocol, bool) {
	if identity == "" {
		return None, false
	}
	identity = strings.ToLower(identity)
	for _, name := range kittyPrograms {
		if strings.Contains(identity, name) {
			return Kitty, true
		}
	}
	for _, name := range iterm2Programs {
		if strings.Contains(identity, name) {
			return ITerm2, true
		}
	}
	return None, false
}
