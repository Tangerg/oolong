package main

import (
	"context"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// defaultReplySource stands in for a remote producer. Its timer exists only while an
// answer is active, and every wait can be cancelled. Tests replace it with deterministic
// sources without replacing any application state or publication logic.
func defaultReplySource(ctx context.Context, prompt string, dst io.Writer) error {
	answer := reply(prompt)
	for chunk := range slices.Chunk([]byte(answer), mockChunkBytes) {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(mockChunkInterval):
		}
		n, err := dst.Write(chunk)
		if err != nil {
			return err
		}
		if n != len(chunk) {
			return io.ErrShortWrite
		}
	}
	return nil
}

func reply(prompt string) string {
	return strings.Join([]string{
		"# Streaming first",
		"",
		"You asked **" + clip(prompt) + "**. This answer is arriving as ordered bytes",
		"from a background producer.",
		"",
		"The complete Markdown document stays revisable until EOF. While receiving it remains",
		"selectable here; finalized output moves into the terminal's own scrollback.",
		"",
		"- ingress is bounded and applies backpressure",
		"- provisional content may change; published captures cannot",
		"- source and publication queues have explicit memory budgets",
		"",
		"> Source cancellation remains distinct from failure.",
		"",
		"When this paragraph finishes, the interface becomes idle and owns no timer.",
		"",
	}, "\n")
}

func clip(s string) string {
	const limit = 40
	return text.Of(s, grid.Style{}).Truncate(limit, "…").String()
}
