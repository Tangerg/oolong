package ansi

import (
	"iter"
	"slices"
	"strings"
)

// Params is a sequence's parameter section, parsed once and read by whichever
// decoder the final byte selects.
//
// There is one parser rather than one per sequence family. Two parsers over the
// same syntax drift: they disagree about what an empty field means, or about what
// to do with a field that is not a number, and the sequence that exercises the
// difference is the one nobody tested.
type Params struct {
	marker byte
	groups []Parameter
}

// Parameter is one semicolon-separated parameter and its colon-separated
// subparameters.
//
// A field left empty is the protocol's default, which is zero. A field that is not
// a number, or is far larger than any parameter legitimately gets, is -1: a decoder
// can then refuse the sequence instead of acting on a value invented for it.
// Parameter is an immutable view into its [Params]; its methods do not expose the
// parser's storage.
type Parameter struct{ values []int }

// Limit is well past any real parameter and short of anything that could overflow.
// A number beyond it is treated as malformed.
const Limit = 1 << 20

// Parse reads a control sequence's parameter section — everything between the
// introducer and the final byte.
func Parse(body string) Params {
	var ps Params
	if body != "" && body[0] >= 0x3c && body[0] <= 0x3f {
		ps.marker = body[0]
		body = body[1:]
	}
	if body == "" {
		return ps
	}
	fields := strings.Split(body, ";")
	ps.groups = make([]Parameter, 0, len(fields))
	for _, field := range fields {
		subs := strings.Split(field, ":")
		group := make([]int, 0, len(subs))
		for _, sub := range subs {
			group = append(group, parseParam(sub))
		}
		ps.groups = append(ps.groups, Parameter{values: group})
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
func (ps Params) Empty() bool { return len(ps.groups) == 0 }

// Marker is the byte a private sequence begins with — '<' for a mouse report, '?'
// for a terminal's answer about a mode — or zero for an ordinary sequence.
func (ps Params) Marker() byte { return ps.marker }

// Valid reports whether every parameter field was numeric and within [Limit]. Empty
// fields are valid protocol defaults; malformed and excessive fields are represented
// by a negative value and make the complete parameter section invalid.
func (ps Params) Valid() bool {
	for _, group := range ps.groups {
		for value := range group.Values() {
			if value < 0 {
				return false
			}
		}
	}
	return true
}

// First is the leading parameter, or zero when there was none. Zero is the
// protocol's own default for a missing parameter, so a caller need not distinguish.
func (ps Params) First() int { return ps.At(0) }

// At is the leading value of group i, or zero when the group is absent.
func (ps Params) At(i int) int {
	group := ps.Group(i)
	return group.At(0)
}

// Group is parameter i and its subparameters, or an empty parameter when i is
// outside the sequence.
func (ps Params) Group(i int) Parameter {
	if i < 0 || i >= len(ps.groups) {
		return Parameter{}
	}
	return ps.groups[i]
}

// Count is how many parameter groups the sequence carried.
func (ps Params) Count() int { return len(ps.groups) }

// Len is how many fields the parameter carries, including its leading value.
func (p Parameter) Len() int { return len(p.values) }

// At returns field i, or zero when i is outside the parameter. Zero is also the
// protocol default for an omitted field.
func (p Parameter) At(i int) int {
	if i < 0 || i >= len(p.values) {
		return 0
	}
	return p.values[i]
}

// Values visits the parameter's fields in wire order without exposing mutable
// parser storage.
func (p Parameter) Values() iter.Seq[int] { return slices.Values(p.values) }
