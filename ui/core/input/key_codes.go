package input

const (
	deleteByte            = 0x7f
	kittyFunctionKeysFrom = 0xe000
	kittyFunctionKeysTo   = 0xf8ff
	kittyF1               = 57364
	kittyKeypadBegin      = kittyF1 + int(KeypadBegin-F1)
)

const (
	kittyEscape = kittyFunctionKeysFrom + iota
	kittyEnter
	kittyTab
	kittyBackspace
	kittyInsert
	kittyDelete
	kittyLeft
	kittyRight
	kittyUp
	kittyDown
	kittyPageUp
	kittyPageDown
	kittyHome
	kittyEnd
)

const (
	tildeHome = 1 + iota
	tildeInsert
	tildeDelete
	tildeEnd
	tildePageUp
	tildePageDown
	tildeHomeAlternate
	tildeEndAlternate
)

const (
	tildeF1 = 11 + iota
	tildeF2
	tildeF3
	tildeF4
	tildeF5
)

const (
	tildeF6 = 17 + iota
	tildeF7
	tildeF8
	tildeF9
	tildeF10
)

const (
	tildeF11 = 23 + iota
	tildeF12
)

var kittyKeyNumbers = map[int]Code{
	'\b': Backspace, deleteByte: Backspace, kittyBackspace: Backspace,
	'\t': Tab, kittyTab: Tab,
	'\r': Enter, kittyEnter: Enter,
	esc: Esc, kittyEscape: Esc,
	kittyInsert:   Insert,
	kittyDelete:   Delete,
	kittyLeft:     Left,
	kittyRight:    Right,
	kittyUp:       Up,
	kittyDown:     Down,
	kittyPageUp:   PageUp,
	kittyPageDown: PageDown,
	kittyHome:     Home,
	kittyEnd:      End,
}

// SS3 and CSI share final-byte identities, except for cursor-position replies.
var finalByteKeys = map[byte]Code{
	'A': Up, 'B': Down, 'C': Right, 'D': Left,
	'H': Home, 'F': End, 'E': KeypadBegin,
	'P': F1, 'Q': F2, 'R': F3, 'S': F4,
}

var tildeKeyNumbers = map[int]Code{
	tildeHome: Home, tildeHomeAlternate: Home,
	tildeInsert: Insert,
	tildeDelete: Delete,
	tildeEnd:    End, tildeEndAlternate: End,
	tildePageUp:      PageUp,
	tildePageDown:    PageDown,
	tildeF1:          F1,
	tildeF2:          F2,
	tildeF3:          F3,
	tildeF4:          F4,
	tildeF5:          F5,
	tildeF6:          F6,
	tildeF7:          F7,
	tildeF8:          F8,
	tildeF9:          F9,
	tildeF10:         F10,
	tildeF11:         F11,
	tildeF12:         F12,
	kittyKeypadBegin: KeypadBegin,
}
