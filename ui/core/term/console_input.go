package term

import (
	"unicode/utf16"
	"unicode/utf8"
)

// These fixed-width fields match INPUT_RECORD and its KEY_EVENT_RECORD union
// member. Windows has already converted virtual-terminal input into character
// records; this layer only preserves their UTF-16 stream.
type consoleInputRecord struct {
	eventType uint16
	_         uint16
	key       consoleKeyEvent
}

type consoleKeyEvent struct {
	keyDown         int32
	repeatCount     uint16
	virtualKeyCode  uint16
	virtualScanCode uint16
	unicodeChar     uint16
	controlKeyState uint32
}

const consoleKeyEventType = 0x0001

// consoleText owns records already removed from the console, including a repeat
// count or surrogate that does not fit in the current UTF-8 chunk.
type consoleText struct {
	records []consoleInputRecord
	leading uint16
}

func (c *consoleText) reset() {
	c.leading = 0
}

func (c *consoleText) read() []byte {
	var out []byte
	for len(out)+utf8.UTFMax <= readChunkBytes {
		unit, ok := c.peek()
		if !ok {
			break
		}
		if c.leading != 0 {
			leading := c.leading
			c.leading = 0
			if unit >= 0xdc00 && unit <= 0xdfff {
				c.consume()
				out = utf8.AppendRune(out, utf16.DecodeRune(rune(leading), rune(unit)))
			} else {
				// Keep the next unit for the next iteration: a malformed pair must
				// neither lose that character nor exceed this chunk's byte budget.
				out = utf8.AppendRune(out, utf8.RuneError)
			}
			continue
		}
		c.consume()
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			c.leading = unit
		case unit >= 0xdc00 && unit <= 0xdfff:
			out = utf8.AppendRune(out, utf8.RuneError)
		default:
			out = utf8.AppendRune(out, rune(unit))
		}
	}
	return out
}

func (c *consoleText) peek() (uint16, bool) {
	for len(c.records) > 0 {
		record := &c.records[0]
		key := &record.key
		// VT character records have scan code zero, including the NUL produced
		// by Ctrl-Space. A zero character with a physical scan code is a key
		// without text, such as an isolated modifier.
		if record.eventType == consoleKeyEventType && key.keyDown != 0 &&
			key.repeatCount != 0 && (key.unicodeChar != 0 || key.virtualScanCode == 0) {
			return key.unicodeChar, true
		}
		c.records = c.records[1:]
	}
	return 0, false
}

func (c *consoleText) consume() {
	key := &c.records[0].key
	key.repeatCount--
	if key.repeatCount == 0 {
		c.records = c.records[1:]
	}
}
