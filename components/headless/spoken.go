package headless

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Ask is the label, with the placeholder as the hint it already is.
func (t *Text) Ask() string {
	if t.Placeholder == "" {
		return t.Label
	}
	return t.Label + " (" + t.Placeholder + ")"
}

// Reply takes what was said as the whole of the answer.
func (t *Text) Reply(said string) error {
	t.ensure()
	before := t.editor.Revision()
	t.editor.SetText(said)
	t.storeSince(before)
	return t.Validate()
}

// Ask is the label and the choices, numbered.
func (s *Select[T]) Ask() string {
	s.ensure()
	return s.Label + choices(s.options)
}

// Reply takes a number or one of the labels.
func (s *Select[T]) Reply(said string) error {
	s.ensure()
	at, err := choose(said, s.options)
	if err != nil {
		return s.check(err)
	}
	s.list.Select(at)
	return s.Validate()
}

// Ask is the label and the choices, numbered, with a word about giving several.
func (m *MultiSelect[T]) Ask() string {
	m.ensure()
	return m.Label + choices(m.options) + " — several, separated by commas"
}

// Reply takes numbers or labels, separated by commas. Nothing at all takes nothing,
// which is how a reader says they want none of them.
func (m *MultiSelect[T]) Reply(said string) error {
	m.ensure()
	taken := make([]bool, len(m.options))
	count := 0
	for part := range strings.SplitSeq(said, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		at, err := choose(part, m.options)
		if err != nil {
			return m.check(err)
		}
		if !taken[at] {
			taken[at], count = true, count+1
		}
	}
	if m.limit > 0 && count > m.limit {
		return m.check(fmt.Errorf("at most %d may be chosen", m.limit))
	}
	if !slices.Equal(m.taken, taken) {
		m.taken = taken
		m.store()
	}
	return m.Validate()
}

// Ask is the question and the two words that answer it.
func (c *Confirm) Ask() string {
	c.ensure()
	return c.Label + " (" + c.word(true) + "/" + c.word(false) + ")"
}

// Reply takes either answer, or as much of one as is unambiguous — nobody types
// "yes" in full twice.
func (c *Confirm) Reply(said string) error {
	c.ensure()
	said = strings.ToLower(strings.TrimSpace(said))
	yes, no := strings.ToLower(c.word(true)), strings.ToLower(c.word(false))
	yesMatch, noMatch := strings.HasPrefix(yes, said), strings.HasPrefix(no, said)
	switch {
	case said == "":
		return c.check(fmt.Errorf("say %s or %s", yes, no))
	case yesMatch && noMatch:
		return c.check(fmt.Errorf("%q could mean either %s or %s", said, yes, no))
	case yesMatch:
		c.Say(true)
	case noMatch:
		c.Say(false)
	default:
		return c.check(fmt.Errorf("say %s or %s", yes, no))
	}
	return c.Validate()
}

// choices is the options, numbered, as they go after a question.
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

// choose reads what was said as one of the options: the number beside it, or its
// label — whole, or as much of it as picks out one.
//
// A label that is a prefix of two of them is refused rather than guessed at. A form
// answered in words is answered by somebody who cannot see which one was taken, so a
// guess is a wrong answer they have no way of noticing.
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

	folded := strings.ToLower(said)
	exact, prefix := -1, -1
	ambiguousExact, ambiguousPrefix := false, false
	for i, option := range options {
		label := strings.ToLower(option.Label)
		if label == folded {
			if exact >= 0 {
				ambiguousExact = true
			} else {
				exact = i
			}
			continue
		}
		if !strings.HasPrefix(label, folded) {
			continue
		}
		if prefix >= 0 {
			ambiguousPrefix = true
		} else {
			prefix = i
		}
	}
	if ambiguousExact {
		return 0, fmt.Errorf("%q names more than one choice", said)
	}
	if exact >= 0 {
		return exact, nil
	}
	if ambiguousPrefix {
		return 0, fmt.Errorf("%q could be more than one of them", said)
	}
	if prefix < 0 {
		return 0, fmt.Errorf("%q is not one of the choices", said)
	}
	return prefix, nil
}
