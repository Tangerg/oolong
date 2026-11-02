package highlight

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/alecthomas/chroma/v2"

	"github.com/Tangerg/oolong/ui/core/text"
)

func TestHighlightedTextDoesNotRetainItsSource(t *testing.T) {
	source := strings.Repeat("// discarded\n", 1<<12) + "var kept = 1\n"
	lines := New("").Lines("go", source)
	assertDetachedText(t, source, lines)
}

func TestPlainFallbackDoesNotRetainItsSource(t *testing.T) {
	source := strings.Repeat("discarded\n", 1<<12) + "kept"
	assertDetachedText(t, source, plain(source))
}

func assertDetachedText(t *testing.T, source string, lines []text.Line) {
	t.Helper()
	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	for row, line := range lines {
		for column, span := range line {
			if span.Text == "" {
				continue
			}
			at := uintptr(unsafe.Pointer(unsafe.StringData(span.Text))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
			if at >= sourceStart && at < sourceEnd {
				t.Fatalf("line %d span %d still shares the source allocation", row, column)
			}
		}
	}
}

type interruptedLexer struct{ chroma.Lexer }

func (interruptedLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	first := true
	return func() chroma.Token {
		if !first {
			panic("lexer interrupted")
		}
		first = false
		return chroma.Token{Type: chroma.Comment, Value: "one\n"}
	}, nil
}

func TestInterruptedLexerFallsBackToEverySourceRow(t *testing.T) {
	lines := highlight(interruptedLexer{}, "one\r\ntwo\n", New("").resolved())
	if len(lines) != 3 || lines[0].String() != "one" || lines[1].String() != "two" || lines[2].String() != "" {
		t.Fatalf("interrupted highlight lost source: %v", lines)
	}
}

func TestTokenStylesMayOnlyCoverTheSource(t *testing.T) {
	for _, tokens := range [][]chroma.Token{
		{{Type: chroma.Name, Value: "a"}},
		{{Type: chroma.Name, Value: "ac"}},
		{{Type: chroma.Name, Value: "abx"}},
	} {
		if got := sourceTokens(chroma.Literator(tokens...), "ab"); got != nil {
			t.Fatalf("published incomplete or altered source: %v", got)
		}
	}
}
