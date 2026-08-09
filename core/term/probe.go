package term

import (
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

const (
	// queryBackground asks the terminal what colour it draws on. The answer comes
	// back as command 11 with an XParseColor specification in it.
	queryBackground = "\x1b]11;?\x07"

	// queryForeground asks the terminal what colour it draws with.
	//
	// It is the other half of the same question, and it is asked for a different
	// reason: the background decides which theme suits, while the pair of them is
	// what a translucent layer mixes with. A cell left at the terminal's own colours
	// is the commonest cell there is, and without both answers such a cell cannot be
	// dimmed — see [grid.Ground].
	queryForeground = "\x1b]10;?\x07"

	// queryVersion asks the terminal to name itself.
	//
	// It is the question worth asking most, because everything else this library does
	// to identify a terminal reads environment variables — and those do not survive
	// ssh, do not exist in a container, and are rewritten by a multiplexer. A terminal
	// that answers this has told the truth about itself; one that does not leaves the
	// environment as the only evidence there was.
	queryVersion = "\x1b[>0q"

	// queryDeviceVersion asks for a version number rather than a name.
	//
	// It is for the terminals that answer nothing else: Alacritty exports no version
	// in its environment and declines to answer the question above, and this is what
	// it does answer.
	queryDeviceVersion = "\x1b[>0c"

	// queryKeyboard asks which of the Kitty keyboard protocol's enhancements are
	// actually on, which is not the same as which were asked for.
	//
	// It must be written after the request that turns them on, which is what modes
	// does before any of this runs. Asking first would report the state of a terminal
	// this session had not spoken to yet.
	queryKeyboard = "\x1b[?u"

	// queryAttributes asks the terminal what it is.
	//
	// It is sent after every other question as the marker that the answers are
	// over. Every terminal answers this one, so its answer arriving without the
	// answer to the question that mattered is how a terminal says it did not
	// understand — which no terminal says any other way. Without it, learning that
	// a terminal does not support a query would cost the whole timeout, on every
	// start, for exactly the terminals that can least afford to look slow.
	queryAttributes = "\x1b[c"

	// answerGrace bounds the wait for a terminal that answers nothing at all.
	//
	// It is the backstop rather than the mechanism: a terminal that speaks at all
	// is done in one round trip, and one that is silent has a broken or filtered
	// connection. Long enough to cross a slow one, short enough that a start
	// nobody is waiting on does not look like a hang.
	answerGrace = 200 * time.Millisecond
)

// probe puts questions to the terminal during startup and collects the answers.
//
// It reads the terminal directly, which is only safe because it runs before the
// pump goroutine does: a terminal has exactly one reader, and two would race for
// the same bytes. Everything it decodes and did not ask for — a key the user
// managed to press first — is kept and handed on, along with the parser itself, so
// that a sequence which straddles the handover still decodes as one.
type probe struct {
	raw    <-chan []byte
	out    *os.File
	parser *input.Parser

	// early holds what was decoded during the probe and was not an answer.
	early []input.Event
}

// answers is what a terminal was willing to say about itself.
type answers struct {
	background grid.RGB
	hasBg      bool
	foreground grid.RGB
	hasFg      bool
	attributes input.DeviceAttributes
	hasAttrs   bool
	// name is what the terminal called itself, which outranks anything the
	// environment says.
	name    string
	hasName bool
	// version is the number a terminal gives when it will not give a name.
	version    input.DeviceVersion
	hasVersion bool
	// keyboard is which of the Kitty protocol's enhancements are on, which is not
	// the same as which were asked for.
	keyboard    input.KeyboardFeatures
	hasKeyboard bool
}

// run asks everything worth asking and returns what came back.
//
// A failure to write, a terminal that says nothing, and a terminal that answers
// only some of it are all the same kind of outcome — a session runs perfectly well
// without knowing any of this — so none of them is an error.
func (p *probe) run() answers {
	var got answers
	if _, err := p.out.WriteString(queryBackground + queryForeground + queryVersion +
		queryDeviceVersion + queryKeyboard + queryAttributes); err != nil {
		return got
	}

	// The wait ends when the attributes arrive, because they were sent last.
	timer := time.NewTimer(answerGrace)
	defer timer.Stop()
	for !got.hasAttrs {
		select {
		case chunk := <-p.raw:
			for _, ev := range p.parser.Feed(chunk) {
				p.take(ev, &got)
			}
		case <-timer.C:
			return got
		}
	}
	return got
}

// take files one decoded event: as an answer if it is one, and otherwise as input
// that belongs to the session.
func (p *probe) take(ev input.Event, got *answers) {
	switch ev := ev.(type) {
	case input.OSC:
		switch ev.Command {
		case 10:
			got.foreground, got.hasFg = parseXParseColor(ev.Params)
			return
		case 11:
			got.background, got.hasBg = parseXParseColor(ev.Params)
			return
		}
		// An answer to a question this program did not ask. It is still addressed
		// to the program rather than to the terminal, so it is passed on.
		p.early = append(p.early, ev)
	case input.DCS:
		if name, found := strings.CutPrefix(ev.Body, ">|"); found {
			got.name, got.hasName = name, true
			return
		}
		// An answer to something else. It is still addressed to the program.
		p.early = append(p.early, ev)
	case input.DeviceVersion:
		got.version, got.hasVersion = ev, true
	case input.KeyboardFlags:
		got.keyboard, got.hasKeyboard = ev.Features, true
	case input.DeviceAttributes:
		got.attributes, got.hasAttrs = ev, true
	default:
		p.early = append(p.early, ev)
	}
}

// parseXParseColor reads the colour specification a terminal answers a colour
// query with.
//
// The form is the one X has always used: "rgb:" and then three channels of one to
// four hexadecimal digits, separated by slashes. The digit count is the precision,
// so "f" and "ffff" are both full brightness and "8" is not the same as "0008" —
// which is why each channel is scaled by the largest value its own width could
// hold rather than simply padded.
func parseXParseColor(spec string) (grid.RGB, bool) {
	body, ok := strings.CutPrefix(spec, "rgb:")
	if !ok {
		return grid.RGB{}, false
	}
	parts := strings.Split(body, "/")
	if len(parts) != 3 {
		return grid.RGB{}, false
	}
	var c grid.RGB
	for i, channel := range parts {
		v, ok := parseChannel(channel)
		if !ok {
			return grid.RGB{}, false
		}
		switch i {
		case 0:
			c.R = v
		case 1:
			c.G = v
		case 2:
			c.B = v
		}
	}
	return c, true
}

// parseChannel reads one channel of a colour specification and scales it to the
// eight bits a cell holds.
func parseChannel(s string) (uint8, bool) {
	if len(s) == 0 || len(s) > 4 {
		return 0, false
	}
	value, full := 0, 0
	for i := range len(s) {
		digit, ok := hexDigit(s[i])
		if !ok {
			return 0, false
		}
		value = value*16 + digit
		full = full*16 + 15
	}
	return scaleChannel(value, full), true
}

// scaleChannel turns a channel worth full at its own width into the eight bits a
// cell holds.
//
// Rounding to nearest rather than towards zero: without it every channel loses up
// to a step, and a white background answered as "f/f/f" comes back not quite
// white. The answer is given in the type it is used as, and clamped on the way, so
// the narrowing is visible here instead of being a conversion whose safety has to
// be reconstructed by reading the caller.
func scaleChannel(value, full int) uint8 {
	scaled := (value*255 + full/2) / full
	switch {
	case scaled <= 0:
		return 0
	case scaled >= 255:
		return 255
	default:
		return uint8(scaled)
	}
}

func hexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	default:
		return 0, false
	}
}
