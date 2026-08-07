package grid

import "io"

// writeAll transfers one logical payload completely. A canvas may settle only
// after every byte has crossed its writer boundary; treating a short write as a
// complete frame would make the next diff relative to terminal state that never
// existed.
func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n < 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
