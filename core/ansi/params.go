package ansi

import "strings"

// Params is a sequence's parameter section, parsed once and read by whichever
// decoder the final byte selects.
//
// There is one parser rather than one per sequence family. Two parsers over the
// same syntax drift: they disagree about what an empty field means, or about what
// to do with a field that is not a number, and the sequence that exercises the
// difference is the one nobody tested.
type Params struct {
	// Private is the marker byte a private sequence begins with — '<' for a mouse
	// report, '?' for a terminal's answer about a mode — or zero for an ordinary
	// sequence.
	Private byte
	// Groups are the semicolon-separated parameters, each of which may carry
	// colon-separated subparameters.
	//
	// A field left empty is the protocol's default, which is zero. A field that is
	// not a number, or is far larger than any parameter legitimately gets, is -1: a
	// decoder can then refuse the sequence instead of acting on a value invented
	// for it.
	Groups [][]int
}

// Limit is well past any real parameter and short of anything that could overflow.
// A number beyond it is treated as malformed.
const Limit = 1 << 20

// Parse reads a control sequence's parameter section — everything between the
// introducer and the final byte.
func Parse(body string) Params {
	var ps Params
	if body != "" && body[0] >= 0x3c && body[0] <= 0x3f {
		ps.Private = body[0]
		body = body[1:]
	}
	if body == "" {
		return ps
	}
	fields := strings.Split(body, ";")
	ps.Groups = make([][]int, 0, len(fields))
	for _, field := range fields {
		subs := strings.Split(field, ":")
		group := make([]int, 0, len(subs))
		for _, sub := range subs {
			group = append(group, parseParam(sub))
		}
		ps.Groups = append(ps.Groups, group)
	}
	return ps
}

func parseParam(s string) int {
	if s == "" {
		return 0
	}
	value := 0
	for i := range len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			return -1
		}
		value = value*10 + int(c-'0')
		if value > Limit {
			return -1
		}
	}
	return value
}

// Empty reports whether the sequence carried no parameters.
func (ps Params) Empty() bool { return len(ps.Groups) == 0 }

// First is the leading parameter, or zero when there was none. Zero is the
// protocol's own default for a missing parameter, so a caller need not distinguish.
func (ps Params) First() int { return ps.At(0) }

// At is the leading value of group i, or zero when the group is absent.
func (ps Params) At(i int) int {
	group := ps.Group(i)
	if len(group) == 0 {
		return 0
	}
	return group[0]
}

// Group is parameter i and its subparameters, or nothing when there is no such
// parameter.
func (ps Params) Group(i int) []int {
	if i < 0 || i >= len(ps.Groups) {
		return nil
	}
	return ps.Groups[i]
}

// Count is how many parameter groups the sequence carried.
func (ps Params) Count() int { return len(ps.Groups) }
