package kit_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type promptReadFunc func([]byte) (int, error)

func (f promptReadFunc) Read(data []byte) (int, error) { return f(data) }

func TestPrompterPreservesInputFailuresContainingControlSentinels(t *testing.T) {
	failure := errors.New("transport failed")
	for _, test := range []struct {
		name string
		err  error
	}{
		{"wrapped EOF", fmt.Errorf("read failed: %w", io.EOF)},
		{"joined EOF", errors.Join(io.EOF, failure)},
		{"buffer full", bufio.ErrBufferFull},
		{"wrapped buffer full", fmt.Errorf("read failed: %w", bufio.ErrBufferFull)},
		{"joined buffer full", errors.Join(bufio.ErrBufferFull, failure)},
	} {
		for _, partial := range []string{"", "partial"} {
			for _, limit := range []int{0, 2} {
				t.Run(test.name+"/bytes="+strconv.Itoa(len(partial))+"/limit="+strconv.Itoa(limit), func(t *testing.T) {
					reads, submissions := 0, 0
					input := promptReadFunc(func(data []byte) (int, error) {
						reads++
						if reads == 1 {
							return copy(data, partial), test.err
						}
						return copy(data, "next\n"), nil
					})
					prompter := newPrompter(t, kit.PrompterConfig{Input: input, Output: io.Discard, MaxAnswerBytes: limit})
					field := base.NewText(base.TextConfig{Initial: "untouched"})
					form := base.NewForm(base.FormConfig{Fields: []base.Field{field}, Submit: func(base.SubmissionID) (scene.ActionState, error) {
						submissions++
						return scene.Applied, nil
					}})
					result, err := prompter.Ask(form)
					if !errors.Is(err, test.err) {
						t.Errorf("input error = %v, want %v", err, test.err)
					}
					if limit > 0 && len(partial) > limit && !errors.Is(err, kit.ErrAnswerTooLong) {
						t.Errorf("input failure lost the exceeded budget: %v", err)
					}
					if reads != 1 || submissions != 0 || field.Text() != "untouched" || result.State != scene.Unchanged {
						t.Fatalf("failed read advanced collection: reads=%d submissions=%d answer=%q state=%v", reads, submissions, field.Text(), result.State)
					}
				})
			}
		}
	}
}

func TestPrompterPreservesBufferedFailureAfterACompleteAnswer(t *testing.T) {
	for _, failure := range []error{bufio.ErrBufferFull, errors.Join(io.EOF, errors.New("read failed after complete answer"))} {
		t.Run(failure.Error(), func(t *testing.T) {
			reads := 0
			input := promptReadFunc(func(data []byte) (int, error) {
				reads++
				return copy(data, "complete\n"), failure
			})
			prompter := newPrompter(t, kit.PrompterConfig{Input: input, Output: io.Discard})
			first := base.NewText(base.TextConfig{})
			if _, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{first}})); err != nil || first.Text() != "complete" {
				t.Fatalf("complete answer = %q, error = %v", first.Text(), err)
			}
			second := base.NewText(base.TextConfig{Initial: "untouched"})
			if _, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{second}})); !errors.Is(err, failure) || second.Text() != "untouched" || reads != 1 {
				t.Fatalf("buffered failure = %v, answer = %q, reads = %d", err, second.Text(), reads)
			}
		})
	}
}

func TestPrompterPreservesFailureAtBufferedReadBoundaries(t *testing.T) {
	bufferBytes := bufio.NewReader(strings.NewReader("")).Size()
	for _, size := range []int{bufferBytes - 1, bufferBytes, bufferBytes + 1, 2 * bufferBytes} {
		for _, limit := range []int{0, size - 1} {
			t.Run("bytes="+strconv.Itoa(size)+"/limit="+strconv.Itoa(limit), func(t *testing.T) {
				remaining := strings.Repeat("x", size)
				reads := 0
				input := promptReadFunc(func(data []byte) (int, error) {
					reads++
					if remaining == "" {
						return copy(data, "next\n"), nil
					}
					n := copy(data, remaining)
					remaining = remaining[n:]
					if remaining == "" {
						return n, bufio.ErrBufferFull
					}
					return n, nil
				})
				prompter := newPrompter(t, kit.PrompterConfig{Input: input, Output: io.Discard, MaxAnswerBytes: limit})
				field := base.NewText(base.TextConfig{Initial: "untouched"})
				result, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{field}}))
				if !errors.Is(err, bufio.ErrBufferFull) || limit > 0 && !errors.Is(err, kit.ErrAnswerTooLong) {
					t.Errorf("read failure = %v", err)
				}
				if reads != (size+bufferBytes-1)/bufferBytes || field.Text() != "untouched" || result.State != scene.Unchanged {
					t.Fatalf("failed read advanced collection: reads=%d answer bytes=%d state=%v", reads, len(field.Text()), result.State)
				}
			})
		}
	}
}
