package term

import (
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/internal/known"
)

const (
	// Bound startup even when a terminal ignores or filters every query.
	answerGrace = 200 * time.Millisecond

	// Startup cannot apply event backpressure until Open returns. Yield the
	// shared decoder to the pump after a bounded prefix, even during a flood.
	probeInputBudget = 16 << 10
)

// queryWriter is the terminal as a probe needs it: somewhere to write a question,
// and a way to stop waiting to be allowed to. The second is not optional. A write
// that cannot time out never reaches the wait that bounds the rest, and until Open
// returns there is no owner whose Close could give raw mode back.
type queryWriter interface {
	io.StringWriter
	SetWriteDeadline(deadline time.Time) error
}

// Startup probing and the later input pump share one Stream and never read
// concurrently. This preserves incomplete framing, Escape deadlines, and arrival
// times while retaining user events received among capability replies.
type probe struct {
	raw    <-chan readResult
	out    queryWriter
	stream *input.Stream
	// deadline is when the whole exchange must be over, asking included. It is set
	// by whoever started the probe, which is also what holds the transport to the
	// same instant; measuring the wait from after the write would leave the write
	// itself unbounded.
	deadline  time.Time
	readBytes int

	graphicsQueryID uint32
	early           []input.Event
	end             *readResult
}

type graphicsProbeResult uint8

const (
	graphicsUnanswered graphicsProbeResult = iota
	graphicsAccepted
	graphicsRejected
)

type answers struct {
	kitty graphicsProbeResult
	// ground keeps an unanswered or unreadable color as the terminal default,
	// which is exactly what blending must not guess past.
	ground     grid.Ground
	attributes known.Value[input.DeviceAttributes]
	// name is what the terminal called itself, which outranks anything the
	// environment says.
	name    known.Value[string]
	version known.Value[input.DeviceVersion]
	// keyboard is which of the Kitty protocol's enhancements are on, which is not
	// the same as which were asked for.
	keyboard known.Value[input.KeyboardFeatures]
	cursor   known.Value[ansi.CursorStyle]
}

// answered reports the DA1 reply, which ends the exchange because DA1 is asked last.
func (a answers) answered() bool {
	_, ok := a.attributes.Get()
	return ok
}

// Missing replies leave capabilities unknown. A transport failure cannot safely
// become an unprobed session: a query prefix may still frame subsequent output.
func (p *probe) run() (got answers, err error) {
	// One budget for the whole exchange, held over the transport as well as the wait.
	if deadlineErr := p.out.SetWriteDeadline(p.deadline); deadlineErr != nil {
		return got, deadlineErr
	}
	defer func() { err = errors.Join(err, p.out.SetWriteDeadline(time.Time{})) }()
	// Query correlation shares the nonwrapping graphics sequence; a=q avoids
	// retaining or replacing an image while testing the direct PNG path.
	id, err := nextGraphicsRequestNumber()
	if err != nil {
		return got, err
	}
	p.graphicsQueryID = id
	var query strings.Builder
	if queryErr := graphics.QueryPNG(&query, p.graphicsQueryID); queryErr != nil {
		return got, queryErr
	}
	sequence := colorQuery(ansi.OSCBackground) + colorQuery(ansi.OSCForeground) + ansi.QueryTerminalVersion +
		ansi.QuerySecondaryAttributes + ansi.QueryCursorStyle + query.String() + ansi.QueryPrimaryAttributes
	written, err := p.out.WriteString(sequence)
	if err != nil {
		return got, err
	}
	if written != len(sequence) {
		return got, io.ErrShortWrite
	}

	// The wait ends when the attributes arrive, because they were sent last. What is
	// left of the budget is what asking did not spend.
	timer := time.NewTimer(time.Until(p.deadline))
	defer timer.Stop()
	// An ambiguous escape waits out its own grace here exactly as it would once the
	// pump is running. The two deadlines are unrelated: one bounds the exchange, the
	// other settles a keystroke, and a probe that woke only for the first would carry
	// an escape past every byte that followed it.
	due := deadline.NewTimer()
	defer due.Stop()
	for !got.answered() {
		due.Schedule(p.stream.DueAt())
		select {
		case result := <-p.raw:
			if !p.read(result, &got) {
				return got, nil
			}
		case <-due.Channel():
			if !p.drainRaw(&got) {
				return got, nil
			}
			for _, ev := range p.stream.Expire(time.Now()) {
				p.take(ev, &got)
			}
		case <-timer.C:
			return got, nil
		}
	}
	return got, nil
}

func (p *probe) read(result readResult, got *answers) bool {
	p.readBytes += len(result.data)
	for _, ev := range result.decode(p.stream) {
		p.take(ev, got)
	}
	if result.err != nil {
		p.end = &readResult{at: result.at, err: result.err}
		return false
	}
	return p.readBytes < probeInputBudget
}

func (p *probe) drainRaw(got *answers) bool {
	for !got.answered() {
		select {
		case result := <-p.raw:
			if !p.read(result, got) {
				return false
			}
		default:
			return true
		}
	}
	return false
}

func (p *probe) take(ev input.Event, got *answers) {
	// The marker ends the exchange even in the middle of one transport read.
	if got.answered() {
		p.early = append(p.early, ev)
		return
	}
	switch ev := ev.(type) {
	case input.OSC:
		switch ev.Command {
		case ansi.OSCForeground:
			got.ground.FG = reportedColor(ev.Params)
			return
		case ansi.OSCBackground:
			got.ground.BG = reportedColor(ev.Params)
			return
		}
		// An answer to a question this program did not ask. It is still addressed
		// to the program rather than to the terminal, so it is passed on.
		p.early = append(p.early, ev)
	case input.DCS:
		if style, ok := ansi.ParseCursorStyleReply(ev.Body); ok {
			if _, learned := got.cursor.Get(); !learned {
				got.cursor = known.Of(style)
			}
			return
		}
		if name, found := strings.CutPrefix(ev.Body, ">|"); found {
			got.name = known.Of(name)
			return
		}
		p.early = append(p.early, ev)
	case input.APC:
		result := p.graphicsResult(ev.Body)
		if result == graphicsUnanswered || got.kitty != graphicsUnanswered {
			p.early = append(p.early, ev)
			return
		}
		got.kitty = result
	case input.DeviceVersion:
		got.version = known.Of(ev)
	case input.KeyboardFlags:
		got.keyboard = known.Of(ev.Features)
	case input.DeviceAttributes:
		got.attributes = known.Of(ev)
	default:
		p.early = append(p.early, ev)
	}
}

func (p *probe) graphicsResult(body string) graphicsProbeResult {
	reply, err := graphics.ParseReply(body)
	if err != nil || p.graphicsQueryID == 0 || reply.ID != p.graphicsQueryID || reply.Number != 0 || reply.Placement != 0 {
		return graphicsUnanswered
	}
	if reply.Err() != nil {
		return graphicsRejected
	}
	return graphicsAccepted
}

func colorQuery(command int) string {
	return ansi.OSCPrefix(command) + "?" + commandEnd
}

// sixelAttribute is the DA1 attribute a terminal reports for Sixel graphics.
const sixelAttribute = 4

// DA1 is sent last. Once it arrives, a missing Kitty reply rules out direct
// graphics on this channel, even when TERM names the outer terminal.
func (a answers) graphics(hint graphics.Protocol) graphics.Protocol {
	if a.kitty == graphicsAccepted {
		return graphics.Kitty
	}
	attributes, answered := a.attributes.Get()
	sixel := answered && attributes.Has(sixelAttribute)
	name, _ := a.name.Get()
	protocol := hint.Negotiated(name, sixel)
	if protocol == graphics.Kitty && (answered || a.kitty == graphicsRejected) {
		if sixel {
			return graphics.Sixel
		}
		return graphics.None
	}
	return protocol
}

func reportedColor(spec string) grid.Color {
	rgb, ok := parseXParseColor(spec)
	if !ok {
		return grid.Color{}
	}
	return grid.RGBColor(rgb.R, rgb.G, rgb.B)
}

// X color channels encode precision in their digit count: f and ffff both
// mean full brightness. Scale by each channel's maximum instead of padding it.
func parseXParseColor(spec string) (grid.RGB, bool) {
	body, ok := strings.CutPrefix(spec, "rgb:")
	if !ok {
		return grid.RGB{}, false
	}
	parts := strings.Split(body, "/")
	var channels [3]uint8
	if len(parts) != len(channels) {
		return grid.RGB{}, false
	}
	for i, part := range parts {
		if channels[i], ok = parseChannel(part); !ok {
			return grid.RGB{}, false
		}
	}
	return grid.RGB{R: channels[0], G: channels[1], B: channels[2]}, true
}

// maxChannelDigits is the widest XParseColor channel: sixteen bits of hex.
const (
	maxChannelDigits = 4
	hexDigitBits     = 4
)

func parseChannel(s string) (uint8, bool) {
	if len(s) == 0 || len(s) > maxChannelDigits {
		return 0, false
	}
	value, err := strconv.ParseUint(s, 16, hexDigitBits*maxChannelDigits)
	if err != nil {
		return 0, false
	}
	full := uint64(1)<<(hexDigitBits*len(s)) - 1
	return scaleChannel(value, full), true
}

// Round channel scaling to nearest to avoid a systematic downward bias. Clamp
// before narrowing so the returned byte cannot wrap.
func scaleChannel(value, full uint64) uint8 {
	scaled := (value*math.MaxUint8 + full/2) / full
	if scaled >= math.MaxUint8 {
		return math.MaxUint8
	}
	return uint8(scaled)
}
