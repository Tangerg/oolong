package kit

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/oolong/components/headless"
)

// Ask puts a form one question at a time, in words.
//
// It is the same form answered without a screen: for somebody reading with a screen
// reader, for a program whose output is a pipe, and for a script that would rather
// say "good" than press the down arrow twice. Every field asks what it is asking and
// says what is wrong with an answer, so what is here is only the conversation: write
// the question, read the line, say what was wrong, ask again.
//
// A field that has no words for itself stops the conversation rather than being
// skipped. Skipping it would collect a form with an answer missing and submit it
// anyway, which is the one outcome worse than refusing.
//
// The form is submitted at the end, and what it says about the answers together is
// reported as an error. Nothing here draws, so a caller wanting both this and a
// screen builds the same form twice and uses whichever the terminal turned out to be.
func Ask(form *headless.Form, in io.Reader, out io.Writer) error {
	if form == nil {
		return nil
	}
	lines := bufio.NewScanner(in)
	for _, field := range form.Fields() {
		spoken, ok := field.(spokenField)
		if !ok {
			return fmt.Errorf("kit: %q cannot be answered in words", field.Prompt())
		}
		if err := ask(spoken, lines, out); err != nil {
			return err
		}
	}
	if !form.Submit() {
		if err := form.Error(); err != nil {
			return err
		}
		return errors.New("kit: the answers were not accepted")
	}
	return nil
}

// spokenField is the capability Ask consumes from a field.
type spokenField interface {
	headless.Field
	Ask() string
	Reply(said string) error
}

// ask puts one question until it is answered. The method set is defined here because
// this is the consumer of the optional spoken-field capability.
func ask(field spokenField, lines *bufio.Scanner, out io.Writer) error {
	for {
		if _, err := fmt.Fprintf(out, "%s\n> ", field.Ask()); err != nil {
			return err
		}
		if !lines.Scan() {
			if err := lines.Err(); err != nil {
				return err
			}
			// The input ended part way through. What has been answered stays answered,
			// and saying so is better than submitting a form nobody finished.
			return fmt.Errorf("kit: %q was left unanswered: %w", field.Prompt(), io.ErrUnexpectedEOF)
		}
		if err := field.Reply(lines.Text()); err == nil {
			return nil
		} else if _, printed := fmt.Fprintf(out, "%v\n", err); printed != nil {
			return printed
		}
	}
}
