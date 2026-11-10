package base

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrReplyUnavailable means an answer could not be applied by its owner. A spoken
// adapter must stop instead of asking the user to correct or repeat that input.
var ErrReplyUnavailable = errors.New("base: reply unavailable")

// SpokenField is a field that can be answered in words, one question and one
// reply at a time, without its scene element. Reply validates like keyboard input:
// a rejected reply leaves the answer unchanged and explains why, so the caller can
// ask again; ErrReplyUnavailable and ErrValidationSuperseded mean asking again
// cannot help.
type SpokenField interface {
	Field
	Ask() string
	Reply(said string) error
}

var (
	_ SpokenField = (*Text)(nil)
	_ SpokenField = (*Confirm)(nil)
	_ SpokenField = (*Select[struct{}])(nil)
	_ SpokenField = (*MultiSelect[struct{}])(nil)
)

func (s *Select[T]) Ask() string {
	return s.label + choices(s.list.items)
}

// Reply takes a number or one of the labels.
func (s *Select[T]) Reply(said string) error {
	return s.validate(func() error {
		at, err := choose(said, s.list.items)
		if err != nil {
			return err
		}
		s.selectAt(at)
		return nil
	}, s.Revision, s.checkAnswer)
}

func (m *MultiSelect[T]) Ask() string {
	return m.label + choices(m.list.items) + " — several, separated by commas"
}

// Reply takes numbers or labels, separated by commas. A reply that is exactly one
// label is that choice even when the label itself contains a comma. Nothing at all
// takes nothing, which is how a reader says they want none of them.
func (m *MultiSelect[T]) Reply(said string) error {
	return m.validate(func() error {
		parts := strings.Split(said, ",")
		if named := namedBy(strings.TrimSpace(said), m.list.items); named.exact >= 0 && !named.ambiguousExact {
			parts = []string{said}
		}
		var values []T
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			at, err := choose(part, m.list.items)
			if err != nil {
				return err
			}
			values = append(values, m.list.items[at].Value)
		}
		taken, _ := m.indices(m.list.items, values)
		if !m.admits(len(taken)) {
			return fmt.Errorf("at most %d may be chosen", m.limit)
		}
		m.store(taken)
		return nil
	}, m.Revision, m.checkAnswer)
}

func choices[T any](options []Option[T]) string {
	var b strings.Builder
	for i, option := range options {
		b.WriteString(" ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(") ")
		b.WriteString(option.Label)
	}
	if b.Len() == 0 {
		return ""
	}
	return ":" + b.String()
}

// choose reads a reply as a label or a 1-based number. A label said exactly is
// what was meant, so it wins over the number it might also be; a numeral that is
// one choice's number and another choice's label cannot be settled and is refused.
func choose[T any](said string, options []Option[T]) (int, error) {
	said = strings.TrimSpace(said)
	n, err := strconv.Atoi(said)
	if err != nil {
		return chooseLabel(said, options)
	}
	named := namedBy(said, options)
	numbered := n >= 1 && n <= len(options)
	switch {
	case named.ambiguousExact:
		return 0, fmt.Errorf("%q names more than one choice", said)
	case named.exact >= 0 && numbered && named.exact != n-1:
		return 0, fmt.Errorf("%q is the number of one choice and the label of another", said)
	case named.exact >= 0:
		return named.exact, nil
	case !numbered:
		return 0, fmt.Errorf("there is no choice %d", n)
	default:
		return n - 1, nil
	}
}

func chooseLabel[T any](said string, options []Option[T]) (int, error) {
	if said == "" {
		return 0, errors.New("nothing was said")
	}
	switch named := namedBy(said, options); {
	case named.ambiguousExact:
		return 0, fmt.Errorf("%q names more than one choice", said)
	case named.exact >= 0:
		return named.exact, nil
	case named.ambiguousPrefix:
		return 0, fmt.Errorf("%q could be more than one of them", said)
	case named.prefix < 0:
		return 0, fmt.Errorf("%q is not one of the choices", said)
	default:
		return named.prefix, nil
	}
}

// naming is what a spoken label reached: the option whose label it is, the first one
// it is the beginning of, and whether either was reached more than once.
type naming struct {
	exact, prefix                   int
	ambiguousExact, ambiguousPrefix bool
}

func namedBy[T any](said string, options []Option[T]) naming {
	folded := strings.ToLower(said)
	found := naming{exact: -1, prefix: -1}
	for i, option := range options {
		label := strings.ToLower(option.Label)
		switch {
		case label == folded:
			if found.exact >= 0 {
				found.ambiguousExact = true
				continue
			}
			found.exact = i
		case strings.HasPrefix(label, folded):
			if found.prefix >= 0 {
				found.ambiguousPrefix = true
				continue
			}
			found.prefix = i
		}
	}
	return found
}
