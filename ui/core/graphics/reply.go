package graphics

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	graphicsCommand  = 'G'
	imageIDField     = "i"
	imageNumberField = "I"
	placementField   = "p"
	wireIntegerBits  = 32
)

const replyAccepted = "OK"

const queryPixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNgYGAAAAAEAAH2FzhVAAAAAElFTkSuQmCC"

var (
	ErrMalformedReply = errors.New("graphics: malformed terminal reply")
	ErrRejected       = errors.New("graphics: terminal rejected image")
)

type Reply struct {
	ID, Number, Placement uint32
	Message               string
}

func (r Reply) Err() error {
	if r.Message == replyAccepted {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrRejected, r.Message)
}

func (r Reply) valid() bool {
	return r.Message != "" && (r.ID != 0 || r.Message != replyAccepted && r.Number != 0)
}

// MatchReplyPrefix keeps a terminal response distinct from Alt+_ typing. A
// pending prefix needs more bytes; an unmatched, nonpending prefix is ordinary
// input. Only the response header is recognized here, not its field values.
func MatchReplyPrefix(body []byte) (matched, pending bool) {
	_, matched, pending = replyFields(string(body))
	return matched, pending
}

func replyFields(body string) (fields string, matched, pending bool) {
	if body == "" {
		return "", false, true
	}
	if body[0] != graphicsCommand {
		return "", false, false
	}
	// A failed allocation can omit i while retaining the separator before I.
	fields, _ = strings.CutPrefix(body[1:], ",")
	if fields == "" {
		return "", false, true
	}
	if fields[0] != imageIDField[0] && fields[0] != imageNumberField[0] {
		return "", false, false
	}
	if len(fields) == 1 {
		return "", false, true
	}
	return fields, fields[1] == '=', false
}

// ParseReply preserves a readable request number when another field is malformed,
// so its owner can fail the request instead of attributing a guessed identity.
func ParseReply(body string) (Reply, error) {
	var reply Reply
	header, message, found := strings.Cut(body, ";")
	fields, introduced, _ := replyFields(header)
	if !found || !introduced {
		return reply, ErrMalformedReply
	}
	valid := true
	var id, number, placement replyNumber
	for field := range strings.SplitSeq(fields, ",") {
		key, raw, separated := strings.Cut(field, "=")
		valid = separated && valid
		switch key {
		case imageIDField:
			valid = id.read(raw) && valid
		case imageNumberField:
			valid = number.read(raw) && valid
		case placementField:
			valid = placement.read(raw) && valid
		default:
			valid = false
		}
	}
	reply = Reply{ID: id.identity(), Number: number.identity(), Placement: placement.identity(), Message: message}
	if !valid || !reply.valid() {
		return reply, ErrMalformedReply
	}
	return reply, nil
}

type replyNumber struct {
	value          uint32
	seen, conflict bool
}

func (n *replyNumber) read(raw string) bool {
	value, err := strconv.ParseUint(raw, 10, wireIntegerBits)
	if err != nil || n.seen {
		n.conflict, n.seen = true, true
		return false
	}
	n.value, n.seen = uint32(value), true
	return true
}

func (n *replyNumber) identity() uint32 {
	if n.conflict {
		return 0
	}
	return n.value
}

// QueryPNG tests the same direct PNG path used for uploads without replacing or
// retaining any existing terminal image with the correlation identity.
func QueryPNG(w io.Writer, id uint32) error {
	if id == 0 {
		return errImageIdentity
	}
	return writeString(w, fmt.Sprintf("\x1b_%c%s=%d,a=q,t=d,f=%d;%s\x1b\\", graphicsCommand, imageIDField, id, kittyPNG, queryPixelPNG))
}
