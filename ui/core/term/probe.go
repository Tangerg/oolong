package term

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

const (
	foregroundCommand = 10
	backgroundCommand = 11

	// A direct name reply describes the destination; environment hints can
	// instead describe an outer terminal or the remote host.
	queryVersion = "\x1b[>0q"

	// Numeric identity probes cover peers that omit a human-readable name.
	queryDeviceVersion = "\x1b[>0c"

	// Send DA1 last so its reply can settle earlier unanswered queries
	// without spending the full timeout. Silent peers still need answerGrace.
	queryAttributes = "\x1b[c"

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
	kitty      graphicsProbeResult
	background grid.RGB
	hasBg      bool
	foreground grid.RGB
	hasFg      bool
	attributes input.DeviceAttributes
	hasAttrs   bool
	// name is what the terminal called itself, which outranks anything the
	// environment says.
	name       string
	hasName    bool
	version    input.DeviceVersion
	hasVersion bool
	// keyboard is which of the Kitty protocol's enhancements are on, which is not
	// the same as which were asked for.
	keyboard    input.KeyboardFeatures
	hasKeyboard bool
}

// Capability discovery is optional: write failure, silence, and partial replies
// retain the session's unprobed defaults instead of preventing startup.
func (p *probe) run() answers {
	var got answers
	// One budget for the whole exchange, held over the transport as well as the wait.
	if err := p.out.SetWriteDeadline(p.deadline); err != nil {
		return got
	}
	defer func() { _ = p.out.SetWriteDeadline(time.Time{}) }()
	// Query correlation shares the nonwrapping graphics sequence; a=q avoids
	// retaining or replacing an image while testing the direct PNG path.
	id, err := nextGraphicsRequestNumber()
	if err != nil {
		return got
	}
	p.graphicsQueryID = id
	var query strings.Builder
	if err := graphics.QueryPNG(&query, p.graphicsQueryID); err != nil {
		return got
	}
	if _, err := p.out.WriteString(colorQuery(backgroundCommand) + colorQuery(foregroundCommand) + queryVersion +
		queryDeviceVersion + query.String() + queryAttributes); err != nil {
		return got
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
	for !got.hasAttrs {
		due.Schedule(p.stream.DueAt())
		select {
		case result := <-p.raw:
			if !p.read(result, &got) {
				return got
			}
		case <-due.Channel():
			if !p.drainRaw(&got) {
				return got
			}
			for _, ev := range p.stream.Expire(time.Now()) {
				p.take(ev, &got)
			}
		case <-timer.C:
			return got
		}
	}
	return got
}

func (p *probe) read(result readResult, got *answers) bool {
	p.readBytes += len(result.data)
	for _, ev := range p.stream.Feed(result.data, result.at) {
		p.take(ev, got)
	}
	if result.err != nil {
		result.data = nil
		p.end = &result
		return false
	}
	return p.readBytes < probeInputBudget
}

func (p *probe) drainRaw(got *answers) bool {
	for !got.hasAttrs {
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
	if got.hasAttrs {
		p.early = append(p.early, ev)
		return
	}
	switch ev := ev.(type) {
	case input.OSC:
		switch ev.Command {
		case foregroundCommand:
			got.foreground, got.hasFg = parseXParseColor(ev.Params)
			return
		case backgroundCommand:
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
		p.early = append(p.early, ev)
	case input.APC:
		result := p.graphicsResult(ev.Body)
		if result == graphicsUnanswered || got.kitty != graphicsUnanswered {
			p.early = append(p.early, ev)
			return
		}
		got.kitty = result
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

func colorQuery(command int) string { return fmt.Sprintf("\x1b]%d;?\x07", command) }

// DA1 is sent last. Once it arrives, a missing Kitty reply rules out direct
// graphics on this channel, even when TERM names the outer terminal.
func (a answers) graphics(hint graphics.Protocol) graphics.Protocol {
	if a.kitty == graphicsAccepted {
		return graphics.Kitty
	}
	sixel := a.hasAttrs && a.attributes.Has(sixelAttribute)
	protocol := hint.Negotiated(a.name, sixel)
	if protocol == graphics.Kitty && (a.hasAttrs || a.kitty == graphicsRejected) {
		if sixel {
			return graphics.Sixel
		}
		return graphics.None
	}
	return protocol
}

// X color channels encode precision in their digit count: f and ffff both
// mean full brightness. Scale by each channel's maximum instead of padding it.
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

// Round channel scaling to nearest to avoid a systematic downward bias. Clamp
// before narrowing so the returned byte cannot wrap.
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
