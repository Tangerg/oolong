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
// Oversized answers are discarded through their line ending before returning
// ErrAnswerTooLong, so a later Ask can resume at the next answer.
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
		if _, err := io.WriteString(p.output, "> "); err != nil {
			return err
		}
		answer, err := p.readAnswer()
		if errors.Is(err, io.EOF) {
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
	for {
		fragment, err := p.input.ReadSlice('\n')
		if err == nil {
			fragment = fragment[:len(fragment)-1]
		}
		line.append(fragment)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		answer, limitErr := line.result()
		if err == nil || errors.Is(err, io.EOF) && !line.empty() {
			return answer, limitErr
		}
		return "", errors.Join(err, limitErr)
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

func (l *answerLine) append(fragment []byte) {
	if len(fragment) == 0 {
		return
	}
	if l.carriageReturn {
		l.write([]byte{'\r'})
	}
	// Keep a possible CRLF terminator out of both the answer and its byte budget,
	// including when the reader splits immediately before the LF.
	l.carriageReturn = fragment[len(fragment)-1] == '\r'
	if l.carriageReturn {
		fragment = fragment[:len(fragment)-1]
	}
	l.write(fragment)
}

func (l *answerLine) write(fragment []byte) {
	if l.tooLong {
		return
	}
	if l.limit > 0 && len(fragment) > l.limit-l.text.Len() {
		l.tooLong = true
		l.text.Reset()
		return
	}
	l.text.Write(fragment)
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
		if _, err := fmt.Fprintln(out, text.Printable(line)); err != nil {
			return err
		}
	}
	return nil
}
