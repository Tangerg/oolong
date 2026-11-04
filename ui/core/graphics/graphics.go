// Package graphics keeps PNG pixel decoding outside terminal transport. Live
// images need reusable handles and erasure; printed images do not. ImageOwner
// binds transmissions to one destination. Sixel encoding is unavailable because
// it would require pixel decoding.
package graphics

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"math/big"
	"strings"
)

// Protocol defaults to None so an unnegotiated destination receives no image
// commands it might print as ordinary text.
type Protocol uint8

const (
	None Protocol = iota
	// Kitty provides the named placements needed for live redraw and erasure.
	Kitty
	// ITerm2 is limited to printed output because placements cannot be named
	// and erased independently.
	ITerm2
	// Sixel is detected but cannot be encoded without pixel decoding.
	Sixel
)

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

type Placement uint8

const (
	// Printed output needs no later erasure, so it is the weaker placement
	// requirement and the zero value.
	Printed Placement = iota
	// Live is a region the interface redraws. An image there has to be placeable
	// again on the next frame and removable on the frame after, which takes a
	// protocol that lets an image be named.
	Live
)

// Supports separates live placement from printed output because image support
// without reusable handles cannot safely redraw an interactive region.
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

type Image struct {
	// ID must be nonzero: Kitty treats zero as unspecified, which cannot name
	// a transmission for later placement or deletion.
	ID uint32
	// Size is the image's size in pixels, for working out how many cells it should
	// occupy with [Fit].
	Size image.Point
}

// pngSize reads only the image configuration. The standard decoder owns PNG
// validation; this package owns terminal transport, not a second partial parser for
// the same format.
func pngSize(data []byte) (image.Point, error) {
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Point{}, fmt.Errorf("graphics: read PNG size: %w", err)
	}
	return image.Pt(config.Width, config.Height), nil
}

// Both Kitty identities must be nonzero; zero means unspecified and cannot
// address the transmission or placement for later removal.
var (
	errImageIdentity     = errors.New("graphics: image identity must be nonzero")
	errPlacementIdentity = errors.New("graphics: placement identity must be nonzero")
)

const (
	chunkLimit           = 4096
	kittyPNG             = 100
	kittySuppressReplies = 2
	kittyPreserveCursor  = 1
	// Kitty puts lower layers behind non-default cell backgrounds.
	minimumImageLayer = math.MinInt32 / 2
)

// Transmit separates upload from placement so moving an image does not resend
// its PNG payload on every frame.
func Transmit(w io.Writer, id uint32, png []byte) (Image, error) {
	if id == 0 {
		return Image{}, errImageIdentity
	}
	size, err := transmit(w, fmt.Sprintf("i=%d,q=%d", id, kittySuppressReplies), png)
	if err != nil {
		return Image{}, err
	}
	return Image{ID: id, Size: size}, nil
}

// Upload requests a fresh physical identity, even when another image already has
// this number. Only the terminal's matching reply identifies the created image.
func Upload(w io.Writer, number uint32, png []byte) (image.Point, error) {
	if number == 0 {
		return image.Point{}, errImageIdentity
	}
	return transmit(w, fmt.Sprintf("I=%d", number), png)
}

func transmit(w io.Writer, identity string, png []byte) (image.Point, error) {
	size, err := pngSize(png)
	if err != nil {
		return image.Point{}, err
	}
	const rawChunk = chunkLimit / 4 * 3
	for offset := 0; offset < len(png); {
		end := min(offset+rawChunk, len(png))
		more := 0
		if end < len(png) {
			more = 1
		}
		chunk := base64.StdEncoding.EncodeToString(png[offset:end])
		var sequence string
		if offset == 0 {
			sequence = fmt.Sprintf("\x1b_Ga=t,f=%d,%s,m=%d;%s\x1b\\", kittyPNG, identity, more, chunk)
		} else {
			sequence = fmt.Sprintf("\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
		if err := writeString(w, sequence); err != nil {
			return image.Point{}, err
		}
		offset = end
	}
	return size, nil
}

// ImagePlacement is one independently removable placement of transmitted data.
// ID is nonzero and unique among simultaneous placements of the same image.
type ImagePlacement struct {
	Image Image
	ID    uint32
	// Source selects source pixels. Its zero value displays the entire image.
	Source image.Rectangle
}

// Placement names one placement without transmitting or drawing it. It panics
// when id is zero or when the image has no identity of its own.
// Reusing an ID moves that placement; use different IDs to show one image twice.
func (i Image) Placement(id uint32) ImagePlacement {
	if i.ID == 0 {
		panic("graphics: image identity must be nonzero")
	}
	if id == 0 {
		panic("graphics: placement identity must be nonzero")
	}
	return ImagePlacement{Image: i, ID: id}
}

func sourceBounds(size image.Point, source image.Rectangle) image.Rectangle {
	bounds := image.Rectangle{Max: size}
	if source == (image.Rectangle{}) {
		return bounds
	}
	return bounds.Intersect(source)
}

// Paint preserves the cursor and keeps native layers below text but above cell
// backgrounds. The composition owns layer; zero and out-of-range layers are refused
// before writing bytes. Zero cell dimensions retain the protocol's automatic sizing;
// all geometry must fit its unsigned integers. A nonzero source must intersect the image.
func (p ImagePlacement) Paint(w io.Writer, size image.Point, layer int) error {
	if p.Image.ID == 0 {
		return errImageIdentity
	}
	if p.ID == 0 {
		return errPlacementIdentity
	}
	if layer < minimumImageLayer || layer >= 0 {
		return fmt.Errorf("graphics: native layer %d outside [%d, -1]: %w", layer, minimumImageLayer, errors.ErrUnsupported)
	}
	if !validKittyPoint(size) {
		return fmt.Errorf("graphics: cell dimensions %v outside [0, %d]: %w", size, uint64(math.MaxUint32), errors.ErrUnsupported)
	}
	var crop string
	if p.Source != (image.Rectangle{}) {
		bounds := sourceBounds(p.Image.Size, p.Source)
		if bounds.Empty() {
			return errors.New("graphics: source rectangle contains no image pixels")
		}
		if !validKittyPoint(bounds.Min) || !validKittyPoint(bounds.Size()) {
			return fmt.Errorf("graphics: source rectangle %v exceeds Kitty coordinate range: %w", bounds, errors.ErrUnsupported)
		}
		crop = fmt.Sprintf(",x=%d,y=%d,w=%d,h=%d", bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy())
	}
	return writeString(w, fmt.Sprintf("\x1b_Ga=p,i=%d,p=%d%s,c=%d,r=%d,z=%d,C=%d,q=%d;\x1b\\", p.Image.ID, p.ID, crop, size.X, size.Y, layer, kittyPreserveCursor, kittySuppressReplies))
}

func validKittyPoint(point image.Point) bool {
	return point.X >= 0 && point.Y >= 0 && uint64(point.X) <= math.MaxUint32 && uint64(point.Y) <= math.MaxUint32
}

// Erase removes only this placement, retaining the image for later Paint calls.
func (p ImagePlacement) Erase(w io.Writer) error {
	if p.Image.ID == 0 {
		return errImageIdentity
	}
	if p.ID == 0 {
		return errPlacementIdentity
	}
	return writeString(w, fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,p=%d,q=%d;\x1b\\", p.Image.ID, p.ID, kittySuppressReplies))
}

// Release destroys transmitted data and all placements. The handle must not be
// painted again after Release; retransmission requires a fresh image identity.
func (i Image) Release(w io.Writer) error {
	if i.ID == 0 {
		return errImageIdentity
	}
	return writeString(w, fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=%d;\x1b\\", i.ID, kittySuppressReplies))
}

// Inline is limited to Printed output because iTerm2 supplies no reusable
// placement identity or erasure. Box dimensions are cells; the protocol fits
// the image inside them while preserving its aspect ratio.
func Inline(w io.Writer, data []byte, size image.Point) error {
	if _, err := pngSize(data); err != nil {
		return err
	}
	// The payload is base64 for the same reason a clipboard's is: the alphabet holds
	// neither the escape byte nor the terminator, so image data cannot end the
	// sequence early and have the rest of itself read as commands.
	sequence := fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1:%s\x07",
		len(data), max(size.X, 1), max(size.Y, 1), base64.StdEncoding.EncodeToString(data))
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

// Fit is the cell box an image of pixels should occupy, keeping its aspect ratio
// and staying inside limit.
//
// cell is the size of one terminal cell in pixels. Nothing sensible can be
// computed without positive image and cell dimensions, so an unusable argument
// gives one cell rather than dividing by zero. Point keeps each two-dimensional
// fact together, so callers cannot silently exchange a width from one coordinate
// space with a height from another.
func Fit(pixels, cell, limit image.Point) image.Point {
	limit.X, limit.Y = max(limit.X, 1), max(limit.Y, 1)
	if pixels.X <= 0 || pixels.Y <= 0 || cell.X <= 0 || cell.Y <= 0 {
		return image.Pt(1, 1)
	}

	natural := image.Pt(ceilDiv(pixels.X, cell.X), ceilDiv(pixels.Y, cell.Y))
	if natural.X <= limit.X && natural.Y <= limit.Y {
		return natural
	}

	// The exact width:height ratio in cells is (pxWidth * cellHeight) :
	// (pxHeight * cellWidth). Rounding before choosing the tighter limit can
	// distort even small pictures. The comparison can involve three full ints.
	var width, height, fromWidth, fromHeight big.Int
	width.Mul(big.NewInt(int64(pixels.X)), big.NewInt(int64(cell.Y)))
	height.Mul(big.NewInt(int64(pixels.Y)), big.NewInt(int64(cell.X)))
	fromWidth.Mul(&height, big.NewInt(int64(limit.X)))
	fromHeight.Mul(&width, big.NewInt(int64(limit.Y)))
	if fromWidth.Cmp(&fromHeight) <= 0 {
		return image.Pt(limit.X, nearestCellExtent(&fromWidth, &width))
	}
	return image.Pt(nearestCellExtent(&fromHeight, &height), limit.Y)
}

func ceilDiv(n, d int) int {
	quotient := n / d
	if n%d != 0 {
		quotient++
	}
	return quotient
}

// The binding axis guarantees the quotient fits the other axis's integer limit.
func nearestCellExtent(numerator, denominator *big.Int) int {
	var rounded big.Int
	rounded.Rsh(denominator, 1)
	rounded.Add(&rounded, numerator)
	rounded.Quo(&rounded, denominator)
	return max(int(rounded.Int64()), 1)
}

// Prefer Kitty when a terminal supports both image protocols, because its
// handles support erasure and live redraw. Environment names are lowercase
// substring matches to accommodate terminal-specific spelling.
var kittyPrograms = []string{"kitty", "ghostty", "wezterm", "warp"}

// iTerm2 uses different names in TERM_PROGRAM, LC_TERMINAL, and replies, so
// detection matches identifying substrings rather than one complete name.
var iterm2Programs = []string{"iterm", "mintty"}

// Detect chooses a protocol from negotiated name, sixel support and environment.
// Negotiated names take precedence because remote or multiplexed sessions can name
// a different terminal in their environment. Explicit inputs keep detection specific
// to each terminal rather than global.
func Detect(lookup func(string) (string, bool), name string, sixel bool) Protocol {
	return environmentProtocol(lookup).Negotiated(name, sixel)
}

// Negotiated resolves probe answers against the protocol measured from the
// environment before acquisition. A known name wins; sixel is the fallback.
func (p Protocol) Negotiated(name string, sixel bool) Protocol {
	if known, ok := named(name); ok {
		return known
	}
	if p == None && sixel {
		return Sixel
	}
	return p
}

func environmentProtocol(lookup func(string) (string, bool)) Protocol {
	if environmentValue(lookup, "KITTY_WINDOW_ID") != "" ||
		environmentValue(lookup, "GHOSTTY_RESOURCES_DIR") != "" {
		return Kitty
	}
	if p, ok := named(environmentValue(lookup, "TERM") + " " +
		environmentValue(lookup, "TERM_PROGRAM") + " " +
		environmentValue(lookup, "LC_TERMINAL")); ok {
		return p
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
