package headless

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Ask is the label and the choices, numbered.
func (s *Select[T]) Ask() string {
	return s.Label + choices(s.list.items)
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
	}, s.checkAnswer)
}

// Ask is the label and the choices, numbered, with a word about giving several.
func (m *MultiSelect[T]) Ask() string {
	return m.Label + choices(m.list.items) + " — several, separated by commas"
}

// Reply takes numbers or labels, separated by commas. Nothing at all takes nothing,
// which is how a reader says they want none of them.
func (m *MultiSelect[T]) Reply(said string) error {
	return m.validate(func() error {
		if !m.beginOperation() {
			return ErrValidationSuperseded
		}
		taken := make([]bool, len(m.list.items))
		count := 0
		for part := range strings.SplitSeq(said, ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			at, err := choose(part, m.list.items)
			if err != nil {
				return err
			}
			if !taken[at] {
				taken[at], count = true, count+1
			}
		}
		if m.limit > 0 && count > m.limit {
			return fmt.Errorf("at most %d may be chosen", m.limit)
		}
		current, _ := m.selection()
		if !slices.Equal(current, taken) {
			m.store(taken)
		}
		return nil
	}, m.checkAnswer)
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

func choose[T any](said string, options []Option[T]) (int, error) {
	said = strings.TrimSpace(said)
	if said == "" {
		return 0, errors.New("nothing was said")
	}
	if n, err := strconv.Atoi(said); err == nil {
		if n < 1 || n > len(options) {
			return 0, fmt.Errorf("there is no choice %d", n)
		}
		return n - 1, nil
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
