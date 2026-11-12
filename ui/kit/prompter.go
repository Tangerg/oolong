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
	"github.com/Tangerg/oolong/ui/internal/identity"
)

var (
	ErrAnswerTooLong = errors.New("kit: answer exceeds byte limit")
	ErrPromptActive  = errors.New("kit: a form is already being answered")
)

type PrompterConfig struct {
	Input  io.Reader
	Output io.Writer
	// The limit excludes the line ending. Zero imposes no transport limit;
	// field validation remains authoritative for accepted answers.
	MaxAnswerBytes int
}

// Prompter retains read-ahead across forms over one borrowed input stream. Reuse
// it for successive Ask calls; creating another reader would lose buffered input.
// A Prompter must not be copied after construction. Calls must be serial;
// callbacks cannot start another form before Ask returns.
// The caller retains responsibility for closing the input and output.
type Prompter struct {
	noCopy         noCopy
	input          *bufio.Reader
	output         io.Writer
	maxAnswerBytes int
	asking         bool
}

func NewPrompter(cfg PrompterConfig) (*Prompter, error) {
	if identity.IsNil(cfg.Input) || identity.IsNil(cfg.Output) {
		return nil, errors.New("kit: prompting requires input and output")
	}
	if cfg.MaxAnswerBytes < 0 {
		return nil, errors.New("kit: negative answer byte limit")
	}
	return &Prompter{input: bufio.NewReader(cfg.Input), output: cfg.Output, maxAnswerBytes: cfg.MaxAnswerBytes}, nil
}

// Ask leaves field validation and submission with the form. An unavailable
// spoken field or failed input cannot silently submit an incomplete form.
// Owner failures stop collection even when a domain effect was accepted; only
// correctable parse and validation failures ask for another answer.
// Oversized answers are discarded through their line ending before returning
// ErrAnswerTooLong, so a later Ask can resume at the next answer.
// Only bare io.EOF completes an unterminated answer; other input failures must
// not turn partial input into a submitted answer.
func (p *Prompter) Ask(form *base.Form) (scene.ActionResult[base.SubmissionID], error) {
	if p == nil || p.input == nil {
		return scene.ActionResult[base.SubmissionID]{}, errors.New("kit: uninitialized prompter")
	}
	if p.asking {
		return scene.ActionResult[base.SubmissionID]{}, ErrPromptActive
	}
	if form == nil {
		return scene.ActionResult[base.SubmissionID]{}, errors.New("kit: Ask requires a form")
	}
	p.asking = true
	defer func() { p.asking = false }()
	return form.SubmitWith(func(field base.Field) error {
		spoken, ok := field.(base.SpokenField)
		if !ok {
			return fmt.Errorf("kit: %q cannot be answered in words", field.Prompt())
		}
		return p.ask(spoken)
	})
}

func (p *Prompter) ask(field base.SpokenField) error {
	for {
		if err := writeAnswerLines(p.output, field.Ask()); err != nil {
			return err
		}
		if err := writePromptText(p.output, "> "); err != nil {
			return err
		}
		answer, err := p.readAnswer()
		if err == io.EOF { //nolint:errorlint // Only bare EOF means an absent answer; wrapped and joined read failures must survive.
			return fmt.Errorf("kit: %q was left unanswered: %w", field.Prompt(), io.ErrUnexpectedEOF)
		}
		if err != nil {
			return err
		}
		if err := field.Reply(answer); err == nil {
			return nil
		} else if errors.Is(err, base.ErrReplyUnavailable) || errors.Is(err, base.ErrValidationSuperseded) {
			return err
		} else if printed := writeAnswerLines(p.output, err.Error()); printed != nil {
			return printed
		}
	}
}

func (p *Prompter) readAnswer() (string, error) {
	line := answerLine{limit: p.maxAnswerBytes}
	// ReadSlice's ErrBufferFull can also originate from its reader. ReadByte
	// leaves that failure distinct from the framing of an answer.
	for {
		value, err := p.input.ReadByte()
		if err == io.EOF {
			if !line.empty() {
				return line.result()
			}
			return "", err
		}
		if err != nil {
			_, limitErr := line.result()
			return "", errors.Join(err, limitErr)
		}
		if value == '\n' {
			return line.result()
		}
		line.append(value)
	}
}

type answerLine struct {
	text           strings.Builder
	limit          int
	tooLong        bool
	carriageReturn bool
}

func (l *answerLine) empty() bool {
	return l.text.Len() == 0 && !l.carriageReturn && !l.tooLong
}

func (l *answerLine) append(value byte) {
	if l.carriageReturn {
		l.write('\r')
	}
	// Keep a possible CRLF terminator out of both the answer and its byte budget,
	// including when the next byte is not available in the current read buffer.
	l.carriageReturn = value == '\r'
	if !l.carriageReturn {
		l.write(value)
	}
}

func (l *answerLine) write(value byte) {
	if l.tooLong {
		return
	}
	if l.limit > 0 && l.text.Len() == l.limit {
		l.tooLong = true
		l.text.Reset()
		return
	}
	l.text.WriteByte(value)
}

func (l *answerLine) result() (string, error) {
	if l.tooLong {
		return "", ErrAnswerTooLong
	}
	return l.text.String(), nil
}

// Field text can originate outside the application. Preserve logical lines while
// applying the screen renderer's control-character policy to each one.
func writeAnswerLines(out io.Writer, message string) error {
	for line := range strings.SplitSeq(message, "\n") {
		if err := writePromptText(out, text.Printable(line)+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// An incomplete question or diagnostic must not consume the next answer.
func writePromptText(out io.Writer, message string) error {
	_, err := io.Copy(out, strings.NewReader(message))
	return err
}
