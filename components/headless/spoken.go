package headless

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Spoken is a field that can be asked and answered in words.
//
// It is the same field, answered without a grid: a question in one line, and a line
// back. That is what somebody reading with a screen reader has, what a program has
// when its output is a pipe, and what a test has when it would rather say "good"
// than press the down arrow twice.
//
// The questions are the field's because only the field knows what it is asking and
// what an answer means. Reading a line and writing one is nobody's business here —
// see the conversation in the kit package for one way to do it, and note that it is
// thirty lines, which is the point of putting the hard half in the fields.
type Spoken interface {
	Field

	// Ask is the question, in one line: what the field wants, and what may be said in
	// reply.
	Ask() string
	// Reply takes what was said and reports what was wrong with it — the same
	// complaint the field would have shown on screen, which is why a form answered
	// this way ends up in exactly the state one answered the other way does.
	Reply(said string) error
}

// The four fields answer in words as well as on screen.
var (
	_ Spoken = (*Text)(nil)
	_ Spoken = (*Confirm)(nil)
	_ Spoken = (*Select[string])(nil)
	_ Spoken = (*MultiSelect[string])(nil)
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
	t.editor.SetText(said)
	t.store()
	return t.Validate()
}

// Ask is the label and the choices, numbered.
func (s *Select[T]) Ask() string {
	s.ensure()
	return s.Label + choices(s.Options)
}

// Reply takes a number or one of the labels.
func (s *Select[T]) Reply(said string) error {
	s.ensure()
	at, err := choose(said, s.Options)
	if err != nil {
		return s.check(err)
	}
	s.list.Select(at)
	s.store()
	return s.Validate()
}

// Ask is the label and the choices, numbered, with a word about giving several.
func (m *MultiSelect[T]) Ask() string {
	m.ensure()
	return m.Label + choices(m.Options) + " — several, separated by commas"
}

// Reply takes numbers or labels, separated by commas. Nothing at all takes nothing,
// which is how a reader says they want none of them.
func (m *MultiSelect[T]) Reply(said string) error {
	m.ensure()
	taken := make([]bool, len(m.Options))
	count := 0
	for part := range strings.SplitSeq(said, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		at, err := choose(part, m.Options)
		if err != nil {
			return m.check(err)
		}
		if !taken[at] {
			taken[at], count = true, count+1
		}
	}
	if m.Limit > 0 && count > m.Limit {
		return m.check(fmt.Errorf("at most %d may be chosen", m.Limit))
	}
	m.taken = taken
	m.store()
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
	switch {
	case said == "":
		return c.check(fmt.Errorf("say %s or %s", yes, no))
	case strings.HasPrefix(yes, said):
		c.Say(true)
	case strings.HasPrefix(no, said):
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
	found := -1
	for i, option := range options {
		label := strings.ToLower(option.Label)
		if label == folded {
			return i, nil
		}
		if !strings.HasPrefix(label, folded) {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("%q could be more than one of them", said)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("%q is not one of the choices", said)
	}
	return found, nil
}
