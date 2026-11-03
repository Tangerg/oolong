package kit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
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
func Ask(form *base.Form, in io.Reader, out io.Writer) (scene.ActionResult[base.SubmissionID], error) {
	if form == nil {
		return scene.ActionResult[base.SubmissionID]{}, nil
	}
	lines := bufio.NewScanner(in)
	return form.SubmitWith(func(field base.Field) error {
		spoken, ok := field.(spokenField)
		if !ok {
			return fmt.Errorf("kit: %q cannot be answered in words", field.Prompt())
		}
		return ask(spoken, lines, out)
	})
}

type spokenField interface {
	base.Field
	Ask() string
	Reply(said string) error
}

// ask puts one question until it is answered. The method set is defined here because
// this is the consumer of the optional spoken-field capability.
func ask(field spokenField, lines *bufio.Scanner, out io.Writer) error {
	for {
		if err := writeAnswerLines(out, field.Ask()); err != nil {
			return err
		}
		if _, err := io.WriteString(out, "> "); err != nil {
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
		} else if errors.Is(err, base.ErrReplyUnavailable) || errors.Is(err, base.ErrValidationSuperseded) {
			return err
		} else if printed := writeAnswerLines(out, err.Error()); printed != nil {
			return printed
		}
	}
}

// Field text can originate outside the application. Preserve logical lines while
// applying the screen renderer's control-character policy to each one.
func writeAnswerLines(out io.Writer, message string) error {
	for line := range strings.SplitSeq(message, "\n") {
		if _, err := fmt.Fprintln(out, text.Printable(line)); err != nil {
			return err
		}
	}
	return nil
}
