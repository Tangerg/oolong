package term

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestConsoleInputRecordMatchesTheWindowsABI(t *testing.T) {
	var record consoleInputRecord
	var key consoleKeyEvent
	for name, sizes := range map[string][2]uintptr{
		"record size":   {unsafe.Sizeof(record), 20},
		"event union":   {unsafe.Offsetof(record.key), 4},
		"key size":      {unsafe.Sizeof(key), 16},
		"key down":      {unsafe.Offsetof(key.keyDown), 0},
		"repeat count":  {unsafe.Offsetof(key.repeatCount), 4},
		"virtual key":   {unsafe.Offsetof(key.virtualKeyCode), 6},
		"scan code":     {unsafe.Offsetof(key.virtualScanCode), 8},
		"character":     {unsafe.Offsetof(key.unicodeChar), 10},
		"control state": {unsafe.Offsetof(key.controlKeyState), 12},
	} {
		if sizes[0] != sizes[1] {
			t.Errorf("%s = %d, want %d", name, sizes[0], sizes[1])
		}
	}
}

func consoleCharacters(text string) []consoleInputRecord {
	units := utf16.Encode([]rune(text))
	records := make([]consoleInputRecord, len(units))
	for i, unit := range units {
		records[i] = consoleCharacter(unit)
	}
	return records
}

func consoleCharacter(unit uint16) consoleInputRecord {
	return consoleInputRecord{
		eventType: consoleKeyEventType,
		key:       consoleKeyEvent{keyDown: 1, repeatCount: 1, unicodeChar: unit},
	}
}

func consoleChunks(t *testing.T, input *consoleText) string {
	t.Helper()
	var out strings.Builder
	for {
		chunk := input.read().data
		if len(chunk) > readChunkBytes || !utf8.Valid(chunk) {
			t.Fatalf("invalid UTF-8 chunk of %d bytes", len(chunk))
		}
		out.Write(chunk)
		if len(input.records) == 0 {
			return out.String()
		}
		if len(chunk) == 0 {
			t.Fatal("buffered records made no progress")
		}
	}
}

func TestConsoleCharactersPreserveTheVTByteStream(t *testing.T) {
	want := "\x1b[1;5A\x00中文😀\x1a"
	input := consoleText{records: consoleCharacters(want)}
	if got := consoleChunks(t, &input); got != want {
		t.Fatalf("characters = %q, want %q", got, want)
	}
}

func TestConsoleRecordsWithoutTextDoNotCreateCharacters(t *testing.T) {
	records := []consoleInputRecord{
		{eventType: 0x0002, key: consoleKeyEvent{keyDown: 1, repeatCount: 1, unicodeChar: 'm'}},
		{eventType: 0x0010, key: consoleKeyEvent{keyDown: 1}},
		{eventType: consoleKeyEventType, key: consoleKeyEvent{repeatCount: 1, unicodeChar: 'u'}},
		{eventType: consoleKeyEventType, key: consoleKeyEvent{keyDown: 1, repeatCount: 1, virtualScanCode: 42}},
		{eventType: consoleKeyEventType, key: consoleKeyEvent{keyDown: 1, unicodeChar: 'z'}},
		consoleCharacter(0),
		consoleCharacter('x'),
	}
	input := consoleText{records: records}
	if got := consoleChunks(t, &input); got != "\x00x" {
		t.Fatalf("characters = %q, want NUL followed by x", got)
	}
}

func TestConsoleRepeatsStayBoundedAndNeedNoNewRecord(t *testing.T) {
	repeated := consoleCharacter('a')
	repeated.key.repeatCount = 65535
	input := consoleText{records: append([]consoleInputRecord{repeated}, consoleCharacters("中😀")...)}
	want := strings.Repeat("a", int(repeated.key.repeatCount)) + "中😀"
	if got := consoleChunks(t, &input); got != want {
		t.Fatalf("repeat output differs: got %d bytes, want %d", len(got), len(want))
	}
}

func TestConsoleSurrogatesContinueAcrossRecordBatches(t *testing.T) {
	input := consoleText{records: []consoleInputRecord{consoleCharacter(0xd83d)}}
	if got := input.read().data; len(got) != 0 {
		t.Fatalf("leading surrogate emitted %q", got)
	}
	input.records = []consoleInputRecord{{eventType: 0x0010}, consoleCharacter(0xde00), consoleCharacter('x')}
	if got := consoleChunks(t, &input); got != "😀x" {
		t.Fatalf("continued characters = %q, want 😀x", got)
	}
}

func TestConsoleSurrogateArrivalDoesNotBackdateTheFollowingCharacter(t *testing.T) {
	started := time.Unix(1, 0)
	completed := started.Add(time.Second)
	for _, test := range []struct {
		name     string
		trailing []consoleInputRecord
		first    rune
	}{
		{"complete pair", []consoleInputRecord{consoleCharacter(0xde00), consoleCharacter('x')}, '😀'},
		{"malformed pair", []consoleInputRecord{consoleCharacter('x')}, utf8.RuneError},
	} {
		t.Run(test.name, func(t *testing.T) {
			text := consoleText{records: []consoleInputRecord{consoleCharacter(0xd83d)}, at: started}
			var stream input.Stream
			if got := text.read().decode(&stream); len(got) != 0 {
				t.Fatalf("leading surrogate produced %+v", got)
			}
			text.records, text.at = []consoleInputRecord{{eventType: 0x0010}}, started.Add(time.Millisecond)
			if got := text.read().decode(&stream); len(got) != 0 {
				t.Fatalf("noncharacter record produced %+v", got)
			}
			text.records, text.at = test.trailing, completed
			got := text.read().decode(&stream)
			if len(got) != 2 || got[0] != (input.Key{Rune: test.first, At: started}) ||
				got[1] != (input.Key{Rune: 'x', At: completed}) {
				t.Fatalf("cross-record origin leaked into following text: %+v", got)
			}
		})
	}
}

func TestConsoleRepeatsRetainTheirRecordObservationAcrossChunks(t *testing.T) {
	at := time.Unix(1, 0)
	record := consoleCharacter('r')
	record.key.repeatCount = readChunkBytes + 20
	want := int(record.key.repeatCount)
	text := consoleText{records: []consoleInputRecord{record}, at: at}
	var stream input.Stream
	var count int
	for len(text.records) > 0 {
		for _, event := range text.read().decode(&stream) {
			if event != (input.Key{Rune: 'r', At: at}) {
				t.Fatalf("buffered repeat changed origin: %+v", event)
			}
			count++
		}
	}
	if count != want {
		t.Fatalf("repeated %d characters, want %d", count, want)
	}
}

func TestConsoleResetReleasesTheIncompleteCharacterOrigin(t *testing.T) {
	started := time.Unix(1, 0)
	completed := started.Add(time.Second)
	text := consoleText{records: []consoleInputRecord{consoleCharacter(0xd83d)}, at: started}
	_ = text.read()
	text.reset()
	text.records, text.at = []consoleInputRecord{consoleCharacter(0xde00), consoleCharacter('x')}, completed
	var stream input.Stream
	got := text.read().decode(&stream)
	if len(got) != 2 || got[0] != (input.Key{Rune: utf8.RuneError, At: completed}) ||
		got[1] != (input.Key{Rune: 'x', At: completed}) {
		t.Fatalf("reset retained the prior foreground's origin: %+v", got)
	}
}

func TestMalformedConsoleSurrogatesPreserveFollowingText(t *testing.T) {
	for name, units := range map[string][]uint16{
		"leading before ordinary": {0xd83d, 'x'},
		"isolated trailing":       {0xde00, 'x'},
		"new leading":             {0xd800, 0xd83d, 0xde00, 'x'},
	} {
		t.Run(name, func(t *testing.T) {
			input := consoleText{}
			for _, unit := range units {
				input.records = append(input.records, consoleCharacter(unit))
			}
			want := string(utf16.Decode(units))
			if got := consoleChunks(t, &input); got != want {
				t.Fatalf("characters = %q, want %q", got, want)
			}
		})
	}
}

func TestConsoleChunkBoundaryDoesNotLoseASurrogateOrFollowingCharacter(t *testing.T) {
	for _, tail := range [][]uint16{{0xd83d, 0xde00}, {0xd83d, 'y'}} {
		prefix := strings.Repeat("x", readChunkBytes-3)
		input := consoleText{records: consoleCharacters(prefix)}
		for _, unit := range tail {
			input.records = append(input.records, consoleCharacter(unit))
		}
		want := prefix + string(utf16.Decode(tail))
		if got := consoleChunks(t, &input); got != want {
			t.Fatalf("chunk boundary changed text: got %d bytes, want %d", len(got), len(want))
		}
	}
}

func TestConsoleResetEndsOnlyTheIncompleteForegroundCharacter(t *testing.T) {
	input := consoleText{records: []consoleInputRecord{consoleCharacter(0xd83d)}}
	_ = input.read()
	input.records = []consoleInputRecord{consoleCharacter(0xde00), consoleCharacter('x')}
	input.reset()
	if got := consoleChunks(t, &input); got != "�x" {
		t.Fatalf("reset mixed foregrounds or lost complete records: %q", got)
	}
}

func FuzzConsoleTextPreservesUTF16AcrossBatches(f *testing.F) {
	f.Add([]byte{0x3d, 0xd8, 0, 0, 0xde, 0, 'x', 0, 2}, uint8(1))
	f.Add([]byte{0x3d, 0xd8, 1, 0, 0xde, 1, 0, 0, 0}, uint8(0))
	var long []byte
	for range 512 {
		long = append(long, 0xe5, 0x65, 7)
	}
	f.Add(long, uint8(63))
	f.Fuzz(func(t *testing.T, data []byte, batch uint8) {
		if len(data) > 4096 {
			return
		}
		var records []consoleInputRecord
		var units []uint16
		for len(data) >= 3 {
			unit := uint16(data[0]) | uint16(data[1])<<8
			count := 1 + int(data[2]&7)
			record := consoleCharacter(unit)
			record.key.repeatCount = uint16(count)
			records = append(records, record)
			for range count {
				units = append(units, unit)
			}
			data = data[3:]
		}
		// An ordinary character settles any final leading surrogate, so the
		// standard library's complete UTF-16 decoder is the streaming oracle.
		records = append(records, consoleCharacter('!'))
		units = append(units, '!')
		want := string(utf16.Decode(units))
		for _, size := range []int{len(records), 1 + int(batch%64)} {
			var input consoleText
			var got strings.Builder
			for start := 0; start < len(records); start += size {
				end := min(start+size, len(records))
				input.records = append([]consoleInputRecord(nil), records[start:end]...)
				got.WriteString(consoleChunks(t, &input))
			}
			if got.String() != want {
				t.Fatalf("batch size %d decoded %q, want %q", size, got.String(), want)
			}
		}
	})
}
