package grid

import (
	"bytes"
	"io"
	"strconv"
)

// writeAll transfers one logical payload completely. io.Copy supplies the
// io.Writer contract here, including rejecting a short write without an error.
// A canvas may settle only after every byte has crossed its writer boundary;
// treating a short write as a complete frame would make the next diff relative
// to terminal state that never existed.
func writeAll(w io.Writer, data []byte) error {
	_, err := io.Copy(w, bytes.NewReader(data))
	return err
}

// appendCSI appends one numeric control-sequence introducer. Screen and Inline
// publish differently, but the terminal wire spelling is one foundation fact.
func appendCSI(dst []byte, n int, final byte) []byte {
	dst = append(dst, '\x1b', '[')
	dst = strconv.AppendInt(dst, int64(n), 10)
	return append(dst, final)
}
