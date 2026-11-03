package graphics

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const ReplyPrefix = "Gi="

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

// ParseReply preserves a readable request number when another field is malformed,
// so its owner can fail the request instead of attributing a guessed identity.
func ParseReply(body string) (Reply, error) {
	var reply Reply
	header, message, found := strings.Cut(body, ";")
	fields, introduced := strings.CutPrefix(header, ReplyPrefix)
	if !found || !introduced {
		return reply, ErrMalformedReply
	}
	id, rest, more := strings.Cut(fields, ",")
	value, err := strconv.ParseUint(id, 10, 32)
	valid := err == nil
	reply.ID, reply.Message = uint32(value), message
	var number, placement replyNumber
	if more {
		for field := range strings.SplitSeq(rest, ",") {
			key, raw, separated := strings.Cut(field, "=")
			valid = separated && valid
			switch key {
			case "I":
				valid = number.read(raw) && valid
			case "p":
				valid = placement.read(raw) && valid
			default:
				valid = false
			}
		}
	}
	reply.Number, reply.Placement = number.identity(), placement.identity()
	if !valid || message == "" || message == replyAccepted && reply.ID == 0 {
		return reply, ErrMalformedReply
	}
	return reply, nil
}

type replyNumber struct {
	value          uint32
	seen, conflict bool
}

func (n *replyNumber) read(raw string) bool {
	value, err := strconv.ParseUint(raw, 10, 32)
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
	return writeString(w, fmt.Sprintf("\x1b_%s%d,a=q,t=d,f=%d;%s\x1b\\", ReplyPrefix, id, kittyPNG, queryPixelPNG))
}
