package input

import (
	"bytes"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// Only the exact framing sequences may change whether subsequent bytes are keys or text.
const (
	pasteOpen  = ansi.CSI + "200~"
	pasteClose = ansi.CSI + "201~"
)

func (p *Parser) readPaste() (string, bool) {
	i := 0
	for i < len(p.buf) {
		if p.buf[i] == esc {
			rest := p.buf[i:]
			switch shared := commonPrefix(rest, pasteClose); {
			case shared == len(pasteClose):
				p.take(i + len(pasteClose))
				return p.closePaste(), true
			case shared == len(rest):
				p.take(i)
				return "", false
			}
		}
		p.paste = append(p.paste, p.buf[i])
		i++
		if len(p.paste) >= maxPaste {
			end := len(p.paste) - 1
			for end > 0 && !utf8.RuneStart(p.paste[end]) {
				end--
			}
			if utf8.FullRune(p.paste[end:]) {
				end = len(p.paste)
			}
			text := string(p.paste[:end])
			p.paste = bytes.Clone(p.paste[end:])
			p.take(i)
			return text, true
		}
	}
	p.take(len(p.buf))
	return "", false
}

func (p *Parser) closePaste() string {
	p.frame = noFrame
	text := string(p.paste)
	p.paste = nil
	return text
}

func commonPrefix(a []byte, b string) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
